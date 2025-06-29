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
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/secret"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/security"
)

const (
	upgradeModeEnvVar = "MODE"
)

type UpgradeJob interface {
	Mode() string
	ActiveDeadlineSeconds() int64
	DBPort() int
	CustomizePodSpec(*v1.PodSpec)
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create;delete

func ensureUpgradeJob(ctx pcontext.Context, p *v1alpha1.PatroniPostgres, j UpgradeJob) (job *batchv1.Job, err error) {
	jobname := upgradeJobname(p, j)
	job = &batchv1.Job{}

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: jobname}, job)
	if err == nil || !errors.IsNotFound(err) {
		return
	}

	activeDeadlineSeconds := j.ActiveDeadlineSeconds()
	enableServiceLinks := false

	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: upgradeJobname(p, j),
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ctx.CommonLabels(),
				},
				Spec: v1.PodSpec{
					EnableServiceLinks: &enableServiceLinks,
					Containers: []v1.Container{
						{
							Name:    j.Mode(),
							Image:   *upgradeImage,
							Command: []string{"/upgrade"},
							Env: []v1.EnvVar{
								{
									Name:  "DBHOST",
									Value: p.Name,
								},
								{
									Name:  "DBPORT",
									Value: fmt.Sprintf("%d", j.DBPort()),
								},
								{
									Name:  "DBUSER",
									Value: statefulset.PatroniSuperuserUsername,
								},
								{
									Name: "DBPASSWORD",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: v1.LocalObjectReference{
												Name: secret.Name(p),
											},
											Key: secret.SuperUserPasswordKey,
										},
									},
								},
								{
									Name:  "DBNAME",
									Value: "postgres",
								},
								{
									Name:  upgradeModeEnvVar,
									Value: j.Mode(),
								},
								{
									Name:  "CLUSTER_NAME",
									Value: p.Name,
								},
								{
									Name:  "CLUSTER_SIZE",
									Value: fmt.Sprintf("%d", len(p.Status.VolumeStatuses)),
								},
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("10m"),
									v1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
							SecurityContext: security.ContainerSecurityContext,
						},
					},
					RestartPolicy:   v1.RestartPolicyNever,
					SecurityContext: security.GenericPodSecurityContext,
				},
			},
		},
	}

	j.CustomizePodSpec(&job.Spec.Template.Spec)

	if err = ctx.SetMeta(job); err != nil {
		return
	}

	if err = ctx.Create(ctx, job); err != nil {
		return
	}

	return
}

// cleanupJob removes job if succeeded or failed (i.e. after a pod exited)
func cleanupJob(ctx pcontext.Context, job *batchv1.Job) (err error) {
	if job.Status.Succeeded+job.Status.Failed > 0 {
		deletePropagationPolicy := metav1.DeletePropagationBackground

		err = ctx.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &deletePropagationPolicy})
	}

	return
}

func upgradeJobname(p *v1alpha1.PatroniPostgres, j UpgradeJob) string {
	return fmt.Sprintf("%s-%s", p.Name, j.Mode())
}
