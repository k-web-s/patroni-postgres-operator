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
	"encoding/json"
	"io"
	"os"

	upgradecommon "github.com/k-web-s/patroni-postgres-operator/private/upgrade/common"
)

func preupgradefn(ctx context.Context) (err error) {
	dbconn, err := connectdb(ctx)
	if err != nil {
		return
	}
	defer dbconn.Close(ctx)

	var cfg upgradecommon.Config

	if err = dbconn.QueryRow(ctx, "SHOW lc_collate").Scan(&cfg.Locale); err != nil {
		return
	}

	if err = dbconn.QueryRow(ctx, "SHOW server_encoding").Scan(&cfg.Encoding); err != nil {
		return
	}

	if err = dbconn.QueryRow(ctx, `SELECT
		pg_catalog.current_setting('data_checksums')::bool,
		pg_catalog.current_setting('max_prepared_transactions')::int`).
		Scan(&cfg.DataChecksums, &cfg.MaxPreparedTransactions); err != nil {
		return
	}

	var out []byte
	if out, err = json.Marshal(&cfg); err != nil {
		return
	}

	var n int
	n, err = os.Stdout.Write(out)
	if err != nil {
		return
	}

	if n < len(out) {
		err = io.ErrShortWrite
	}

	return
}
