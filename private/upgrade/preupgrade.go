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

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/secret"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/upgrade/preupgrade"
)

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete

func createPreupgradeJob(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (ret ctrl.Result, err error) {
	var activeDeadlineSeconds int64 = 60
	var completions int32 = 1

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: preupgradeJobname(p),
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Completions:           &completions,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "preprocess",
							Image:   operatorImage,
							Command: []string{"/helper"},
							Env: []v1.EnvVar{
								{
									Name:  "DBHOST",
									Value: p.Name,
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
									Name:  helperModeEnvVar,
									Value: preupgrade.ModeString,
								},
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("10m"),
									v1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
							SecurityContext: statefulset.SecurityContext,
						},
					},
					RestartPolicy:   v1.RestartPolicyOnFailure,
					SecurityContext: statefulset.PodSecurityContext,
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

	return
}

func checkPreupgradeJob(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (ready bool, ret ctrl.Result, err error) {
	job := &batchv1.Job{}
	jobname := preupgradeJobname(p)

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: jobname}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		ret, err = createPreupgradeJob(ctx, p)

		return
	}

	if job.Status.Succeeded+job.Status.Failed > 0 {
		if job.Status.Succeeded > 0 {
			ready = true
		} else {
			err = ctx.Delete(ctx, job)
			ret.Requeue = true
		}
	}

	return
}

func preupgradeJobname(p *v1alpha1.PatroniPostgres) string {
	return fmt.Sprintf("%s-preupgrade", p.Name)
}
