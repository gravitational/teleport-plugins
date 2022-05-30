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

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	resourcesv2 "github.com/gravitational/teleport-plugins/kubernetes/apis/resources/v2"
)

var _ = Describe("Users", func() {
	When("a new user is created in k8s", func() {
		ctx := context.Background()
		ns := &core.Namespace{}
		var userName string

		BeforeEach(func() {
			ns = createNamespaceForTest(ctx)
			userName = validRandomResourceName("user-")
			kCreateDummyUser(ctx, ns.Name, userName)
		})

		AfterEach(func() {
			deleteNamespaceForTest(ctx, ns)
		})

		It("creates the user in Teleport", func() {
			Eventually(func(g Gomega) {
				tUser, err := teleportClient.GetUser(userName, false)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(tUser.GetName()).Should(Equal(userName))

			}).Should(Succeed())
		})

		It("sets the OriginLabel to kubernetes", func() {
			Eventually(func(g Gomega) {
				tUser, err := teleportClient.GetUser(userName, false)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(tUser.GetMetadata().Labels).Should(HaveKeyWithValue(types.OriginLabel, types.OriginCloud /* types.OriginKubernetes */))

			}).Should(Succeed())
		})

		When("the user is deleted", func() {
			BeforeEach(func() {
				Eventually(func(g Gomega) {
					var r resourcesv2.User
					err := k8sClient.Get(ctx, client.ObjectKey{
						Namespace: ns.Name,
						Name:      userName,
					}, &r)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(r.Finalizers).To(ContainElement(DeletionFinalizer))

				}).Should(Succeed())

				kDeleteUser(ctx, userName, ns.Name)
			})

			It("deletes the user in Teleport", func() {
				Eventually(func(g Gomega) {
					_, err := teleportClient.GetUser(userName, false)
					g.Expect(trace.IsNotFound(err)).To(BeTrue())

				}).Should(Succeed())
			})
		})
	})

	Context("the user exists in Teleport", func() {
		ctx := context.Background()
		ns := &core.Namespace{}
		var userName string

		BeforeEach(func() {
			ns = createNamespaceForTest(ctx)
			userName = validRandomResourceName("user-")

			tUser, err := types.NewUser(userName)
			Expect(err).ShouldNot(HaveOccurred())

			tUser.SetRoles([]string{"c"})

			err = teleportClient.CreateUser(ctx, tUser)
			Expect(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			deleteNamespaceForTest(ctx, ns)
		})

		It("doesn't exist in K8S", func() {
			var r resourcesv2.User
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: ns.Name,
				Name:      userName,
			}, &r)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		When("the user is created in k8s", func() {
			BeforeEach(func() {
				kUser := resourcesv2.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: ns.Name,
					},
					Spec: resourcesv2.UserSpec{
						Roles: []string{"x", "z"},
					},
				}
				kCreateUser(ctx, &kUser)
			})

			It("updates the user in Teleport", func() {
				Eventually(func(g Gomega) {
					tUser, err := teleportClient.GetUser(userName, false)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(tUser.GetRoles()).Should(ContainElements("x", "z"))

				}).Should(Succeed())
			})

			It("does not set the user OriginLabel", func() {
				Eventually(func(g Gomega) {
					user, err := teleportClient.GetUser(userName, false)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(user.GetMetadata().Labels).ShouldNot(HaveKeyWithValue(types.OriginLabel, types.OriginCloud /* types.OriginKubernetes */))

				}).Should(Succeed())
			})

			When("updating the user in k8s", func() {
				BeforeEach(func() {
					Eventually(func(g Gomega) {
						var kUserNewVersion resourcesv2.User
						err := k8sClient.Get(ctx, client.ObjectKey{
							Namespace: ns.Name,
							Name:      userName,
						}, &kUserNewVersion)
						g.Expect(err).ShouldNot(HaveOccurred())

						kUserNewVersion.Spec.Roles = append(kUserNewVersion.Spec.Roles, "admin", "root")
						err = k8sClient.Update(ctx, &kUserNewVersion)
						g.Expect(err).ShouldNot(HaveOccurred())
					}).Should(Succeed())
				})

				It("updates the user in Teleport", func() {
					Eventually(func(g Gomega) {
						tUser, err := teleportClient.GetUser(userName, false)
						g.Expect(err).ShouldNot(HaveOccurred())
						g.Expect(tUser.GetRoles()).Should(ContainElements("x", "z", "admin", "root"))

					}).Should(Succeed())
				})
			})
		})
	})
})

func kCreateDummyUser(ctx context.Context, namespace, userName string) {
	user := resourcesv2.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: namespace,
		},
		Spec: resourcesv2.UserSpec{
			Roles: []string{"a", "b"},
		},
	}
	kCreateUser(ctx, &user)
}

func kDeleteUser(ctx context.Context, userName, namespace string) {
	user := resourcesv2.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: namespace,
		},
	}
	err := k8sClient.Delete(ctx, &user)
	Expect(err).ShouldNot(HaveOccurred())
}

func kCreateUser(ctx context.Context, user *resourcesv2.User) {
	err := k8sClient.Create(ctx, user)
	Expect(err).ShouldNot(HaveOccurred())
}
