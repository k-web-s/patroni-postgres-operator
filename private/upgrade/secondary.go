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

package upgrade

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
)

const (
	rsyncPort     = 5873
	rsyncPortName = "rsync"

	rsyncImage = "ghcr.io/rkojedzinszky/webhost-images/static"
)

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;delete

func upgradeSecondaries(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (ret ctrl.Result, err error) {
	// shortcut if handling one-member cluster
	if len(p.Status.VolumeStatuses) == 1 {
		p.Status.State = v1alpha1.PatroniPostgresStateUpgradePostupgrade

		ret.Requeue = true

		return
	}

	var leader int
	leader, err = configmap.GetSyncLeader(ctx, p)
	if err != nil {
		return
	}

	var sts *appsv1.StatefulSet
	if sts, err = upgradeSecondariesEnsurestreamer(ctx, p, leader); err != nil {
		return
	}

	var jobs []*batchv1.Job
	if jobs, err = upgradeSecondariesEnsureseclients(ctx, p, leader, sts); err != nil {
		return
	}

	succeeded := 0
	for _, job := range jobs {
		if job.Status.Succeeded > 0 {
			succeeded += 1
		} else if job.Status.Failed > 0 {
			propagationPolicy := metav1.DeletePropagationBackground
			if err = ctx.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagationPolicy}); err != nil {
				return
			}

			ret.Requeue = true
		}
	}

	if succeeded == len(jobs) {
		propagationPolicy := metav1.DeletePropagationBackground
		if err = ctx.Delete(ctx, sts, &client.DeleteOptions{PropagationPolicy: &propagationPolicy}); err != nil {
			return
		}

		for _, job := range jobs {
			if err = ctx.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagationPolicy}); err != nil {
				return
			}
		}

		p.Status.State = v1alpha1.PatroniPostgresStateUpgradePostupgrade

		ret.Requeue = true
	}

	return
}
