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

package statefulset

import (
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	"github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/pvc"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/rbac"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/secret"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/service"
)

const (
	Image             = "ghcr.io/rkojedzinszky/postgres-patroni:20240318"
	postgresComponent = "postgres"
	patroniPort       = 8008
	patroniPortName   = "patroni"

	PatroniSuperuserUsername   = "postgres"
	patroniReplicationUsername = "standby"

	DataVolumeMountPath = "/var/lib/postgresql"
)

var (
	user                = int64(15432)
	fsGroupChangePolicy = corev1.FSGroupChangeOnRootMismatch

	PodSecurityContext = &corev1.PodSecurityContext{
		RunAsUser:           &user,
		RunAsGroup:          &user,
		FSGroup:             &user,
		RunAsNonRoot:        pointer.Bool(true),
		FSGroupChangePolicy: &fsGroupChangePolicy,
	}

	SecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: pointer.Bool(false),
	}
)

// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;delete

func ReconcileSts(ctx context.Context, p *v1alpha1.PatroniPostgres) (sts *appsv1.StatefulSet, err error) {
	var create bool

	sts, err = GetK8SStatefulSet(ctx, p)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		sts = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: p.Name,
			},
		}

		create = true
	}

	if err = ctx.SetMeta(sts); err != nil {
		return
	}

	enableServiceLinks := false
	replicas := int32(len(p.Spec.Nodes))
	podLabels := ctx.CommonLabels()
	labelsBytes, err := json.Marshal(podLabels)
	if err != nil {
		return
	}
	labelsString := string(labelsBytes)

	// Handle PodAntiAffinityTopologyKey
	affinity := p.Spec.Affinity.DeepCopy()
	if p.Spec.PodAntiAffinityTopologyKey != "" {
		if affinity == nil {
			affinity = &corev1.Affinity{}
		}

		if affinity.PodAntiAffinity == nil {
			affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
		}

		affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(
			affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
			corev1.PodAffinityTerm{
				TopologyKey: p.Spec.PodAntiAffinityTopologyKey,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: podLabels,
				},
			},
		)
	}

	sts.Spec = appsv1.StatefulSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: podLabels,
		},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: pvc.VolumeName,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
				},
			},
		},
		PodManagementPolicy: appsv1.ParallelPodManagement,
		MinReadySeconds:     60,
		Replicas:            &replicas,
		ServiceName:         service.HeadlessServiceName(p),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      podLabels,
				Annotations: p.Spec.Annotations,
			},
			Spec: corev1.PodSpec{
				Affinity:           affinity,
				ServiceAccountName: rbac.ServiceAccountName(p),
				EnableServiceLinks: &enableServiceLinks,
				Containers: []corev1.Container{
					{
						Name:  "postgres",
						Image: Image,
						Env: []corev1.EnvVar{
							{
								Name:  "PG_VERSION",
								Value: fmt.Sprintf("%d", p.Status.Version),
							},
							{
								Name: "PATRONI_KUBERNETES_POD_IP",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "status.podIP",
									},
								},
							},
							{
								Name: "PATRONI_KUBERNETES_NAMESPACE",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.namespace",
									},
								},
							},
							{
								Name: "PATRONI_NAME",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.name",
									},
								},
							},
							{
								Name:  "PATRONI_SCOPE",
								Value: p.Name,
							},
							{
								Name:  "PATRONI_KUBERNETES_LABELS",
								Value: labelsString,
							},
							{
								Name:  "PATRONI_SUPERUSER_USERNAME",
								Value: PatroniSuperuserUsername,
							},
							{
								Name: "PATRONI_SUPERUSER_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: secret.Name(p),
										},
										Key: secret.SuperUserPasswordKey,
									},
								},
							},
							{
								Name:  "PATRONI_REPLICATION_USERNAME",
								Value: patroniReplicationUsername,
							},
							{
								Name: "PATRONI_REPLICATION_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: secret.Name(p),
										},
										Key: secret.ReplicationUserPasswordKey,
									},
								},
							},
							{
								Name:  "PATRONI_INITIAL_SYNCHRONOUS_MODE",
								Value: "true",
							},
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          service.PostgresPortName,
								ContainerPort: service.PostgresPort,
							},
							{
								Name:          patroniPortName,
								ContainerPort: patroniPort,
							},
						},
						Resources: p.Spec.Resources,
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/liveness",
									Port: intstr.FromInt(patroniPort),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readiness",
									Port: intstr.FromInt(patroniPort),
								},
							},
							InitialDelaySeconds: 5,
							// Workaround until https://github.com/kubernetes/kubernetes/issues/119234 is fixed
							SuccessThreshold: 3,
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      pvc.VolumeName,
								MountPath: DataVolumeMountPath,
							},
						},
						SecurityContext: SecurityContext,
					},
				},
				SecurityContext:  PodSecurityContext,
				ImagePullSecrets: p.Spec.ImagePullSecrets,
				NodeSelector:     p.Spec.NodeSelector,
				Tolerations:      p.Spec.Tolerations,
			},
		},
	}

	sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, genNodeTagsEnvs(p)...)

	sts.Spec.Template.Spec.Containers = append(sts.Spec.Template.Spec.Containers, p.Spec.ExtraContainers...)

	if create {
		err = ctx.Create(ctx, sts)
	} else {
		err = ctx.Update(ctx, sts)
	}

	p.Status.Ready = sts.Status.ReadyReplicas

	return
}

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	sts, err := ReconcileSts(ctx, p)
	if err != nil {
		return
	}

	if int(sts.Status.ReadyReplicas) == len(p.Spec.Nodes) {
		p.Status.State = v1alpha1.PatroniPostgresStateReady
	} else {
		p.Status.State = v1alpha1.PatroniPostgresStateScaling
	}

	return
}

func GetK8SStatefulSet(ctx context.Context, p *v1alpha1.PatroniPostgres) (sts *appsv1.StatefulSet, err error) {
	sts = &appsv1.StatefulSet{}
	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: p.Name}, sts)
	return
}

func genNodeTagsEnvs(p *v1alpha1.PatroniPostgres) (envs []corev1.EnvVar) {
	podPrefix := strings.ReplaceAll(p.Name, "-", "_")

	for idx := range p.Spec.Nodes {
		node := &p.Spec.Nodes[idx]

		if node.Tags.NoSync {
			envs = append(envs, corev1.EnvVar{
				Name:  fmt.Sprintf("PATRONI_NODE_%s_%d_TAG_%s", podPrefix, idx, "nosync"),
				Value: "true",
			})
		}

		if node.Tags.NoFailover {
			envs = append(envs, corev1.EnvVar{
				Name:  fmt.Sprintf("PATRONI_NODE_%s_%d_TAG_%s", podPrefix, idx, "nofailover"),
				Value: "true",
			})
		}
	}

	return
}
