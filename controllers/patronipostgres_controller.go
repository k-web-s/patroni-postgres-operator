/*
Copyright 2023 Richard Kojedzinszky

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

  1. Redistributions of source code must retain the above copyright notice, this
     list of conditions and the following disclaimer.

  2. Redistributions in binary form must reproduce the above copyright notice,
     this list of conditions and the following disclaimer in the documentation
     and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS “AS IS”
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/networkpolicy"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/pdb"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/pvc"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/rbac"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/secret"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/service"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/upgrade"
)

// PatroniPostgresReconciler reconciles a PatroniPostgres object
type PatroniPostgresReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset *kubernetes.Clientset
}

type reconcilerFunc func(pcontext.Context, *v1alpha1.PatroniPostgres) error

//+kubebuilder:rbac:groups=kwebs.cloud,resources=patronipostgres,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kwebs.cloud,resources=patronipostgres/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kwebs.cloud,resources=patronipostgres/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PatroniPostgresReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ret ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	// Fetch PatroniPostgres instance
	instance := &v1alpha1.PatroniPostgres{}
	err = r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return
	}

	// Ignore on request
	if instance.Spec.Ignore {
		logger.Info("spec.ignore set")
		return
	}

	wctx := pcontext.New(ctx, r.Client, r.Clientset, instance)
	defer func() {
		if err == nil {
			err = wctx.Status().Update(wctx, instance)
		}

		logger.Info("reconciling done")
	}()

	logger.Info("reconciling")

	// initialization
	if instance.Status.State == "" {
		instance.Status.Version = instance.Spec.Version
		instance.Status.State = v1alpha1.PatroniPostgresStateScaling
		instance.Status.VolumeStatuses = make([]v1alpha1.VolumeStatus, 0)

		ret.Requeue = true
		return
	}

	// handle upgrade
	if instance.Status.UpgradeVersion != 0 {
		return upgrade.Handle(wctx, instance)
	}

	if instance.Status.State == v1alpha1.PatroniPostgresStateReady {
		if instance.Status.Version < instance.Spec.Version {
			if _, err = configmap.GetSyncLeader(wctx, instance); err != nil {
				return
			}

			instance.Status.UpgradeVersion = instance.Spec.Version

			ret.Requeue = true
			return
		}
	}

	for _, f := range []reconcilerFunc{
		pvc.Reconcile,
		secret.Reconcile,
		configmap.Reconcile,
		rbac.Reconcile,
		statefulset.Reconcile,
		service.Reconcile,
		networkpolicy.Reconcile,
		pdb.Reconcile,
	} {
		if err = f(wctx, instance); err != nil {
			return
		}
	}

	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *PatroniPostgresReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// dont watch for creates or updates with no generation changes
	watchPredicates := predicate.Not(
		predicate.Or(
			predicate.Funcs{
				DeleteFunc: func(de event.DeleteEvent) bool {
					return false
				},
				UpdateFunc: func(ue event.UpdateEvent) bool {
					return false
				},
				GenericFunc: func(ge event.GenericEvent) bool {
					return false
				},
			},
			predicate.GenerationChangedPredicate{},
		),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PatroniPostgres{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.PersistentVolumeClaim{}, handler.EnqueueRequestForOwner(r.Scheme, r.RESTMapper(), &v1alpha1.PatroniPostgres{}),
			builder.WithPredicates(watchPredicates)).
		Watches(&appsv1.StatefulSet{}, handler.EnqueueRequestForOwner(r.Scheme, r.RESTMapper(), &v1alpha1.PatroniPostgres{}),
			builder.WithPredicates(watchPredicates)).
		Watches(&batchv1.Job{}, handler.EnqueueRequestForOwner(r.Scheme, r.RESTMapper(), &v1alpha1.PatroniPostgres{}),
			builder.WithPredicates(watchPredicates)).
		Complete(r)
}
