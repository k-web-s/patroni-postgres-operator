package security

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

const (
	databaseUserId = 15432
)

var (
	fsGroupChangePolicy = corev1.FSGroupChangeOnRootMismatch

	// Generic container security contexts
	ContainerSecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: pointer.Bool(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}

	// GenericPodSecurityContext defines pod level security context
	// for generic/other workloads (e.g. pre/post-upgrade jobs)
	GenericPodSecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot: pointer.Bool(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	// DatabasePodSecurityContext defines pod level security context
	// for database workloads
	DatabasePodSecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot:        pointer.Bool(true),
		RunAsUser:           pointer.Int64(databaseUserId),
		RunAsGroup:          pointer.Int64(databaseUserId),
		FSGroup:             pointer.Int64(databaseUserId),
		FSGroupChangePolicy: &fsGroupChangePolicy,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
)
