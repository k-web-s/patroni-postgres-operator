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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/pvc"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/security"
	"github.com/k-web-s/patroni-postgres-operator/private/upgrade/preupgrade"
)

var (
	//go:embed upgrade-scripts/primary-upgrade
	primaryUpgrade string

	errPrimaryUpgradeJobFailed = fmt.Errorf("primary upgrade job failed")
)

type primaryUpgradeHandler struct {
}

func (primaryUpgradeHandler) name() v1alpha1.PatroniPostgresState {
	return v1alpha1.PatroniPostgresStateUpgradePrimary
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete

func (primaryUpgradeHandler) handle(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (done bool, err error) {
	job := &batchv1.Job{}
	jobname := fmt.Sprintf("%s-up", p.Name)

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: jobname}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		// preprocessJob must exist at this point
		preprocessJob := &batchv1.Job{}
		if err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: preupgradeJobname(p)}, preprocessJob); err != nil {
			return
		}

		var initdbArgs string
		if initdbArgs, err = getInitdbArgsFromJob(ctx, preprocessJob); err != nil {
			return
		}

		var leaderIndex int
		leaderIndex, err = configmap.GetSyncLeader(ctx, p)
		if err != nil {
			return
		}
		var dbid string
		dbid, err = configmap.GetDBId(ctx, p)
		if err != nil {
			return
		}

		var activeDeadlineSeconds int64 = 600
		var completions int32 = 1

		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: jobname,
			},
			Spec: batchv1.JobSpec{
				ActiveDeadlineSeconds: &activeDeadlineSeconds,
				Completions:           &completions,
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: ctx.CommonLabels(),
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:    "primary-upgrade",
								Image:   statefulset.Image,
								Command: []string{"sh", "-c", primaryUpgrade},
								Env: []v1.EnvVar{
									{
										Name:  "DB_SYSTEM_ID",
										Value: dbid,
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
										Name:  "INITDB_ARGS",
										Value: initdbArgs,
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
										ClaimName: p.Status.VolumeStatuses[leaderIndex].ClaimName,
									},
								},
							},
						},
						RestartPolicy:   v1.RestartPolicyOnFailure,
						SecurityContext: security.DatabasePodSecurityContext,
					},
				},
			},
		}

		if err = ctx.SetMeta(job); err != nil {
			return
		}

		err = ctx.Create(ctx, job)

		return
	}

	// handle success
	if job.Status.Succeeded > 0 {
		var dbid string
		if dbid, err = getDBIDFromJob(ctx, job); err != nil {
			return
		}

		if err = configmap.SetDBId(ctx, p, dbid); err != nil {
			return
		}

		// delete preprocessJob if still exists
		preprocessJob := &batchv1.Job{}
		if err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: preupgradeJobname(p)}, preprocessJob); err == nil {
			propagationPolicy := metav1.DeletePropagationBackground
			if err = ctx.Delete(ctx, preprocessJob, &client.DeleteOptions{PropagationPolicy: &propagationPolicy}); err != nil {
				return
			}
		} else if !errors.IsNotFound(err) {
			return
		}

		p.Status.Version = p.Status.UpgradeVersion

		done = true
	}

	// cleanup job in case of success/fail
	if job.Status.Succeeded+job.Status.Failed > 0 {
		deletePropagationPolicy := metav1.DeletePropagationBackground

		if err = ctx.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &deletePropagationPolicy}); err != nil {
			return
		}
	}

	// handle failed case
	if job.Status.Failed > 0 {
		err = errPrimaryUpgradeJobFailed
	}

	return
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

func getDBIDFromJob(ctx pcontext.Context, job *batchv1.Job) (dbid string, err error) {
	var ls labels.Selector
	if ls, err = metav1.LabelSelectorAsSelector(job.Spec.Selector); err != nil {
		return
	}

	var pods v1.PodList
	if err = ctx.List(ctx, &pods, &client.ListOptions{LabelSelector: ls}); err != nil {
		return
	}

	if len(pods.Items) == 0 {
		err = fmt.Errorf("no pod found for upgrade job")
		return
	}

	pod := &pods.Items[0]

	var tailLines int64 = 1
	request := ctx.Clientset().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{
		TailLines: &tailLines,
	})
	var logs io.ReadCloser
	if logs, err = request.Stream(ctx); err != nil {
		return
	}
	defer logs.Close()
	buf := make([]byte, 2048)
	n, _ := logs.Read(buf)
	if n == 0 {
		err = fmt.Errorf("short read from pod logs")
		return
	}

	dbid = strings.TrimSpace(string(buf[:n]))

	return
}

func getInitdbArgsFromJob(ctx pcontext.Context, job *batchv1.Job) (args string, err error) {
	var ls labels.Selector
	if ls, err = metav1.LabelSelectorAsSelector(job.Spec.Selector); err != nil {
		return
	}

	var pods v1.PodList
	if err = ctx.List(ctx, &pods, &client.ListOptions{LabelSelector: ls}); err != nil {
		return
	}

	if len(pods.Items) == 0 {
		err = fmt.Errorf("no pod found for preprocess job")
		return
	}

	pod := &pods.Items[0]

	var tailLines int64 = 1
	request := ctx.Clientset().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{
		TailLines: &tailLines,
	})
	var logs io.ReadCloser
	if logs, err = request.Stream(ctx); err != nil {
		return
	}
	defer logs.Close()
	buf := make([]byte, 2048)
	n, _ := logs.Read(buf)
	if n == 0 {
		err = fmt.Errorf("short read from pod logs")
		return
	}

	var cfg preupgrade.Config
	if err = json.Unmarshal(buf[:n], &cfg); err != nil {
		return
	}

	var argsa []string
	if cfg.Locale != "" {
		argsa = append(argsa, fmt.Sprintf("--locale=%s", cfg.Locale))
	}
	if cfg.Encoding != "" {
		argsa = append(argsa, fmt.Sprintf("--encoding=%s", cfg.Encoding))
	}
	if cfg.DataChecksums {
		argsa = append(argsa, "--data-checksums")
	}

	args = strings.Join(argsa, " ")

	return
}
