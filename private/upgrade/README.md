# Upgrade procedure

The operator does the upgrade using PostgreSQL's pg_upgrade --link mode. Usually this results in little downtime. The upgrade has the following stages:

- preupgrade
- preupgrade-sync
- scaledown
- primary-upgrade
- secondary-upgrade
- primary-upgrade-move
- postupgrade

## Upgrade stages

### Preupgrade

Database settings which are needed for `initdb` are extracted from current running database.

Database is reachable in this stage.

[preuprgrade.go](../../cmd/upgrade/preupgrade.go)

### Preupgrade-sync

Database is reconfigured to listen on different port (55432) to ensure no clients are connected. Then, the operator starts a job to monitor replicas that they are caught up with primary. It does this by issuing CHECKPOINT on primary, then waiting for wal to be replicated, then issuing CHECKPOINT in replicas. Then repeat this cycle until WAL position does not change on primary.

During this, database is not reachable.

[preuprgrade-sync.go](../../cmd/upgrade/preupgrade-sync.go)

### Scaledown

Database is stopped in this phase.

[scaledown.go](scaledown.go)

### Primary upgrade

Primary node is upgraded, and new database system id is returned. Upgraded database is in `data.new`, which will be moved to its final location later.

[primary-upgrade](upgrade-scripts/primary-upgrade)

### Secondary upgrade

Secondaries are upgraded as [recommended](https://www.postgresql.org/docs/current/pgupgrade.html#:~:text=Upgrade%20streaming%20replication%20and%20log%2Dshipping%20standby%20servers).

[secondary-upgrade](upgrade-scripts/secondary-upgrade)

### Primary upgrade move

The new database in `data.new` on primary is moved back to its final `data` directory.

[primary-upgrade-move](upgrade-scripts/primary-upgrade-move)

### Postupgrade

Database is started, can accept connections. After available, the operator updates all extensions in all databases, then runs `ANALYZE` on all databases.

[postupgrade.go](../../cmd/upgrade/postupgrade.go)
