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

package pdb

import (
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	context "github.com/k-web-s/patroni-postgres-operator/private/context"
)

var (
	maxUnavailable = intstr.FromInt(1)
)

// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;create;update

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	policyName := p.Name
	pdb := &policyv1.PodDisruptionBudget{}
	var create bool

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: policyName}, pdb)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		pdb = &policyv1.PodDisruptionBudget{
			ObjectMeta: v1.ObjectMeta{
				Name: policyName,
			},
		}

		create = true
	}

	if err = ctx.SetMeta(pdb); err != nil {
		return
	}

	pdb.Spec = policyv1.PodDisruptionBudgetSpec{
		MaxUnavailable: &maxUnavailable,
		Selector: &v1.LabelSelector{
			MatchLabels: ctx.CommonLabels(),
		},
	}
	if create {
		err = ctx.Create(ctx, pdb)
	} else {
		err = ctx.Update(ctx, pdb)
	}

	return
}
