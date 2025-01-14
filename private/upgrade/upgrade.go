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

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
)

var (
	operatorImage = "ghcr.io/k-web-s/patroni-postgres-operator"
)

type upgradehandler interface {
	// name returns the Postgres State corresponding to upgrade step
	name() v1alpha1.PatroniPostgresState

	// handle processes an upgrade step.
	// When finished, it must return done==true to indicate that this step is done
	handle(pcontext.Context, *v1alpha1.PatroniPostgres) (done bool, err error)
}

type upgradestep struct {
	handler upgradehandler
	next    upgradehandler
}

var (
	upgrademap = map[v1alpha1.PatroniPostgresState]*upgradestep{}
)

func init() {
	// escape '$' in embedded scripts
	primaryUpgrade = strings.ReplaceAll(primaryUpgrade, "$", "$$")
	primaryStream = strings.ReplaceAll(primaryStream, "$", "$$")
	secondaryUpgrade = strings.ReplaceAll(secondaryUpgrade, "$", "$$")
	primaryUpgradeMove = strings.ReplaceAll(primaryUpgradeMove, "$", "$$")

	var step *upgradestep
	for _, handler := range []upgradehandler{
		preupgradeHandler{},
		preupgradeScaledownHandler{},
		preupgradeSyncHandler{},
		scaledownHandler{},
		primaryUpgradeHandler{},
		secondaryUpgradeHandler{},
		primaryUpgradeMoveHandler{},
		postupgradeHandler{},
	} {
		if step != nil {
			step.next = handler
		}

		step = &upgradestep{
			handler: handler,
		}

		upgrademap[step.handler.name()] = step
	}
}

func Handle(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (ret ctrl.Result, err error) {
	handler, ok := upgrademap[p.Status.State]
	if !ok {
		p.Status.State = v1alpha1.PatroniPostgresStateUpgradePreupgrade
		ret.Requeue = true
		return
	}

	done, err := handler.handler.handle(ctx, p)
	if err != nil {
		return
	}

	if done {
		if handler.next != nil {
			p.Status.State = handler.next.name()
		} else {
			p.Status.State = v1alpha1.PatroniPostgresStateReady
			p.Status.UpgradeVersion = 0

			err = configmap.ClearUpgradeAnnotations(ctx, p)
		}

		ret.Requeue = true
	}

	return
}
