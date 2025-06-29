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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/pvc"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/security"
)

var (
	//go:embed upgrade-scripts/primary-stream
	primaryStream string
)

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;create
// +kubebuilder:rbac:groups="",resources=services,verbs=get;create

func upgradeSecondariesEnsurestreamer(ctx pcontext.Context, p *v1alpha1.PatroniPostgres, leader int) (sts *appsv1.StatefulSet, err error) {
	sts = &appsv1.StatefulSet{}
	name := fmt.Sprintf("%s-pstream", p.Name)

	if err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: name}, sts); err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		labels := ctx.CommonLabels()
		labels["upgrade"] = "pstream"

		sts = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				ServiceName: name,
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:    "pstream",
								Image:   statefulset.Image,
								Command: []string{"sh", "-c", primaryStream},
								VolumeMounts: []v1.VolumeMount{
									{
										Name:      pvc.VolumeName,
										MountPath: statefulset.DataVolumeMountPath,
									},
								},
								Ports: []v1.ContainerPort{
									{
										Name:          rsyncPortName,
										ContainerPort: rsyncPort,
									},
								},
								ReadinessProbe: &v1.Probe{
									ProbeHandler: v1.ProbeHandler{
										TCPSocket: &v1.TCPSocketAction{
											Port: intstr.FromString(rsyncPortName),
										},
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
										ClaimName: p.Status.VolumeStatuses[leader].ClaimName,
									},
								},
							},
						},
						SecurityContext:  security.DatabasePodSecurityContext,
						ImagePullSecrets: p.Spec.ImagePullSecrets,
						NodeSelector:     p.Spec.NodeSelector,
						Tolerations:      p.Spec.Tolerations,
					},
				},
			},
		}

		if err = ctx.SetMeta(sts); err != nil {
			return
		}

		if err = ctx.Create(ctx, sts); err != nil {
			return
		}
	}

	service := &v1.Service{}

	if err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: name}, service); err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		service = &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: p.Namespace,
				Name:      name,
			},
			Spec: v1.ServiceSpec{
				Selector: sts.Spec.Template.Labels,
				Ports: []v1.ServicePort{
					{
						Name:       rsyncPortName,
						Port:       rsyncPort,
						TargetPort: intstr.FromString(rsyncPortName),
					},
				},
			},
		}

		if err = controllerutil.SetOwnerReference(sts, service, ctx.Scheme()); err != nil {
			return
		}

		if err = ctx.Create(ctx, service); err != nil {
			return
		}
	}

	return
}
