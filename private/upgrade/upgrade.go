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
	_ "embed"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/statefulset"
)

const (
	operatorImage    = "github.com/k-web-s/patroni-postgres-operator"
	helperModeEnvVar = "MODE"
)

func init() {
	// escape '$' in embedded scripts
	primaryUpgrade = strings.ReplaceAll(primaryUpgrade, "$", "$$")
	primaryStream = strings.ReplaceAll(primaryStream, "$", "$$")
	secondaryUpgrade = strings.ReplaceAll(secondaryUpgrade, "$", "$$")
}

func Do(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (ret ctrl.Result, err error) {
	switch p.Status.State {
	case v1alpha1.PatroniPostgresStateReady:
		p.Status.State = v1alpha1.PatroniPostgresStateUpgradePreupgrade
		ret.Requeue = true
		return
	case v1alpha1.PatroniPostgresStateUpgradePreupgrade:
		var ok bool
		ok, ret, err = checkPreupgradeJob(ctx, p)
		if !ok {
			return
		}

		var sts *appsv1.StatefulSet
		sts, err = statefulset.GetK8SStatefulSet(ctx, p)
		if err != nil {
			return
		}
		replicas := int32(0)
		sts.Spec.Replicas = &replicas
		if err = ctx.Update(ctx, sts); err != nil {
			return
		}
		p.Status.State = v1alpha1.PatroniPostgresStateUpgradeScaleDown
		return
	case v1alpha1.PatroniPostgresStateUpgradeScaleDown:
		var sts *appsv1.StatefulSet
		sts, err = statefulset.GetK8SStatefulSet(ctx, p)
		if err != nil {
			return
		}
		if sts.Status.AvailableReplicas == 0 {
			if _, err = configmap.GetSyncLeader(ctx, p); err == nil {
				p.Status.State = v1alpha1.PatroniPostgresStateUpgradePrimary
				ret.Requeue = true
			}

			return
		}
	case v1alpha1.PatroniPostgresStateUpgradePrimary:
		return upgradePrimary(ctx, p)

	case v1alpha1.PatroniPostgresStateUpgradeSecondaries:
		return upgradeSecondaries(ctx, p)
	}

	return
}
