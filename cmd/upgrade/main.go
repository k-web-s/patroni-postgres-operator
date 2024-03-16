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
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/namsral/flag"

	"github.com/k-web-s/patroni-postgres-operator/private/upgrade/postupgrade"
	"github.com/k-web-s/patroni-postgres-operator/private/upgrade/preupgrade"
)

var (
	dbhost     = flag.String("dbhost", "", "postgresql host")
	dbport     = flag.Int("dbport", 5432, "postgresql port")
	dbuser     = flag.String("dbuser", "postgres", "postgres user")
	dbpassword = flag.String("dbpassword", "", "postgres password")
	dbname     = flag.String("dbname", "postgres", "postgres database name")
	mode       = flag.String("mode", "", "operation mode")
)

var dbconn *pgx.Conn

func connectdbname(ctx context.Context, name string) (conn *pgx.Conn, err error) {
	sleep := 250 * time.Millisecond
	connects := 0

	for {
		if conn, err = pgx.Connect(ctx, fmt.Sprintf(
			"host=%s port=%d user=%s password=%s database=%s sslmode=disable",
			*dbhost, *dbport, *dbuser, *dbpassword, name,
		)); err == nil {
			return
		}
		connects++

		if connects == 5 {
			return
		}

		time.Sleep(sleep)
		sleep = sleep * 2
	}
}

func connectdb(ctx context.Context) (err error) {
	dbconn, err = connectdbname(ctx, *dbname)

	return
}

func preupgradefn(ctx context.Context) (err error) {
	var cfg preupgrade.Config

	if err = dbconn.QueryRow(ctx, "SHOW lc_collate").Scan(&cfg.Locale); err != nil {
		return
	}

	if err = dbconn.QueryRow(ctx, "SHOW server_encoding").Scan(&cfg.Encoding); err != nil {
		return
	}

	if err = dbconn.QueryRow(ctx, "SELECT current_setting('data_checksums')::bool").Scan(&cfg.DataChecksums); err != nil {
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

func readdatabases(ctx context.Context) (databases []string, err error) {
	rows, err := dbconn.Query(ctx, "SELECT datname FROM pg_database WHERE datallowconn")
	if err != nil {
		return
	}

	for rows.Next() {
		var database string
		if err = rows.Scan(&database); err != nil {
			return
		}

		databases = append(databases, database)
	}

	err = rows.Err()

	return
}

func analyzedatabase(ctx context.Context, database string) (err error) {
	log.Printf("Running ANALYZE on database '%s'", database)

	conn, err := connectdbname(ctx, database)
	if err != nil {
		return
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "ANALYZE")

	return
}

func postupgradefn(ctx context.Context) (err error) {
	databases, err := readdatabases(ctx)
	if err != nil {
		return
	}

	for _, database := range databases {
		if err = analyzedatabase(ctx, database); err != nil {
			return
		}
	}

	return
}

var modes = map[string]func(context.Context) error{
	preupgrade.ModeString:  preupgradefn,
	postupgrade.ModeString: postupgradefn,
}

func main() {
	var err error

	flag.Parse()

	opfunc := modes[*mode]
	if opfunc == nil {
		log.Fatal("Invalid mode selected")
	}

	ctx := context.Background()

	if err = connectdb(ctx); err != nil {
		log.Fatal(err)
	}
	defer dbconn.Close(ctx)

	if err = opfunc(ctx); err != nil {
		log.Fatal(err)
	}
}
