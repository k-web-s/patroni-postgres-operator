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

package configmap

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	"github.com/k-web-s/patroni-postgres-operator/private/context"
)

var (
	ErrNoDBIDfound  = errors.New("no Database system identifier found")
	ErrNoSyncLeader = errors.New("no sync leader found")
)

const (
	configCMName = "config"
	leaderCMName = "leader"
	syncCMName   = "sync"

	syncCMLeaderAnnotation = "leader"
	configCMdbidAnnotation = "initialize"
)

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	for _, name := range []string{configCMName, leaderCMName, syncCMName} {
		cmName := fmt.Sprintf("%s-%s", p.Name, name)
		cm := &corev1.ConfigMap{}

		err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: cmName}, cm)
		if err == nil {
			if len(cm.OwnerReferences) == 0 {
				if err = ctx.SetMeta(cm); err != nil {
					return
				}

				err = ctx.Update(ctx, cm)
			}
		} else {
			if !apierrors.IsNotFound(err) {
				return
			}

			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: cmName,
				},
			}

			if err = ctx.SetMeta(cm); err != nil {
				return
			}

			err = ctx.Create(ctx, cm)
		}

		if err != nil {
			return
		}
	}

	return
}

func GetDBId(ctx context.Context, p *v1alpha1.PatroniPostgres) (dbid string, err error) {
	cmName := fmt.Sprintf("%s-%s", p.Name, configCMName)
	cm := &corev1.ConfigMap{}

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: cmName}, cm)
	if err != nil {
		return
	}

	dbid, ok := cm.ObjectMeta.Annotations[configCMdbidAnnotation]
	if !ok {
		return "", ErrNoDBIDfound
	}

	return
}

func SetDBId(ctx context.Context, p *v1alpha1.PatroniPostgres, dbid string) (err error) {
	cmName := fmt.Sprintf("%s-%s", p.Name, configCMName)
	cm := &corev1.ConfigMap{}

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: cmName}, cm)
	if err != nil {
		return
	}

	cm.ObjectMeta.Annotations[configCMdbidAnnotation] = dbid

	err = ctx.Update(ctx, cm)

	return
}

func GetSyncLeader(ctx context.Context, p *v1alpha1.PatroniPostgres) (index int, err error) {
	// with one-node cluster, index 0 is the leader always
	if len(p.Status.VolumeStatuses) > 1 {
		cmName := fmt.Sprintf("%s-%s", p.Name, syncCMName)
		cm := &corev1.ConfigMap{}

		err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: cmName}, cm)
		if err != nil {
			return
		}

		leader, ok := cm.ObjectMeta.Annotations[syncCMLeaderAnnotation]
		if !ok {
			return 0, ErrNoSyncLeader
		}

		splitted := strings.Split(leader, "-")
		_, err = fmt.Sscanf(splitted[len(splitted)-1], "%d", &index)

	}

	return
}
