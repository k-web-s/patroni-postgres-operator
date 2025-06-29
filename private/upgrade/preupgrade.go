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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k-web-s/patroni-postgres-operator/api/v1alpha1"
	pcontext "github.com/k-web-s/patroni-postgres-operator/private/context"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/configmap"
	"github.com/k-web-s/patroni-postgres-operator/private/controllers/service"
	upgradecommon "github.com/k-web-s/patroni-postgres-operator/private/upgrade/common"
)

var (
	errPreupgradeJobFailed            = fmt.Errorf("preupgrade job failed")
	errMaxPreparedTransactionsNotZero = fmt.Errorf("max_prepared_transactions not zero, cannot continue")
)

type preupgradeHandler struct {
}

func (preupgradeHandler) name() v1alpha1.PatroniPostgresState {
	return v1alpha1.PatroniPostgresStateUpgradePreupgrade
}

func (preupgradeHandler) handle(ctx pcontext.Context, p *v1alpha1.PatroniPostgres) (done bool, err error) {
	// Create/handle preupgrade job
	job, err := ensureUpgradeJob(ctx, p, preupgradeJob{p})
	if err != nil {
		return
	}

	var err2 error

	if job.Status.Succeeded > 0 {
		var config upgradecommon.Config
		if err = getInitdbArgsFromJob(ctx, job, &config); err != nil {
			return
		}

		if config.MaxPreparedTransactions != 0 {
			err2 = errMaxPreparedTransactionsNotZero
		} else {
			if err = configmap.SetPrimaryInitdbArgs(ctx, p, parseConfigToInitdbArgs(&config)); err != nil {
				return
			}

			done = true
		}
	}

	if err = cleanupJob(ctx, job); err != nil {
		return
	}

	if job.Status.Failed > 0 {
		err = errPreupgradeJobFailed
	}

	if err == nil {
		err = err2
	}

	return
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

func getInitdbArgsFromJob(ctx pcontext.Context, job *batchv1.Job, config *upgradecommon.Config) (err error) {
	var ls labels.Selector
	if ls, err = metav1.LabelSelectorAsSelector(job.Spec.Selector); err != nil {
		return
	}

	var pods v1.PodList
	if err = ctx.List(ctx, &pods, &client.ListOptions{LabelSelector: ls}); err != nil {
		return
	}

	if len(pods.Items) == 0 {
		err = fmt.Errorf("no pod found for preprocess job")
		return
	}

	pod := &pods.Items[0]

	var tailLines int64 = 1
	request := ctx.Clientset().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{
		TailLines: &tailLines,
	})
	var logs io.ReadCloser
	if logs, err = request.Stream(ctx); err != nil {
		return
	}
	defer logs.Close()
	buf := make([]byte, 2048)
	n, _ := logs.Read(buf)
	if n == 0 {
		err = fmt.Errorf("short read from pod logs")
		return
	}

	err = json.Unmarshal(buf[:n], config)

	return
}

func parseConfigToInitdbArgs(config *upgradecommon.Config) string {
	var argsa []string
	if config.Locale != "" {
		argsa = append(argsa, fmt.Sprintf("--locale=%s", config.Locale))
	}
	if config.Encoding != "" {
		argsa = append(argsa, fmt.Sprintf("--encoding=%s", config.Encoding))
	}
	if config.DataChecksums {
		argsa = append(argsa, "--data-checksums")
	}

	return strings.Join(argsa, " ")
}

type preupgradeJob struct {
	p *v1alpha1.PatroniPostgres
}

// ActiveDeadlineSeconds implements UpgradeJob.
func (preupgradeJob) ActiveDeadlineSeconds() int64 {
	return 300
}

// DBPort implements UpgradeJob.
func (preupgradeJob) DBPort() int {
	return service.PostgresPort
}

// Mode implements UpgradeJob.
func (preupgradeJob) Mode() string {
	return upgradecommon.UpgradeMODEPre
}

// CustomizePodSpec implements UpgradeJob.
func (preupgradeJob) CustomizePodSpec(*v1.PodSpec) {
}

var _ UpgradeJob = preupgradeJob{}
