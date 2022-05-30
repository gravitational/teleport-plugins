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

package resources

import (
	"context"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/gravitational/teleport/api/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	resourcesv2 "github.com/gravitational/teleport-plugins/kubernetes/apis/resources/v2"
	resourcesv5 "github.com/gravitational/teleport-plugins/kubernetes/apis/resources/v5"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient kclient.Client
var testEnv *envtest.Environment
var ctxTearDown context.CancelFunc

var teleportClient *integration.Client
var teleportServer teleportServerManager

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	var err error

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = resourcesv5.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = resourcesv2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = kclient.New(cfg, kclient.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	ctx := context.Background()
	userName := validRandomResourceName("user-")
	roleName := validRandomResourceName("role-")

	teleportServer = startTeleportServer(ctx)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		err = teleportServer.auth.Run(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	ready, err := teleportServer.auth.WaitReady(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(ready).To(BeTrue())

	teleportServer.createUserRole(ctx, userName, roleName)
	teleportClient = teleportServer.newSignedClient(ctx, userName)

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&RoleReconciler{
		Client:         k8sClient,
		Scheme:         k8sManager.GetScheme(),
		TeleportClient: teleportClient.Client,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&UserReconciler{
		Client:         k8sClient,
		Scheme:         k8sManager.GetScheme(),
		TeleportClient: teleportClient.Client,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	ctx, ctxTearDown = context.WithCancel(ctx)
	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

func (t *teleportServerManager) newSignedClient(ctx context.Context, userName string) *integration.Client {
	client, err := t.NewClient(ctx, t.auth, userName)
	Expect(err).NotTo(HaveOccurred())
	return client
}

type teleportServerManager struct {
	*integration.Integration
	auth *integration.AuthService
}

func (t *teleportServerManager) createUserRole(ctx context.Context, userName, roleName string) {
	bootstrap := integration.Bootstrap{}
	unrestricted := []string{"list", "create", "read", "update", "delete"}
	_, err := bootstrap.AddRole(roleName, types.RoleSpecV5{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("role", unrestricted),
				types.NewRule("user", unrestricted),
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	_, err = bootstrap.AddUserWithRoles(userName, roleName)
	Expect(err).NotTo(HaveOccurred())

	err = t.Bootstrap(ctx, t.auth, bootstrap.Resources())
	Expect(err).NotTo(HaveOccurred())
}

func startTeleportServer(ctx context.Context) teleportServerManager {
	teleport, err := integration.NewFromEnv(ctx)
	Expect(err).NotTo(HaveOccurred())

	auth, err := teleport.NewAuthService(integration.WithCache())
	Expect(err).NotTo(HaveOccurred())

	return teleportServerManager{
		auth:        auth,
		Integration: teleport,
	}
}

var _ = AfterSuite(func() {
	By("tearing down the test environment")

	ctxTearDown()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	teleportServer.Close()
})

func createNamespaceForTest(ctx context.Context) *core.Namespace {
	ns := &core.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: validRandomResourceName("ns-")},
	}

	err := k8sClient.Create(ctx, ns)
	Expect(err).ToNot(HaveOccurred())

	return ns
}

func deleteNamespaceForTest(ctx context.Context, ns *core.Namespace) {
	err := k8sClient.Delete(ctx, ns)
	Expect(err).ToNot(HaveOccurred())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz1234567890")

func validRandomResourceName(prefix string) string {
	b := make([]rune, 5)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}
