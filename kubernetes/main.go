/*
Copyright 2022 Gravitational, Inc.

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
	"embed"
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"

	resourcesv2 "github.com/gravitational/teleport-plugins/kubernetes/apis/resources/v2"
	resourcesv5 "github.com/gravitational/teleport-plugins/kubernetes/apis/resources/v5"
	resourcescontrollers "github.com/gravitational/teleport-plugins/kubernetes/controllers/resources"
	"github.com/gravitational/teleport-plugins/kubernetes/crd"
	"github.com/gravitational/teleport-plugins/kubernetes/sidecar"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	//go:embed config/crd/bases/*.teleport.dev_*.yaml
	crdFS embed.FS
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(resourcesv5.AddToScheme(scheme))
	utilruntime.Must(resourcesv2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(apiextv1.AddToScheme(scheme))
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	var err error
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         true,
		LeaderElectionID:       "cf52bb2b.teleport.dev",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var teleportClient *client.Client

	backoff := backoff.NewDecorr(time.Second, 5*time.Second, clockwork.NewRealClock())
	for {
		teleportClient, err = sidecar.NewSidecarClient(ctx, sidecar.Options{})
		if err == nil {
			break
		}
		setupLog.Error(err, "failed to connect to teleport cluster, backing off")

		err = backoff.Do(ctx)
		if lib.IsCanceled(err) {
			setupLog.Error(err, "deadline exceeded waiting for teleport cluster")
			os.Exit(1)
		}
		if err != nil {
			setupLog.Error(err, "backoff failed")
			os.Exit(1)
		}
	}
	setupLog.Info("connected to Teleport")

	if err = (&resourcescontrollers.RoleReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		TeleportClient: teleportClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Role")
		os.Exit(1)
	}
	if err = (&resourcescontrollers.UserReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		TeleportClient: teleportClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "User")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		// Install CRDs
		setupLog.Info("installing CRDs")
		if err := crd.Upsert(ctx, setupLog, crdFS, mgr.GetClient()); err != nil {
			return trace.Wrap(err)
		}

		return nil
	})); err != nil {
		setupLog.Error(err, "unable to set up CRDs")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
