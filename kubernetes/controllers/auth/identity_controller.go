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

package auth

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	klog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	authv8 "github.com/gravitational/teleport-plugins/kubernetes/apis/auth/v8"
)

const IdentityKey = "identity"

// IdentityReconciler handles Identity object events and generates the identity secret object containing a credentials.
type IdentityReconciler struct {
	// Kube is a Kubernetes client.
	Kube kclient.Client

	// Scheme is a Kubernetes scheme
	Scheme *kruntime.Scheme

	// Signer is a credentials generator.
	Signer Signer

	// OwnerRefEnabled indicates should we assign an owner reference to generated secret objects.
	OwnerRefEnabled bool

	// RefreshRate is a rate at which to re-check the CA rotation.
	RefreshRate time.Duration
}

// Signer is a Teleport client used to generate identities and load cert authorities.
type Signer interface {
	// Generate and sign the identity file for a specific user.
	SignToString(ctx context.Context, username string, TTL time.Duration) (string, error)

	// Load the list of cert authorities.
	GetCAs(ctx context.Context) ([]types.CertAuthority, error)
}

//+kubebuilder:rbac:groups=auth.teleport.dev,resources=identities,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=auth.teleport.dev,resources=identities/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=auth.teleport.dev,resources=identities/finalizers,verbs=update
//+kubebuilder:rbac:resources=secrets,verbs=get;create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r IdentityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := klog.FromContext(ctx)

	namespace := req.Namespace

	var identity authv8.Identity
	if err := r.Kube.Get(ctx, req.NamespacedName, &identity); err != nil {
		return ctrl.Result{}, trace.Wrap(err)
	}

	username := identity.Spec.Username

	log.Info("reconciling an identity", "user", username)

	var ttl time.Duration
	if identity.Spec.TTL != nil {
		ttl = identity.Spec.TTL.Duration
	}
	secretData, err := r.Signer.SignToString(ctx, username, ttl)
	if err != nil {
		return ctrl.Result{}, trace.Wrap(err)
	}

	secretName := identity.Spec.SecretName

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}
	result, err := controllerutil.CreateOrPatch(ctx, r.Kube, &secret, func() error {
		secret.StringData = map[string]string{IdentityKey: secretData}
		if !r.OwnerRefEnabled {
			return nil
		}
		ownerRefs := secret.GetOwnerReferences()
		for _, ref := range ownerRefs {
			if ref.UID == identity.UID {
				return nil
			}
		}
		secret.SetOwnerReferences(append(ownerRefs, *metav1.NewControllerRef(&identity, authv8.GroupVersion.WithKind("Identity"))))

		return nil
	})
	if err != nil {
		return ctrl.Result{}, trace.Wrap(err)
	}

	log.Info("identity secret written", "result", result, "identity", identity.Name, "secret", secretName)

	if identity.Status.NeedRenewal {
		patch := kclient.MergeFrom(identity.DeepCopy())
		identity.Status.NeedRenewal = false
		if err := r.Kube.Status().Patch(ctx, &identity, patch); err != nil {
			return ctrl.Result{}, trace.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r IdentityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicates := []predicate.Predicate{
		predicate.Funcs{
			DeleteFunc: func(_ event.DeleteEvent) bool {
				// We don't handle a deletion.
				return false
			},
		},
		predicate.Or(
			// Handle updates if the spec field has been changed.
			predicate.GenerationChangedPredicate{},
			// Or if the object is marked for renewal.
			predicate.NewPredicateFuncs(func(object kclient.Object) bool {
				identity, ok := object.(*authv8.Identity)
				return ok && identity.Status.NeedRenewal
			}),
		),
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&authv8.Identity{}, builder.WithPredicates(predicates...)).
		Complete(r); err != nil {
		return trace.Wrap(err)
	}

	if err := (identitySecretReconciler{kube: r.Kube, signer: r.Signer, refreshRate: r.RefreshRate}).SetupWithManager(mgr); err != nil {
		return trace.Wrap(err)
	}

	return nil
}
