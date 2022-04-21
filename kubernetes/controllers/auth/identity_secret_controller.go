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

package auth

import (
	"bytes"
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	klog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	authv10 "github.com/gravitational/teleport-plugins/kubernetes/apis/auth/v10"
	"github.com/gravitational/teleport-plugins/lib/certs"
	"github.com/gravitational/teleport/api/identityfile"
	"github.com/gravitational/teleport/api/types"

	"github.com/gravitational/trace"
)

// identitySecretReconciler tracks the identity secret lifetime.
// Once it's needed to re-generate a secret, the identity reconciler will be informed.
type identitySecretReconciler struct {
	kube        kclient.Client
	signer      Signer
	refreshRate time.Duration
}

//+kubebuilder:rbac:groups=auth.teleport.dev,resources=identities,verbs=list;patch
//+kubebuilder:rbac:resources=secrets,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r identitySecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := klog.FromContext(ctx)

	namespace, secretName := req.Namespace, req.Name

	var identityList authv10.IdentityList
	if err := r.kube.List(ctx, &identityList,
		kclient.InNamespace(namespace),
		kclient.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.secretName", secretName)},
	); err != nil {
		return ctrl.Result{}, trace.Wrap(err)
	}
	if len(identityList.Items) == 0 {
		return ctrl.Result{}, nil
	}

	var secret corev1.Secret
	if err := r.kube.Get(ctx, req.NamespacedName, &secret); err != nil {
		return ctrl.Result{}, trace.Wrap(err)
	}

	isValid, validityDuration, err := r.verifyIdentity(ctx, string(secret.Data[IdentityKey]))
	if err != nil {
		log.Error(err, "error has occurred while checking the identity")
	}

	requeueAfter := validityDuration
	if r.refreshRate < requeueAfter {
		requeueAfter = r.refreshRate
	}

	if isValid {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	log.Info("reconciling identity secret...", "secret", secretName)

	for _, identity := range identityList.Items {
		if identity.Status.NeedRenewal {
			continue
		}
		patch := kclient.MergeFrom(identity.DeepCopy())
		identity.Status.NeedRenewal = true
		if err := r.kube.Status().Patch(ctx, &identity, patch); err != nil {
			return ctrl.Result{}, trace.Wrap(err)
		}
		log.Info("identity marked for renewal", "identity", identity.Name, "secret", secretName)
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r identitySecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicates := []predicate.Predicate{predicate.Funcs{
		UpdateFunc: func(evt event.UpdateEvent) bool {
			secretNew, ok := evt.ObjectNew.(*corev1.Secret)
			if !ok {
				return false
			}
			secretOld, ok := evt.ObjectOld.(*corev1.Secret)
			if !ok {
				return false
			}
			return !bytes.Equal(secretNew.Data[IdentityKey], secretOld.Data[IdentityKey])
		},
	}}
	return trace.Wrap(ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(predicates...)).
		Complete(r),
	)
}

// verifyIdentity checks the identity file contents against the current CA key sets.
func (r identitySecretReconciler) verifyIdentity(ctx context.Context, identityStr string) (isValid bool, validity time.Duration, err error) {
	log := klog.FromContext(ctx)
	identityFile, err := identityfile.FromString(identityStr)
	if err != nil {
		return false, 0, trace.Wrap(err, "failed to parse the identity contents, maybe the secret data has been changed or deleted")
	}

	identityCerts, err := certs.ParseIdentity(identityFile)
	if err != nil {
		return false, 0, trace.Wrap(err, "failed to decode the identity certificates")
	}

	authorities, err := r.signer.GetCAs(ctx)
	if err != nil {
		return false, 0, trace.Wrap(err, "failed to load CA set")
	}

	cas, err := certs.ParseCAs(authorities)
	if err != nil {
		return false, 0, trace.Wrap(err, "failed to parse CA set")
	}

	keySet, err := cas.GetKeys(types.UserCA)
	if err != nil {
		return false, 0, trace.Wrap(err)
	}

	res, err := keySet.VerifyCerts(identityCerts)
	if err != nil {
		return false, 0, trace.Wrap(err)
	}

	isValid, validity = (res.KeySet == keySet.Active), res.Validity
	if !isValid {
		log.Info("the identity certificates seem to be signed with an older CA key set being rotated now")
	}

	return isValid, validity, nil
}
