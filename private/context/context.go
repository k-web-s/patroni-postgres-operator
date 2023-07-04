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

package context

import (
	gocontext "context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
)

const (
	nameLabel      = "app.kubernetes.io/name"
	nameValue      = "patroni-postgres"
	instanceLabel  = "app.kubernetes.io/instance"
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "kwebs-patroni-postgres-operator"

	clusterNameLabel = "cluster-name"
)

type Context interface {
	gocontext.Context
	client.Client

	// CommonLabels returns common labels to use on objects
	CommonLabels() map[string]string

	// ListOption returns list options to use to filter for own objects
	ListOption() (client.ListOption, error)

	// SetMeta sets Namespace and Labels in ObjectMeta, sets ownerreference to current PatroniPostgres instance
	SetMeta(metav1.Object) error

	// Clientset returns *kubernetes.Clientset
	Clientset() *kubernetes.Clientset
}

func New(ctx gocontext.Context, cl client.Client, clientset *kubernetes.Clientset, pp *v1alpha1.PatroniPostgres) Context {
	return &context{
		Context:   ctx,
		Client:    cl,
		clientset: clientset,
		pp:        pp,
	}
}

type context struct {
	gocontext.Context
	client.Client
	clientset *kubernetes.Clientset
	pp        *v1alpha1.PatroniPostgres
}

func (c *context) CommonLabels() (ret map[string]string) {
	ret = map[string]string{
		nameLabel:        nameValue,
		instanceLabel:    c.pp.Name,
		managedByLabel:   managedByValue,
		clusterNameLabel: c.pp.Name,
	}

	return
}

// ListOption returns filter matching owned objects
func (c *context) ListOption() (lo client.ListOption, err error) {
	return client.MatchingLabels(c.CommonLabels()), nil
}

func (c *context) SetMeta(m metav1.Object) error {
	m.SetNamespace(c.pp.Namespace)
	m.SetLabels(c.CommonLabels())
	return controllerutil.SetOwnerReference(c.pp, m, c.Client.Scheme())
}

func (c *context) Clientset() *kubernetes.Clientset {
	return c.clientset
}
