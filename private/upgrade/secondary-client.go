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
	_ "embed"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/pvc"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/security"
)

var (
	//go:embed upgrade-scripts/secondary-upgrade
	secondaryUpgrade string
)

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create

func upgradeSecondariesEnsureseclients(ctx pcontext.Context, p *v1alpha1.PatroniPostgres, leader int, sts *appsv1.StatefulSet) (ret []*batchv1.Job, err error) {
	latestCheckpointLocation, newDBSystemId, err := configmap.GetSecondaryStreamArgs(ctx, p)
	if err != nil {
		return
	}

	for idx := range p.Status.VolumeStatuses {
		if idx == leader {
			continue
		}

		job := &batchv1.Job{}
		jobname := fmt.Sprintf("%s-sup-%d", p.Name, idx)
		if err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: jobname}, job); err != nil {
			if !errors.IsNotFound(err) {
				return
			}

			var activeDeadlineSeconds int64 = 600

			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: jobname,
				},
				Spec: batchv1.JobSpec{
					ActiveDeadlineSeconds: &activeDeadlineSeconds,
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: ctx.CommonLabels(),
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:    "secondary-upgrade",
									Image:   ctx.Image().Image(),
									Command: []string{"sh", "-c", secondaryUpgrade},
									Env: []v1.EnvVar{
										{
											Name:  "PRIMARY_ADDRESS",
											Value: sts.Name,
										},
										{
											Name:  "DB_CHECKPOINT",
											Value: latestCheckpointLocation,
										},
										{
											Name:  "OLD",
											Value: fmt.Sprintf("%d", p.Status.Version),
										},
										{
											Name:  "NEW",
											Value: fmt.Sprintf("%d", p.Status.UpgradeVersion),
										},
										{
											Name:  "NEW_DB_SYSTEM_ID",
											Value: newDBSystemId,
										},
									},
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      pvc.VolumeName,
											MountPath: statefulset.DataVolumeMountPath,
										},
									},
									// only use requests
									Resources: v1.ResourceRequirements{
										Requests: p.Spec.Resources.Requests,
									},
									SecurityContext: security.ContainerSecurityContext,
								},
							},
							Volumes: []v1.Volume{
								{
									Name: pvc.VolumeName,
									VolumeSource: v1.VolumeSource{
										PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
											ClaimName: p.Status.VolumeStatuses[idx].ClaimName,
										},
									},
								},
							},
							RestartPolicy:    v1.RestartPolicyOnFailure,
							SecurityContext:  security.DatabasePodSecurityContext,
							ImagePullSecrets: p.Spec.ImagePullSecrets,
							NodeSelector:     p.Spec.NodeSelector,
							Tolerations:      p.Spec.Tolerations,
						},
					},
				},
			}

			if err = ctx.SetMeta(job); err != nil {
				return
			}

			if err = ctx.Create(ctx, job); err != nil {
				return
			}
		}

		ret = append(ret, job)
	}

	return
}
