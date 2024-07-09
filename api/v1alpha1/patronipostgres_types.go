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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeTags holds a subset of Patroni tags
// https://patroni.readthedocs.io/en/latest/yaml_configuration.html#tags
type NodeTags struct {
	// NoSync If set to true the node will never be selected as a synchronous replica.
	NoSync bool `json:"nosync,omitempty"`

	// NoFailover controls whether this node is allowed to participate in the leader
	// race and become a leader. Defaults to false, meaning this node _can_
	// participate in leader races.
	NoFailover bool `json:"nofailover,omitempty"`
}

// Node represents a PatroniPostgres node's configuration
type Node struct {
	// StorageClassName references a storage class to allocate volume from
	StorageClassName string `json:"storageClassName"`

	// AccessMode allows for overriding implicit ReadWriteOnce accessmode
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`

	// Tags for Node
	Tags NodeTags `json:"tags,omitempty"`
}

type VolumeStatus struct {
	// ClaimName holds PersistentVolumeClaim's name
	ClaimName string `json:"claimName"`

	// Phase mirrors PersistentVolumeClaimStatus.Phase
	Phase corev1.PersistentVolumeClaimPhase `json:"phase,omitempty"`

	// Capacity mirrors PersistentVolumeClaimStatus.Capacity[ResourceStorage]
	Capacity resource.Quantity `json:"capacity,omitempty"`
}

// PatroniPostgresSpec defines the desired state of PatroniPostgres
type PatroniPostgresSpec struct {
	// Ignore marks this instance to be ignored by the operator
	Ignore bool `json:"ignore,omitempty"`

	// +kubebuilder:validation:Enum:=13;15
	Version int `json:"version"`

	// Nodes holds nodes's desired configuration.
	// Thus it implicitly defines the number of PostgreSQL nodes (replicas).
	// +kubebuilder:validation:MinItems:=1
	Nodes []Node `json:"nodes"`

	// VolumeSize sets size for volumes
	VolumeSize resource.Quantity `json:"volumeSize"`

	// ServiceType defines primary service type
	// +kubebuilder:validation:Enum:=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default:=ClusterIP
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// Annotations will be added to PODs
	Annotations map[string]string `json:"annotations,omitempty"`

	// PodAntiAffinityTopologyKey defines topology key used for PodAntiAffinity
	// empty means no PodAntiAffinity
	// +optional
	PodAntiAffinityTopologyKey string `json:"podAntiAffinityTopologyKey,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Compute Resources required by postgres and upgrade containers.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// ExtraContainers lists extra containers added to pods
	// +optional
	ExtraContainers []corev1.Container `json:"extraContainers,omitempty"`

	// AccessControl controls access to PostgreSQL service.
	// If undefined, allows access from anywhere
	// More info: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#networkpolicyingressrule-v1-networking-k8s-io
	// +optional
	AccessControl []networking.NetworkPolicyPeer `json:"accessControl,omitempty"`
}

// PatroniPostgresState represents overall cluster state
type PatroniPostgresState string

const (
	PatroniPostgresStateScaling            PatroniPostgresState = "scaling"
	PatroniPostgresStateReady              PatroniPostgresState = "ready"
	PatroniPostgresStateUpgradePreupgrade  PatroniPostgresState = "upgrade-preupgrade"
	PatroniPostgresStateUpgradeScaleDown   PatroniPostgresState = "upgrade-scaledown"
	PatroniPostgresStateUpgradePrimary     PatroniPostgresState = "upgrade-primary"
	PatroniPostgresStateUpgradeSecondaries PatroniPostgresState = "upgrade-secondaries"
	PatroniPostgresStateUpgradePostupgrade PatroniPostgresState = "upgrade-postupgrade"
)

// PatroniPostgresStatus defines the observed state of PatroniPostgres
type PatroniPostgresStatus struct {
	// VolumeStatuses holds status for each allocated volume
	VolumeStatuses []VolumeStatus `json:"volumeStatuses"`

	// Ready replicas are ready
	Ready int32 `json:"ready"`

	// Version represents current cluster version
	Version int `json:"version"`

	// State represents cluster state
	State PatroniPostgresState `json:"state"`

	// UpgradeVersion represents upgrade target version
	UpgradeVersion int `json:"upgradeVersion,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=.status.version,description="Current version",name=CVer,type=string
//+kubebuilder:printcolumn:JSONPath=.status.ready,description="Ready replicas",name=Ready,type=integer
//+kubebuilder:printcolumn:JSONPath=.status.state,description="Cluster state",name=State,type=string

// PatroniPostgres is the Schema for the patronipostgres API
type PatroniPostgres struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PatroniPostgresSpec   `json:"spec,omitempty"`
	Status PatroniPostgresStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PatroniPostgresList contains a list of PatroniPostgres
type PatroniPostgresList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PatroniPostgres `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PatroniPostgres{}, &PatroniPostgresList{})
}
