/*
Copyright 2021 Gravitational, Inc.

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

// UserReconciler reconciles a User object
type UserReconciler struct {
	reconcilerBase
}

// UserObject is an object that can be converted into Teleport user object.
type UserObject interface {
	kclient.Object

	// ToTeleportUser returns a Teleport user object.
	ToTeleportUser() types.User
}

// NewUserReconciler builds a User resource controller.
func NewUserReconciler(client kclient.Client, scheme *runtime.Scheme) ReconcilerImpl {
	base := reconcilerBase{client: client, scheme: scheme, typeObj: &resources.UserV2{}}
	return UserReconciler{reconcilerBase: base}
}

//+kubebuilder:rbac:groups=resources.teleport.dev,resources=users,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=resources.teleport.dev,resources=users/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=resources.teleport.dev,resources=users/finalizers,verbs=update

// Do upserts the Teleport user based on the Kubernetes object state.
func (r UserReconciler) Do(ctx context.Context, client *client.Client, obj ResourceObject, op ResourceOp) error {
	log := log.FromContext(ctx)
	userObj := obj.(UserObject)
	switch op {
	case ResourceOpPut:
		user := userObj.ToTeleportUser()
		existingUser, err := client.GetUser(user.GetName(), false)
		if err != nil && !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		exists := (err == nil)

		if exists {
			user.SetCreatedBy(existingUser.GetCreatedBy())

			log.Info("updating a user", "name", user.GetName())
			if err := client.UpdateUser(ctx, user); err != nil {
				return trace.Wrap(err)
			}
		} else {
			log.Info("creating a user", "name", user.GetName())
			if err := client.CreateUser(ctx, user); err != nil {
				return trace.Wrap(err)
			}
		}

		return nil
	case ResourceOpDelete:
		name := userObj.GetName()
		log.Info("deleting a user", "name", name)
		if err := client.DeleteUser(ctx, name); err != nil {
			if trace.IsNotFound(err) {
				log.Info("User is not found in Teleport", "name", name)
				return nil
			}
			return trace.Wrap(err)
		}
	default:
		return trace.Errorf("unknown op %v", op)
	}

	return nil
}
