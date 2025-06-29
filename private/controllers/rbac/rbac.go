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

package rbac

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	"github.com/k-web-s/patroni-postgres-operator/private/context"
)

// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;create;update

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	if err = reconcileServiceAccount(ctx, p); err != nil {
		return
	}

	if err = reconcileRole(ctx, p); err != nil {
		return
	}

	if err = reconcileRoleBinding(ctx, p); err != nil {
		return
	}

	return
}

// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;create;update

func reconcileServiceAccount(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	serviceAccount := &corev1.ServiceAccount{}

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ServiceAccountName(p)}, serviceAccount)
	if err == nil {
		if len(serviceAccount.OwnerReferences) == 0 {
			if err = ctx.SetMeta(serviceAccount); err != nil {
				return
			}

			err = ctx.Update(ctx, serviceAccount)
		}
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		serviceAccount = &corev1.ServiceAccount{
			ObjectMeta: v1.ObjectMeta{
				Name: ServiceAccountName(p),
			},
		}

		if err = ctx.SetMeta(serviceAccount); err != nil {
			return
		}

		err = ctx.Create(ctx, serviceAccount)
	}

	return
}

// delegated permissions
// +kubebuilder:rbac:groups="",resources=configmaps;pods,verbs=get;list;watch;patch;update

func reconcileRole(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	role := &rbacv1.Role{}
	var create bool

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ServiceAccountName(p)}, role)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		role = &rbacv1.Role{
			ObjectMeta: v1.ObjectMeta{
				Name: ServiceAccountName(p),
			},
		}

		create = true
	}

	if err = ctx.SetMeta(role); err != nil {
		return
	}

	role.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps", "pods"},
			Verbs:     []string{"get", "list", "watch", "patch", "update"},
		},
	}

	if create {
		err = ctx.Create(ctx, role)
	} else {
		err = ctx.Update(ctx, role)
	}

	return err
}

func reconcileRoleBinding(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	role := &rbacv1.RoleBinding{}
	var create bool

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ServiceAccountName(p)}, role)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		role = &rbacv1.RoleBinding{
			ObjectMeta: v1.ObjectMeta{
				Name: ServiceAccountName(p),
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     ServiceAccountName(p),
			},
		}

		create = true
	}

	if err = ctx.SetMeta(role); err != nil {
		return
	}

	role.Subjects = []rbacv1.Subject{
		{
			Kind: rbacv1.ServiceAccountKind,
			Name: ServiceAccountName(p),
		},
	}

	if create {
		err = ctx.Create(ctx, role)
	} else {
		err = ctx.Update(ctx, role)
	}

	return err
}

func ServiceAccountName(p *v1alpha1.PatroniPostgres) string {
	return p.Name
}
