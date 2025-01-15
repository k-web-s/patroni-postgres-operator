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

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/namsral/flag"

	upgradecommon "github.com/k-web-s/patroni-postgres-operator/private/upgrade/common"
)

var (
	clustername = flag.String("cluster-name", "", "Cluster (StatefulSet) name to derive POD names from")
	clustersize = flag.Int("cluster-size", 0, "Cluster size to use for replication progress checking")
	pause       = flag.Bool(upgradecommon.UpgradeMODEPauseFlag, false, "Pause cluster before exiting")

	memberNames []string
)

func readPrimaryWalLsn(ctx context.Context, conn *pgx.Conn) (wal_lsn string, err error) {
	err = conn.QueryRow(ctx, "SELECT pg_current_wal_lsn()").Scan(&wal_lsn)

	return
}

// readReplicas returns exactly len(memberNames)-1 ip addresses, which have reached the given lsn
// according to postgresql primary server
func readReplicas(ctx context.Context, conn *pgx.Conn, lsn string) (ips []netip.Addr, err error) {
	for {
		var rows pgx.Rows
		rows, err = conn.Query(ctx, "SELECT client_addr FROM pg_catalog.pg_stat_replication WHERE application_name = ANY ($1) AND replay_lsn >= $2", memberNames, lsn)
		if err != nil {
			return
		}

		ips = make([]netip.Addr, 0, len(memberNames))

		for rows.Next() {
			var ip netip.Addr
			if err = rows.Scan(&ip); err != nil {
				return
			}

			ips = append(ips, ip)
		}

		if err = rows.Err(); err != nil {
			return
		}

		if len(ips) == len(memberNames)-1 {
			return
		}
	}
}

func waitReplica(ctx context.Context, ip netip.Addr, lsn string) (err error) {
	conn, err := connectdb(ctx, connectwithhostname(ip.String()))
	if err != nil {
		return
	}
	defer conn.Close(ctx)

	var lsn_diff int64
	if err = conn.QueryRow(ctx, "SELECT pg_wal_lsn_diff($1, pg_last_wal_replay_lsn())", lsn).Scan(&lsn_diff); err != nil {
		return
	}

	if lsn_diff > 0 {
		return fmt.Errorf("lagging by %d bytes", lsn_diff)
	}

	if _, err = conn.Exec(ctx, "CHECKPOINT"); err != nil {
		return
	}

	return
}

func waitReplicas(ctx context.Context, ips []netip.Addr, lsn string) bool {
	var synced atomic.Int32

	wg := &sync.WaitGroup{}
	for _, ip := range ips {
		wg.Add(1)
		go func(ip netip.Addr) {
			defer wg.Done()

			if err := waitReplica(ctx, ip, lsn); err != nil {
				log.Printf("waitReplica(%s): %+v", ip.String(), err)
				return
			}

			synced.Add(1)
		}(ip)
	}
	wg.Wait()

	return int(synced.Load()) == len(memberNames)-1
}

func waitReplicasReplayLSN(ctx context.Context, conn *pgx.Conn, lsn string) (err error) {
	ticker := time.NewTicker(connectTimeout)
	defer ticker.Stop()

	for {
		var replica_ips []netip.Addr

		replica_ips, err = readReplicas(ctx, conn, lsn)
		if err != nil {
			return
		}

		if waitReplicas(ctx, replica_ips, lsn) {
			return
		}

		select {
		case <-ctx.Done():
			return context.DeadlineExceeded
		case <-ticker.C:
		}
	}
}

func syncRestorePoints(ctx context.Context) (err error) {
	log.Print("Connecting to primary")
	conn, err := connectprimarydb(ctx)
	if err != nil {
		return
	}
	defer conn.Close(ctx)

	for {
		// checkpoint primary
		log.Print("Checkpointing primary")
		if _, err = conn.Exec(ctx, "CHECKPOINT"); err != nil {
			return
		}

		// read primary wal_lsn
		var wal_lsn string
		wal_lsn, err = readPrimaryWalLsn(ctx, conn)
		if err != nil {
			return
		}

		// wait for all replicas to catch up
		log.Print("Waiting for all replicas to catch up")
		if err = waitReplicasReplayLSN(ctx, conn, wal_lsn); err != nil {
			return
		}
		log.Print("All replicas caught up")

		// read increase of wal_lsn. if same as before, end loop
		var wal_lsn_increase int64
		if err = conn.QueryRow(ctx, "SELECT pg_wal_lsn_diff(pg_current_wal_lsn(), $1)", wal_lsn).Scan(&wal_lsn_increase); err != nil {
			return
		}

		if wal_lsn_increase == 0 {
			log.Print("wal_lsn on primary did not increase, ready to upgrade.")
			return
		}

		log.Printf("wal_lsn on primary increased by %d bytes, starting next iteration", wal_lsn_increase)
	}
}

func pauseCluster(ctx context.Context) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("http://%s:8008/config", *dbhost), strings.NewReader(`{"pause":true}`))
	if err != nil {
		return
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Print("patroni has been paused")
	} else {
		err = fmt.Errorf("unexpected http return status for pause request: %d", resp.StatusCode)
	}

	return
}

func preupgradesyncfn(ctx context.Context) (err error) {
	for i := 0; i < *clustersize; i++ {
		memberNames = append(memberNames, fmt.Sprintf("%s-%d", *clustername, i))
	}

	if err = syncRestorePoints(ctx); err != nil {
		return
	}

	if *pause {
		if err = pauseCluster(ctx); err != nil {
			return
		}
	}

	return
}
