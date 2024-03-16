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
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	"github.com/k-web-s/patroni-postgres-operator/private/upgrade/preupgrade"
)

var (
	errPreupgradeJobFailed = fmt.Errorf("preupgrade job failed")
)

type preupgradeHandler struct {
}

func (preupgradeHandler) name() v1alpha1.PatroniPostgresState {
	return v1alpha1.PatroniPostgresStateUpgradePreupgrade
}

func (preupgradeHandler) handle(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (done bool, err error) {
	// Ensure cluster is up & running
	if _, err = statefulset.ReconcileSts(ctx, p); err != nil {
		return
	}

	// Create/handle preupgrade job
	job := &batchv1.Job{}
	jobname := preupgradeJobname(p)

	err = ctx.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: jobname}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return
		}

		err = createUpgradeJob(ctx, p, preupgrade.ModeString)

		return
	}

	if job.Status.Succeeded > 0 {
		done = true
	} else if job.Status.Failed > 0 {
		if err = ctx.Delete(ctx, job); err != nil {
			return
		}

		err = errPreupgradeJobFailed
	}

	return
}

func preupgradeJobname(p *v1alpha1.PatroniPostgres) string {
	return upgradeJobname(p, preupgrade.ModeString)
}
