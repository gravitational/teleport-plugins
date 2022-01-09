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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gravitational/teleport-plugins/lib/stringset"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"
)

// DeletionFinalizer is a name of finalizer added to resource's 'finalizers' field
// for tracking deletion events.
const DeletionFinalizer = "resources.teleport.dev/deletion"

// ResourceObject is an object being reconciled e.g. User or Role.
type ResourceObject interface {
	kclient.Object
	SetErrorStatus(error)
}

// ResourceOp is a resource operation needed to perform in Teleport.
type ResourceOp int

const (
	ResourceOpInvalid ResourceOp = iota - 1
	ResourceOpPut
	ResourceOpDelete
)

// ReconcilerImpl is an implementation of resource controller.
type ReconcilerImpl interface {
	GetClient() kclient.Client
	GetScheme() *runtime.Scheme
	GetType() kclient.Object
	Do(context.Context, *client.Client, ResourceObject, ResourceOp) error
}

// Reconciler is a base wrapper of a resource controller. It tracks errors
type Reconciler struct {
	ReconcilerImpl
	Client *client.Client
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	name := req.NamespacedName

	object := r.GetType().DeepCopyObject().(ResourceObject)
	kube := r.GetClient()
	if err := kube.Get(ctx, name, object); err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "failed to reconcile the non-existing resource", "resource", name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, trace.Wrap(err)
	}

	finalizers := stringset.New(object.GetFinalizers()...)
	var op ResourceOp
	if object.GetDeletionTimestamp().IsZero() {
		// Resource is created or updated.
		op = ResourceOpPut

		// Update a resource with a finalizer if it doesn't exist.
		if !finalizers.Contains(DeletionFinalizer) {
			patch := kclient.MergeFrom(object.DeepCopyObject().(ResourceObject))
			controllerutil.AddFinalizer(object, DeletionFinalizer)
			if err := kube.Patch(ctx, object, patch); err != nil {
				return ctrl.Result{}, trace.Wrap(err)
			}
		}
	} else {
		// Resource is being deleted.
		op = ResourceOpDelete

		// Skip if there's no finalizer set by us.
		if !finalizers.Contains(DeletionFinalizer) {
			return ctrl.Result{}, nil
		}
	}

	patch := kclient.MergeFrom(object.DeepCopyObject().(ResourceObject))

	doErr := trace.Wrap(r.Do(ctx, r.Client, object, op))
	if doErr != nil {
		log.Error(doErr, "failed to reconcile resource")
	}
	object.SetErrorStatus(doErr)
	if op == ResourceOpDelete && doErr == nil {
		controllerutil.RemoveFinalizer(object, DeletionFinalizer)
	}
	err := trace.Wrap(kube.Patch(ctx, object, patch))

	return ctrl.Result{}, trace.NewAggregate(doErr, err)
}

// SetupWithManager sets up the controller with the Manager.
func (r Reconciler) SetupWithManager(mgr manager.Manager) error {
	predicates := []predicate.Predicate{
		predicate.Funcs{
			DeleteFunc: func(_ event.DeleteEvent) bool {
				// We skip deletion events cause they don't carry any useful info.
				return false
			},
		},
		predicate.GenerationChangedPredicate{}, // Handle updates if only the spec field is changed.
	}
	ctrlOptions := controller.Options{
		MaxConcurrentReconciles: 1, // It's better for us not to be racy.
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(ctrlOptions).
		For(r.GetType(), builder.WithPredicates(predicates...)).
		Complete(r)
}
