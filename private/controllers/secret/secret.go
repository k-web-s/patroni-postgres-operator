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

package secret

import (
	"crypto/rand"
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	"github.com/k-web-s/patroni-postgres-operator/private/context"
)

const (
	SuperUserPasswordKey       = "superuser-password"
	ReplicationUserPasswordKey = "replication-password"
)

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	secret := &corev1.Secret{}

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: Name(p)}, secret)
	if err == nil {
		if len(secret.OwnerReferences) == 0 {
			if err = ctx.SetMeta(secret); err != nil {
				return
			}

			err = ctx.Update(ctx, secret)
		}
	} else {
		if !errors.IsNotFound(err) {
			return
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: Name(p),
			},
			StringData: map[string]string{
				SuperUserPasswordKey:       genSecret(),
				ReplicationUserPasswordKey: genSecret(),
			},
		}

		if err = ctx.SetMeta(secret); err != nil {
			return
		}

		err = ctx.Create(ctx, secret)
	}

	return
}

// Name returns name for secret
func Name(p *v1alpha1.PatroniPostgres) string {
	return p.Name
}

func genSecret() string {
	key := make([]byte, 48)

	_, _ = rand.Read(key)

	return base64.StdEncoding.EncodeToString(key)
}
