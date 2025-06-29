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

package networkpolicy

import (
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	"github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/service"
)

// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;create;update

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	policyName := p.Name
	policy := &networking.NetworkPolicy{}
	var create bool

	// default policy
	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: policyName}, policy)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		policy = &networking.NetworkPolicy{
			ObjectMeta: v1.ObjectMeta{
				Name: policyName,
			},
		}

		create = true
	}

	if err = ctx.SetMeta(policy); err != nil {
		return
	}

	port := intstr.FromInt(service.PostgresPort)
	policy.Spec = networking.NetworkPolicySpec{
		PodSelector: v1.LabelSelector{
			MatchLabels: ctx.CommonLabels(),
		},
		Ingress: []networking.NetworkPolicyIngressRule{
			{
				// Allow everything between cluster pods
				From: []networking.NetworkPolicyPeer{
					{
						PodSelector: &v1.LabelSelector{
							MatchLabels: ctx.CommonLabels(),
						},
					},
				},
			},
			{
				// PostgreSQL service
				From: p.Spec.AccessControl,
				Ports: []networking.NetworkPolicyPort{
					{
						Port: &port,
					},
				},
			},
		},
	}
	policy.Spec.Ingress = append(policy.Spec.Ingress, p.Spec.AdditionalNetworkPolicyIngress...)

	if create {
		err = ctx.Create(ctx, policy)
	} else {
		err = ctx.Update(ctx, policy)
	}

	return
}
