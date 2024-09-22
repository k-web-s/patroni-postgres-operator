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
	"log"
	"sync"
)

func readdatabases(ctx context.Context) (databases []string, err error) {
	dbconn, err := connectdb(ctx)
	if err != nil {
		return
	}
	defer dbconn.Close(ctx)

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

	conn, err := connectdb(ctx, connectwithdbname(database))
	if err != nil {
		return
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "ANALYZE")

	return
}

func updateextensionsindb(ctx context.Context, database string) (err error) {
	conn, err := connectdb(ctx, connectwithdbname(database))
	if err != nil {
		return
	}
	defer conn.Close(ctx)

	var qexts []string
	rows, err := conn.Query(ctx, "SELECT quote_ident(extname) FROM pg_catalog.pg_extension")
	if err != nil {
		return
	}
	for rows.Next() {
		var qext string
		if err = rows.Scan(&qext); err != nil {
			return
		}

		qexts = append(qexts, qext)
	}
	if err = rows.Err(); err != nil {
		return
	}

	for _, qext := range qexts {
		if _, err = conn.Exec(ctx, fmt.Sprintf("ALTER EXTENSION %s UPDATE", qext)); err != nil {
			return
		}
	}

	return
}

func updateextensions(ctx context.Context, databases []string) (err error) {
	errchan := make(chan error, len(databases))

	wg := &sync.WaitGroup{}
	for _, database := range databases {
		wg.Add(1)
		go func(db string) {
			defer wg.Done()

			if err := updateextensionsindb(ctx, db); err != nil {
				errchan <- err
			}
		}(database)
	}
	wg.Wait()

	select {
	case err = <-errchan:
	default:
	}

	return
}

func postupgradefn(ctx context.Context) (err error) {
	databases, err := readdatabases(ctx)
	if err != nil {
		return
	}

	if err = updateextensions(ctx, databases); err != nil {
		return
	}

	for _, database := range databases {
		if err = analyzedatabase(ctx, database); err != nil {
			return
		}
	}

	return
}
