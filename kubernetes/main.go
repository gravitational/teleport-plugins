/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/alecthomas/kong"
	"github.com/jonboulle/clockwork"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport-plugins/lib/tctl"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"

	authv10 "github.com/gravitational/teleport-plugins/kubernetes/apis/auth/v10"
	configv10 "github.com/gravitational/teleport-plugins/kubernetes/apis/config/v10"
	"github.com/gravitational/teleport-plugins/kubernetes/apis/resources"
	authctrl "github.com/gravitational/teleport-plugins/kubernetes/controllers/auth"
	resourcesctrl "github.com/gravitational/teleport-plugins/kubernetes/controllers/resources"
	"github.com/gravitational/teleport-plugins/kubernetes/crd"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const initTimeout = 10 * time.Second

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(resources.AddToScheme(scheme))
	utilruntime.Must(configv10.AddToScheme(scheme))
	utilruntime.Must(authv10.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

var CLI struct {
	StartSidecar struct {
		Config string `kong:"help='The operator will load its initial configuration from this file. Omit this flag to use the default configuration values.',placeholder='/etc/teleport/operator.yaml',default='/etc/teleport/operator.yaml'"`
	} `kong:"cmd,help='Runs Teleport Operator in a sidecar mode'"`
	InstallCRDs struct {
		Force bool `kong:"help='Overwrite existing CRDs anyway'"`
	} `kong:"cmd,name=install-crds,help='Installs Custom Resource Definitions to your Kubernetes cluster'"`
	ZapCLI
}

func main() {
	CLI.ZapDevel = true
	cli := kong.Parse(&CLI)

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(CLI.ZapOptions())))

	var (
		newClient func(ctx context.Context) (*client.Client, error)
		signer    authctrl.Signer
	)
	ctrlOptions := ctrl.Options{Scheme: scheme}
	restConfig := ctrl.GetConfigOrDie()

	switch cli.Command() {
	case "start-sidecar":
		var err error

		config := configv10.DefaultSidecarConfig()

		ctrlOptions, err = ctrlOptions.AndFrom(ctrl.ConfigFile().AtPath(CLI.StartSidecar.Config).OfKind(&config))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			cli.Exit(1)
		}

		namespace := config.Scope.Namespace
		if namespace == "" {
			setupLog.Error(trace.BadParameter("namespace scope must be set in a sidecar mode"), "unable to initialize a connection")
			cli.Exit(1)
		}
		ctrlOptions.Namespace = namespace

		newClient = func(ctx context.Context) (*client.Client, error) {
			clt, err := lib.NewSidecarClient(ctx, lib.SidecarOptions{
				ConfigPath: config.Teleport.Config,
				Role:       config.Teleport.Role,
				User:       config.Teleport.User,
				Addr:       config.Teleport.Addr,
			})
			if err != nil {
				return nil, trace.Wrap(err, "failed to connect to a locally running Teleport as a sidecar")
			}
			return clt, nil
		}

		signer = tctl.Tctl{ConfigPath: config.Teleport.Config, AuthServer: config.Teleport.Addr}

	case "install-crds":
		ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
		defer cancel()
		results, err := crd.Install(ctx, restConfig, Version, CLI.InstallCRDs.Force)
		for _, result := range results {
			logFields := []interface{}{
				"name", result.CRDName,
				"result", result.OperationResult,
				"new-operator-version", result.NewOperatorVersion,
				"added-crd-versions", result.AddedCRDVersions,
			}
			updated := make([]string, 0, len(result.UpdatedCRDVersions))
			for ver := range result.UpdatedCRDVersions {
				updated = append(updated, ver)
			}
			logFields = append(logFields, "updated-crd-versions", updated)
			for ver, old := range result.UpdatedCRDVersions {
				logFields = append(logFields, fmt.Sprintf("current.%s.version", ver), old)
			}

			setupLog.Info("successfully installed CRD to the cluster", logFields...)
		}
		if err != nil {
			setupLog.Error(err, "installation failed")
			cli.Exit(1)
		}
		return

	default:
		// Lets panic here because normally it must not happen - kong must prevent the case when the command is unknown.
		panic("unsupported command " + cli.Command())
	}

	process := lib.NewProcess(context.Background())
	go lib.ServeSignals(process, 10*time.Second)

	mgr, err := ctrl.NewManager(restConfig, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "unable to start operator")
		cli.Exit(1)
	}

	// This variable should be considered to be set if only the initJob is completed without error.
	var client *client.Client

	initJob := lib.NewServiceJob(func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, initTimeout)
		defer cancel()
		process.OnTerminate(func(ctx context.Context) error {
			cancel()
			return nil
		})

		setupLog.Info("connecting to Teleport")
		backoff := backoff.NewDecorr(time.Millisecond, time.Second, clockwork.NewRealClock())
		for {
			var clientErr error
			client, clientErr = newClient(ctx)
			if clientErr == nil {
				break
			}
			if err := backoff.Do(ctx); lib.IsCanceled(err) {
				return trace.Wrap(err)
			} else if err != nil {
				return trace.Wrap(clientErr)
			}
		}
		setupLog.Info("connected to Teleport")

		setupLog.Info("checking CRDs installed")
		if err := crd.Check(ctx, restConfig, Version); err != nil {
			return trace.Wrap(err)
		}

		if err := (resourcesctrl.Reconciler{
			ReconcilerImpl: resourcesctrl.NewRoleReconciler(mgr.GetClient(), mgr.GetScheme()),
			Client:         client,
		}).SetupWithManager(mgr); err != nil {
			return trace.Wrap(err, "unable to set up Role controller")
		}

		if err := (resourcesctrl.Reconciler{
			ReconcilerImpl: resourcesctrl.NewUserReconciler(mgr.GetClient(), mgr.GetScheme()),
			Client:         client,
		}).SetupWithManager(mgr); err != nil {
			return trace.Wrap(err, "unable to set up User controller")
		}

		if err = (authctrl.IdentityReconciler{
			Kube:        mgr.GetClient(),
			Scheme:      mgr.GetScheme(),
			Signer:      signer,
			RefreshRate: 30 * time.Second,
		}).SetupWithManager(mgr); err != nil {
			return trace.Wrap(err, "unable to set up Identity controller")
		}
		//+kubebuilder:scaffold:builder
		return nil
	})

	if err := mgr.AddReadyzCheck("readyz", func(r *http.Request) error {
		ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
		defer cancel()
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		case <-initJob.Done():
			return trace.Wrap(initJob.Err())
		}
	}); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		cli.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", func(r *http.Request) error {
		ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
		defer cancel()
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		case <-initJob.Done():
			if err := initJob.Err(); err != nil {
				return trace.Wrap(err)
			}
		}
		_, err = client.Ping(ctx)
		return trace.Wrap(err)
	}); err != nil {
		setupLog.Error(err, "unable to set up health check")
		cli.Exit(1)
	}

	process.SpawnCriticalJob(initJob)
	process.SpawnCritical(func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		if err := authctrl.SetupIndexes(ctx, mgr.GetCache()); err != nil {
			return trace.Wrap(err, "unable to set up cache indexes")
		}
		process.OnTerminate(func(ctx context.Context) error {
			cancel()
			return nil
		})
		setupLog.Info("starting operator")
		if err := mgr.Start(ctx); err != nil {
			return trace.Wrap(err, "problem running operator")
		}
		return nil
	})

	<-process.Done()
	err = process.CriticalError()
	if err == nil {
		return
	}
	if agg, ok := trace.Unwrap(err).(trace.Aggregate); ok {
		for _, err := range agg.Errors() {
			setupLog.Error(err, "critical error occurred while running the operator")
		}
	} else {
		setupLog.Error(err, "critical error occurred while running the operator")
	}
	cli.Exit(1)
}
