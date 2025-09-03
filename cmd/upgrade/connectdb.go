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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/namsral/flag"
)

var (
	dbhost     = flag.String("dbhost", "", "postgresql host")
	dbport     = flag.Int("dbport", 5432, "postgresql port")
	dbuser     = flag.String("dbuser", "postgres", "postgres user")
	dbpassword = flag.String("dbpassword", "", "postgres password")
	dbname     = flag.String("dbname", "postgres", "postgres database name")
	mode       = flag.String("mode", "", "operation mode")
)

const (
	connectTimeout = 100 * time.Millisecond
)

type connectoption interface {
	apply(*pgx.ConnConfig)
}

type connectwithhostname string

func (h connectwithhostname) apply(c *pgx.ConnConfig) {
	c.Host = string(h)
}

type connectwithdbname string

func (d connectwithdbname) apply(c *pgx.ConnConfig) {
	c.Database = string(d)
}

// connectdb connects to a database within connectTimeout
func connectdb(ctx context.Context, options ...connectoption) (*pgx.Conn, error) {
	config, err := pgx.ParseConfig(
		fmt.Sprintf(
			"host=%s port=%d user=%s password=%s database=%s sslmode=disable",
			*dbhost, *dbport, *dbuser, *dbpassword, *dbname))
	if err != nil {
		return nil, err
	}

	config.ConnectTimeout = connectTimeout

	for _, opt := range options {
		opt.apply(config)
	}

	retries := 10
	for {
		conn, err := pgx.ConnectConfig(ctx, config)
		if err == nil {
			return conn, nil
		}
		if retries == 0 {
			return nil, err
		}
		time.Sleep(time.Second)
		retries--
	}
}

// connectprimarydb connects to primary database. Returns connection or
// when context cancelled.
func connectprimarydb(ctx context.Context) (conn *pgx.Conn, err error) {
	t := time.NewTicker(connectTimeout)
	defer t.Stop()

	for {
		conn, err = connectdb(ctx)
		if err == nil {
			var pg_is_in_recovery bool
			if err = conn.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&pg_is_in_recovery); err == nil && !pg_is_in_recovery {
				return
			}

			conn.Close(ctx)
		}

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
