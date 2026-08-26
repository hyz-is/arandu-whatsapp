# Migrations

The WhatsApp package exposes its schema through `foundation.Migratable`. The
host application applies it with Arandu's migration command:

```bash
aru migrate
```

The module never runs schema changes from `New`, `Boot` or `Start`. This keeps
route inspection, tests and multi-replica startup free from migration races.

## Supported databases

The runtime supports PostgreSQL and SQLite. SQLite must have foreign keys
enabled before the module starts:

```sql
PRAGMA foreign_keys = ON;
```

MySQL is intentionally rejected because the WhatsMeow SQL store does not offer
the same supported runtime contract.

## Declared migrations

The module returns exactly four ordered Foundation migrations:

1. `20260825_0001_create_whatsapp_tables` creates the final tenant-scoped
   `whatsapp_*` domain schema. It has a real `Down` and is reversible.
2. `20260825_0002_upgrade_whatsmeow_store` delegates to the
   `sqlstore.Container` built by `New`. WhatsMeow owns that schema and its
   upgrade sequence. The migration runs outside Arandu's transaction and has no
   `Down`, because the upstream upgrader does not expose a reversible operation.
3. `20260825_0003_create_webhook_deliveries` creates the tenant-scoped durable
   snapshots used by webhook jobs. Successful rows are compacted immediately,
   all snapshots expire through the configured retention window, and its
   tenant/creation index serves that cleanup. It has a real `Down` and is
   reversible.
4. `20260825_0004_create_message_jobs` creates the tenant-scoped mention-all
   snapshots. The native queue payload carries only `processId`, while this
   table holds the prepared protobuf and metadata beyond the queue's payload
   ceiling. A second native job expires recoverable snapshots and their
   processing jobs after `Config.Processing.Retention`; successful and
   durably-notified terminal results remove that cleanup job immediately. It
   has a real `Down` and is reversible.

The first migration includes the connection and address-mapping tables, event
metadata and the final unique indexes. Legacy data repairs are not replayed on
a clean package installation: the tables have just been created and contain no
legacy rows to repair.

All package-owned domain rows include `tenant_id`. Child tables use composite
foreign keys containing the tenant and every read, update and delete is scoped
with the tenant obtained from an Arandu Grant.

The `jobs` table is not package-owned. It comes from the Hesape
`queue.DatabaseQueue` module registered by the host application, whose
migrations must be present in the same `aru migrate` run. Dispatch creates each
webhook delivery or mention-all snapshot and its native job in one database
transaction. Mention-all dispatch creates both its processing and retention
jobs in that same transaction.

## WhatsMeow ownership

The package does not copy WhatsMeow's SQL schema. The delegated migration calls
the upstream `Container.Upgrade`, so the dependency remains the single source
of its SQL and upgrade rules. When the pinned WhatsMeow version changes, the
package must add a new ordered delegating migration; an applied Foundation
migration name is immutable and is not run again.

`aru migrate --pretend` does not invoke the upstream upgrader. Its SQL is opaque
to the Foundation connection, so the delegated migration prints no individual
WhatsMeow statements in pretend mode.

## Existing installations

An existing compatible WhatsMeow store is upgraded by the upstream container.
Data from the legacy standalone API requires an explicit import plan before
enabling this module. Its schema has no Arandu tenant key, so an importer must
receive the destination tenant and verify device ownership. The module does not
guess a tenant or silently adopt legacy domain rows.

Once a Foundation migration name is published, do not change its meaning. Add
a new, ordered migration for every package-owned schema change.

## Rollback boundary

Never roll back a batch that includes
`20260825_0002_upgrade_whatsmeow_store`. The upstream upgrader has no reverse
operation, so this package can only roll forward across that boundary. Limit a
rollback to a newer reversible migration, or restore the complete database from
a backup. The current Hesape migrator can otherwise remove an irreversible
migration from its history without changing the upstream schema.

## Startup errors

`Start` verifies that `whatsmeow_version` exists and contains a version row. If
it does not, startup fails before WhatsApp connection restoration begins and
the error instructs the operator to run `aru migrate`. Version compatibility itself
belongs to the upstream upgrader and is not duplicated as a hardcoded check.

## Verification

The package test suite runs all four migrations through Hesape's official
migrator against SQLite, checks tenant constraints and delegation to WhatsMeow,
and uses the migrator's tracked batch rollback to remove the newest message-job
migration without crossing the irreversible WhatsMeow migration:

```bash
GOWORK=off go test -run 'TestMigrations' ./...
```
