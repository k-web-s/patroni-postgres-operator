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

package pvc

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	context "github.com/k-web-s/patroni-postgres-operator/private/context"
)

const (
	// VolumeName used inside container
	VolumeName = "pgdata"
)

// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;create;patch;delete

func Reconcile(ctx context.Context, p *v1alpha1.PatroniPostgres) (err error) {
	var lo client.ListOption

	if lo, err = ctx.ListOption(); err != nil {
		return
	}

	// List of existing PVCs
	existingPVCList := &corev1.PersistentVolumeClaimList{}
	if err = ctx.List(ctx, existingPVCList, lo); err != nil {
		return
	}

	// Organize them in a map
	existingPVCMap := make(map[string]*corev1.PersistentVolumeClaim)
	for idx := range existingPVCList.Items {
		pvc := &existingPVCList.Items[idx]
		existingPVCMap[pvc.Name] = pvc
	}

	// During iteration we remove entries which we need
	// Entries left in the map will be deleted
	p.Status.VolumeStatuses = nil
	for idx, node := range p.Spec.Nodes {
		name := PVCName(p, idx)
		var origpvc, pvc *corev1.PersistentVolumeClaim
		var existing bool

		if origpvc, existing = existingPVCMap[name]; existing {
			delete(existingPVCMap, name)
			pvc = origpvc.DeepCopy()
		} else {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},

				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{node.GetAccessMode()},
					StorageClassName: &node.StorageClassName,
				},
			}

		}

		if err = ctx.SetMeta(pvc); err != nil {
			return
		}

		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: p.Spec.VolumeSize,
		}

		if existing {
			err = ctx.Patch(ctx, pvc, client.MergeFrom(origpvc))
		} else {
			err = ctx.Create(ctx, pvc)
		}

		if err != nil {
			return
		}

		p.Status.VolumeStatuses = append(p.Status.VolumeStatuses, v1alpha1.VolumeStatus{
			ClaimName: pvc.Name,
			Phase:     pvc.Status.Phase,
			Capacity:  pvc.Status.Capacity[corev1.ResourceStorage],
		})
	}

	propagation := metav1.DeletePropagationBackground
	for _, pvc := range existingPVCMap {
		if err = ctx.Delete(ctx, pvc, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// PVCName returns name PersistentVolumeClaim associated with pod idx
func PVCName(i *v1alpha1.PatroniPostgres, idx int) string {
	return fmt.Sprintf("%s-%s-%d", VolumeName, i.Name, idx)
}
