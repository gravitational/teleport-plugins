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

	resourcesv5 "github.com/gravitational/teleport-plugins/kubernetes/apis/resources/v5"
)

var _ = Describe("Roles", func() {
	Context("a new role is created in k8s", func() {
		ctx := context.Background()
		ns := &core.Namespace{}
		var roleName string

		BeforeEach(func() {
			ns = createNamespaceForTest(ctx)
			roleName = validRandomResourceName("role-")
			kCreateDummyRole(ctx, ns.Name, roleName)
		})

		AfterEach(func() {
			deleteNamespaceForTest(ctx, ns)
		})

		It("creates the role in Teleport", func() {
			Eventually(func(g Gomega) {
				tRole, err := teleportClient.GetRole(ctx, roleName)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(tRole.GetName()).Should(Equal(roleName))

			}).Should(Succeed())
		})

		It("sets the OriginLabel to kubernetes", func() {
			Eventually(func(g Gomega) {
				tRole, err := teleportClient.GetRole(ctx, roleName)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(tRole.GetMetadata().Labels).Should(HaveKeyWithValue(types.OriginLabel, types.OriginCloud /* types.OriginKubernetes */))

			}).Should(Succeed())
		})

		When("the role is deleted", func() {
			BeforeEach(func() {
				Eventually(func(g Gomega) {
					var r resourcesv5.Role
					err := k8sClient.Get(ctx, client.ObjectKey{
						Namespace: ns.Name,
						Name:      roleName,
					}, &r)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(r.Finalizers).To(ContainElement(DeletionFinalizer))

				}).Should(Succeed())

				kDeleteRole(ctx, roleName, ns.Name)
			})

			It("deletes the role in Teleport", func() {
				Eventually(func(g Gomega) {
					_, err := teleportClient.GetRole(ctx, roleName)
					g.Expect(trace.IsNotFound(err)).To(BeTrue())

				}).Should(Succeed())
			})
		})
	})

	Context("a role exists in Teleport", func() {
		ctx := context.Background()
		ns := &core.Namespace{}
		var roleName string

		BeforeEach(func() {
			ns = createNamespaceForTest(ctx)
			roleName = validRandomResourceName("role-")

			tRole, err := types.NewRoleV5(roleName, types.RoleSpecV5{
				Allow: types.RoleConditions{
					Logins: []string{"c"},
				},
			})
			Expect(err).ShouldNot(HaveOccurred())

			err = teleportClient.UpsertRole(ctx, tRole)
			Expect(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			deleteNamespaceForTest(ctx, ns)
		})

		It("doesn't exist in K8S", func() {
			var r resourcesv5.Role
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: ns.Name,
				Name:      roleName,
			}, &r)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		When("the role is created in k8s", func() {
			BeforeEach(func() {
				kRole := resourcesv5.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      roleName,
						Namespace: ns.Name,
					},
					Spec: resourcesv5.RoleSpec{
						Allow: types.RoleConditions{
							Logins: []string{"x", "z"},
						},
					},
				}
				kCreateRole(ctx, &kRole)
			})

			It("updates the role in Teleport", func() {
				Eventually(func(g Gomega) {
					tRole, err := teleportClient.GetRole(ctx, roleName)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(tRole.GetLogins(types.Allow)).Should(ContainElements("x", "z"))

				}).Should(Succeed())
			})

			It("does not set the role OriginLabel", func() {
				Eventually(func(g Gomega) {
					role, err := teleportClient.GetRole(ctx, roleName)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(role.GetMetadata().Labels).ShouldNot(HaveKeyWithValue(types.OriginLabel, types.OriginCloud /* types.OriginKubernetes */))

				}).Should(Succeed())
			})

			When("updating the role in k8s", func() {
				BeforeEach(func() {
					Eventually(func(g Gomega) {
						var kRoleNewVersion resourcesv5.Role
						err := k8sClient.Get(ctx, client.ObjectKey{
							Namespace: ns.Name,
							Name:      roleName,
						}, &kRoleNewVersion)
						g.Expect(err).ShouldNot(HaveOccurred())

						kRoleNewVersion.Spec.Allow.Logins = append(kRoleNewVersion.Spec.Allow.Logins, "admin", "root")
						err = k8sClient.Update(ctx, &kRoleNewVersion)
						g.Expect(err).ShouldNot(HaveOccurred())
					}).Should(Succeed())
				})

				It("updates the role in Teleport", func() {
					Eventually(func(g Gomega) {
						tRole, err := teleportClient.GetRole(ctx, roleName)
						g.Expect(err).ShouldNot(HaveOccurred())
						g.Expect(tRole.GetLogins(types.Allow)).Should(ContainElements("x", "z", "admin", "root"))

					}).Should(Succeed())
				})
			})
		})
	})
})

func kCreateDummyRole(ctx context.Context, namespace, roleName string) {
	role := resourcesv5.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
		Spec: resourcesv5.RoleSpec{
			Allow: types.RoleConditions{
				Logins: []string{"a", "b"},
			},
		},
	}
	kCreateRole(ctx, &role)
}

func kDeleteRole(ctx context.Context, roleName, namespace string) {
	role := resourcesv5.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
	}
	err := k8sClient.Delete(ctx, &role)
	Expect(err).ShouldNot(HaveOccurred())
}

func kCreateRole(ctx context.Context, role *resourcesv5.Role) {
	err := k8sClient.Create(ctx, role)
	Expect(err).ShouldNot(HaveOccurred())
}
