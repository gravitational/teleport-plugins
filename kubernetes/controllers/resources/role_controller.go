/*
Copyright 2021-2022 Gravitational, Inc.

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

	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/gravitational/teleport-plugins/kubernetes/apis/resources"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// RoleReconciler reconciles a Role object
type RoleReconciler struct {
	reconcilerBase
}

// RoleObject is an object that can be converted into Teleport role object.
type RoleObject interface {
	kclient.Object

	// ToTeleportRole returns a Teleport role object.
	ToTeleportRole() types.Role
}

// NewRoleReconciler builds a Role resource controller.
func NewRoleReconciler(client kclient.Client, scheme *runtime.Scheme) ReconcilerImpl {
	base := reconcilerBase{client: client, scheme: scheme, typeObj: &resources.RoleV5{}}
	return RoleReconciler{reconcilerBase: base}
}

//+kubebuilder:rbac:groups=resources.teleport.dev,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=resources.teleport.dev,resources=roles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=resources.teleport.dev,resources=roles/finalizers,verbs=update

// Do upserts the Teleport role based on the Kubernetes object state.
func (r RoleReconciler) Do(ctx context.Context, client *client.Client, obj ResourceObject, op ResourceOp) error {
	log := log.FromContext(ctx)
	roleObj := obj.(RoleObject)
	switch op {
	case ResourceOpPut:
		role := roleObj.ToTeleportRole()
		log.Info("upserting a role", "name", role.GetName())
		if err := client.UpsertRole(ctx, role); err != nil {
			return trace.Wrap(err)
		}
		return nil
	case ResourceOpDelete:
		name := roleObj.GetName()
		log.Info("deleting a role", "name", name)
		if err := client.DeleteRole(ctx, name); err != nil {
			if trace.IsNotFound(err) {
				log.Info("Role is not found in Teleport", "name", name)
				return nil
			}
			return trace.Wrap(err)
		}
		return nil
	default:
		return trace.Errorf("unknown op %v", op)
	}
}
