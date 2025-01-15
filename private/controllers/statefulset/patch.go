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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
)

type Patch interface {
	Patch(*appsv1.StatefulSet)
}

type withPostgresqlPort int

func (p withPostgresqlPort) Patch(sts *appsv1.StatefulSet) {
	sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "POSTGRESQL_PORT",
		Value: fmt.Sprintf("%d", p),
	})
}

func WithPostgresqlPort(port int) Patch {
	return withPostgresqlPort(port)
}

type withPausedPatroni int

func (p withPausedPatroni) Patch(sts *appsv1.StatefulSet) {
	sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "PGDATA",
		Value: fmt.Sprintf("%s/data", DataVolumeMountPath),
	})

	sts.Spec.Template.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"sh",
					"-c",
					`
host=
eval $(sed -n -r -e "/^primary_conninfo[[:space:]]*=/{s/^.*=[[:space:]]*'//; s/'.*$//; p}" ${PGDATA}/postgresql.conf)
if [ -n "$host" ]; then
  echo "Waiting for primary to shutdown"
  while nc -z -w 1 "$host" "$port"; do
	  sleep 1
	done
fi

/usr/lib/postgresql/${PG_VERSION}/bin/pg_ctl stop -D ${PGDATA} -m  fast -w`,
				},
			},
		},
	}
}

func WithPausedPatroni(p *v1alpha1.PatroniPostgres) Patch {
	return withPausedPatroni(p.Status.Version)
}
