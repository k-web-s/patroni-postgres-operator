/*
Copyright 2024 Richard Kojedzinszky

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
