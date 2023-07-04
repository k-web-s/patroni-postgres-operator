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

package service

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	"github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
)

const (
	PatroniPodRoleKey     = "role"
	PatroniPodRole_Master = "master"
)

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	serviceName := p.Name
	service := &corev1.Service{}
	var create bool

	// main service
	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: serviceName}, service)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		service = &corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name: serviceName,
			},
		}

		create = true
	}

	if err = ctx.SetMeta(service); err != nil {
		return
	}

	service.Spec.Type = p.Spec.ServiceType
	service.Spec.Selector = ctx.CommonLabels()
	service.Spec.Selector[PatroniPodRoleKey] = PatroniPodRole_Master

	service.Spec.Ports = []corev1.ServicePort{
		{
			Name:       statefulset.PostgresPortName,
			Port:       statefulset.PostgresPort,
			TargetPort: intstr.FromInt(statefulset.PostgresPort),
		},
	}

	if create {
		err = ctx.Create(ctx, service)
	} else {
		err = ctx.Update(ctx, service)
	}

	if err != nil {
		return
	}

	// headless service
	serviceName = fmt.Sprintf("%s-headless", p.Name)
	create = false

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: serviceName}, service)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		service = &corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name: serviceName,
			},
		}

		create = true
	}

	if err = ctx.SetMeta(service); err != nil {
		return
	}

	service.Spec.ClusterIP = corev1.ClusterIPNone
	service.Spec.Selector = ctx.CommonLabels()
	service.Spec.Selector[PatroniPodRoleKey] = PatroniPodRole_Master

	if create {
		err = ctx.Create(ctx, service)
	} else {
		err = ctx.Update(ctx, service)
	}

	return
}
