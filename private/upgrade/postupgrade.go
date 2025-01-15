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

	v1 "k8s.io/api/core/v1"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/service"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
	upgradecommon "github.com/k-web-s/patroni-postgres-operator/private/upgrade/common"
)

var (
	errPostupgradeJobFailed = fmt.Errorf("preupgrade job failed")
)

type postupgradeHandler struct {
}

func (postupgradeHandler) name() v1alpha1.PatroniPostgresState {
	return v1alpha1.PatroniPostgresStateUpgradePostupgrade
}

func (postupgradeHandler) handle(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (done bool, err error) {
	// Ensure cluster is up & running
	sts, err := statefulset.ReconcileSts(ctx, p)
	if err != nil {
		return
	}
	if err = service.Reconcile(ctx, p); err != nil {
		return
	}

	if int(sts.Status.ReadyReplicas) != len(p.Spec.Nodes) {
		return
	}

	job, err := ensureUpgradeJob(ctx, p, postupgradeJob{p})
	if err != nil {
		return
	}

	if job.Status.Succeeded > 0 {
		done = true
	}

	if err = cleanupJob(ctx, job); err != nil {
		return
	}

	if job.Status.Failed > 0 {
		err = errPostupgradeJobFailed
	}

	return
}

type postupgradeJob struct {
	p *v1alpha1.PatroniPostgres
}

// ActiveDeadlineSeconds implements UpgradeJob.
func (postupgradeJob) ActiveDeadlineSeconds() int64 {
	return 3600
}

// DBPort implements UpgradeJob.
func (postupgradeJob) DBPort() int {
	return service.PostgresPort
}

// Mode implements UpgradeJob.
func (postupgradeJob) Mode() string {
	return upgradecommon.UpgradeMODEPost
}

// CustomizePodSpec implements UpgradeJob.
func (postupgradeJob) CustomizePodSpec(*v1.PodSpec) {
}

var _ UpgradeJob = postupgradeJob{}
