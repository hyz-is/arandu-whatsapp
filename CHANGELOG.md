# Release Notes

## v0.8.3 - 2026-11-22

### Added
- `InboundMessage` says who sent a message and where it came from, so a
  listener can match a sender that WhatsApp addresses by a private LID:
  `SenderPhone` (digits with the country code, no plus sign; the number
  WhatsApp delivered, or the one the WhatsMeow LID store maps the LID to, and
  empty while the LID has no known number), `SenderJID` (the address as
  delivered) and `SenderLID`. A message from a private LID used to reach the
  listener with an empty `RemoteJID` and no number at all.
- `InboundMessage.IsBroadcast` and `BroadcastListJID` mark a message that came
  through a broadcast list, and `GroupJID` names the group a message was
  posted in. `IsGroup` is unchanged: it is still WhatsApp's flag, true for a
  group and for a broadcast list.
- `Service.ListGroups` lists the groups an instance has joined (`ID`, `Subject`,
  `Size`, `Owner`, `Creation`, `IsCommunity`), without the participants, through
  the new `whatsapp.group.list` action (`ActionGroupList`). Like every action,
  it stays closed until `Policy.Roles` or `Policy.Actions` lists it.

## [v0.8.2](https://github.com/hyz-is/arandu-whatsapp/compare/v0.8.0...v0.8.2) - 2026-11-22

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.8.0...v0.8.2

## v0.8.2 - 2026-11-22

### Fixed
- A paired session is no longer left disconnected for good when its restore
  lost a race at boot. Restore ran once: if another container held the
  instance's connection lease (two minutes, taken while one connects), that
  instance was skipped without a line in the log and never tried again, so
  three containers started within ninety seconds of each other (consecutive
  zero-downtime deploys) could leave every paired session in the database
  and none in memory. `Start` now keeps a supervisor running that, every
  `WhatsApp.RestoreInterval` (default 30 seconds, up to 20% jitter), tries the
  paired instances this process holds no client for and connects the ones whose
  lease is free or expired. It never pairs and never asks for a QR code, and it
  skips instances whose stored status is `logged_out` or `session_missing`.
- A restore skipped because the lease is held is logged once (`WhatsApp restore
  skipped: the connection lease is held by another process; it will be
  retried`), and the success is logged when the instance later connects.
- Two processes can no longer connect the same session. The lease was released
  the moment a client connected, so a container booting beside a connected one
  took the free lease and replaced the other's WhatsApp stream. A connected
  client now keeps its lease, and every `RestoreInterval` the process renews
  the lease of each client it holds. A process whose lease was taken over (it
  stalled past the two minutes) disconnects its client rather than fight for the
  socket.
- `Close` disconnects the clients first and releases their leases afterwards,
  with a deadline of its own, so the next container takes the sessions over at
  once and not after the lease expires. A process that dies without closing
  loses them when the lease expires, at most two minutes.
- `Health` reports a restore failure only until a later pass succeeds.
- A restore that cannot read the WhatsMeow store no longer marks the instance
  `session_missing`, which says the device is gone. It stores `connection_error`
  and the supervisor tries again; only a store that answers there is no device
  stores `session_missing`.

### Added
- `Config.WhatsApp.RestoreInterval`: the period of the restore supervisor and of
  the lease renewal. Zero uses 30 seconds; it cannot exceed one minute and a
  negative value is refused.

### Changed
- An instance connected in one process is owned by that process: `ConnectQRCode`
  and `ConnectPhone` called on another container answer connection in progress,
  and logging out or deleting it there answers `ErrConnectionInProgress`, where
  they used to take the free lease and open a second connection. Send the
  request to the container that holds the connection, or let the supervisor move
  the session when that container goes away.

## v0.8.1 - 2026-11-13

### Fixed
- The WhatsMeow store upgrade no longer fails with `relation "whatsmeow_version"
  does not exist` when the connection's schema is not the only one holding a
  table of that name (a per-test schema, a development database that also has
  `public`). On PostgreSQL the migration creates the version table in the
  connection's own schema before the upstream upgrader runs.
- `Start` no longer fails when the schema has not been migrated. The process
  boots, the error is logged, `Module.Available` returns `ErrSchemaUnavailable`,
  operations answer an error matching `ErrWhatsAppUnavailable`, and the module
  recovers by itself once `aru migrate` has run. A new image booting before the
  migration used to take the whole application down.

### Added
- `Module.Available` and `ErrSchemaUnavailable`.

## [v0.8.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.7.0...v0.8.0) - 2026-10-25

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.7.0...v0.8.0

## [v0.7.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.3...v0.7.0) - 2026-10-15

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.3...v0.7.0

## [v0.7.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.3...v0.7.0) - 2026-10-15

### Changed
- Requires `github.com/arandu-io/framework` v0.57.0 and
  `github.com/arandu-io/hesape` v0.59.0 (were v0.56.0 and v0.54.0);
  `arandu.mod.toml` declares the Framework floor `>= 0.57`. The module's API
  is unchanged.
- Webhook attempts run on Hesape's delivery engine through the `Manager`
  facade. The wire is the same -- queue, job, payload, retries, headers, body
  and signature, pinned by `TestDeliveryWireIsPinned` -- and a target sees
  three differences: an address in 100.64.0.0/10 is refused as
  `unsafe_destination`, no proxy is taken from the environment, and a 2xx
  answer longer than 1 MiB is delivered instead of failing
  `response_too_large`. `UPGRADE.md` has the details.

## [v0.6.3](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.2...v0.6.3) - 2026-10-11

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.2...v0.6.3

## [v0.6.3](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.2...v0.6.3) - 2026-10-11

### Fixed
- `ConnectQRCode` called while the instance is already pairing now answers
  the QR code the pairing shows at that moment (`Count`, `Code`, `Base64`,
  status `qrcode`), alongside `AlreadyConnecting`. It answered connecting with
  no image, so a screen could only show the first code, which WhatsApp
  replaces after about twenty seconds: polling now keeps up with the rotation
  until the phone scans it.

## [v0.6.2](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.1...v0.6.2) - 2026-10-11

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.1...v0.6.2

## [v0.6.2](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.1...v0.6.2) - 2026-10-11

### Fixed
- `migrate:rollback`, `migrate:reset` and `migrate:refresh` of an application
  that installs the module no longer stop on
  `20260825_0002_upgrade_whatsmeow_store`. It declares Irreversible: WhatsMeow
  owns that schema and publishes no downgrade, so the store is left applied and
  the module's own tables are rolled back around it.

## [v0.6.1](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.0...v0.6.1) - 2026-10-11

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.0...v0.6.1

## [v0.6.1](https://github.com/hyz-is/arandu-whatsapp/compare/v0.6.0...v0.6.1) - 2026-10-11

### Fixed
- A process that builds the module more than once -- an application whose
  tests boot it again -- no longer panics registering
  `20260825_0002_upgrade_whatsmeow_store` twice. The module registers that
  migration without an upgrader, and it opens the WhatsMeow store over the
  migrator's own handle when it runs, so every module declares the same value.

## [v0.6.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.5.0...v0.6.0) - 2026-10-11

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.5.0...v0.6.0

## [v0.6.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.5.0...v0.6.0) - 2026-10-11

### Added
- `PolicyConfig.Actions`: actions decided by the subject's own actions. A
  subject that carries a listed action may take it, for applications whose
  permission groups fill the subject's actions (arandu-permission) rather than
  its roles. Roles work as before, and an action in neither list stays closed.

## [v0.5.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.5...v0.5.0) - 2026-10-11

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.5...v0.5.0

## [v0.5.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.5...v0.5.0) - 2026-10-11

### Added
- One registration hosts every tenant of the application. `Start` discovers the
  tenants with paired online instances (`ActionRuntimeDiscover`) and restores
  each one under a runtime Grant of that tenant.
- `Module.OnInbound` and `InboundMessage`: an in-process listener told about
  every persisted inbound message with the runtime Grant of its tenant.

### Changed
- A subject is no longer refused for carrying a tenant other than
  `Config.Tenant`; the policy decides against the instance's tenant. A subject
  with no tenant is still refused before authorization or SQL.
- Restored instances run under the module's lifecycle context, so `Close`
  stops them the same way as paired ones.

## [v0.4.5](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.4...v0.4.5) - 2026-10-10

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.4...v0.4.5

## [v0.4.4](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.3...v0.4.4) - 2026-10-09

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.3...v0.4.4

## [v0.4.3](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.2...v0.4.3) - 2026-10-09

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.2...v0.4.3

## [v0.4.2](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.1...v0.4.2) - 2026-10-09

## What's Changed
* build(deps): bump the gomod group across 1 directory with 4 updates by @dependabot[bot] in https://github.com/hyz-is/arandu-whatsapp/pull/8

## New Contributors
* @dependabot[bot] made their first contribution in https://github.com/hyz-is/arandu-whatsapp/pull/8

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.1...v0.4.2

## [v0.4.1](https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.0...v0.4.1) - 2026-09-17

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.4.0...v0.4.1

## [v0.4.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.3.0...v0.4.0) - 2026-09-14

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.3.0...v0.4.0

## [v0.3.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.2.0...v0.3.0) - 2026-09-09

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.2.0...v0.3.0

## [v0.2.0](https://github.com/hyz-is/arandu-whatsapp/compare/v0.1.0...v0.2.0) - 2026-09-07

**Full Changelog**: https://github.com/hyz-is/arandu-whatsapp/compare/v0.1.0...v0.2.0

## v0.1.0 - 2026-08-26

### Added

- Initial Arandu module with 36 WhatsApp API routes.
- Arandu session authentication and default-deny role policy.
- Tenant-scoped PostgreSQL and SQLite repositories.
- Four ordered Foundation migrations: three reversible package schema
  migrations and one non-reversible WhatsMeow store upgrade delegated to its
  upstream container.
- Durable webhook delivery snapshots and retries through Hesape's native
  database queue, with a stable `X-Arandu-Delivery-ID` idempotency key.
- Durable mention-all snapshots and native processing/retention jobs whose
  serialized payload contains only the process id.
- Explicit Boot, Start, Health and Close lifecycle integration.
- Typed WhatsApp, persistence, webhook, processing and media configuration.
- Native Arandu Swagger integration that publishes the 36-route OpenAPI 3.1
  contract from the runtime route table, plus messaging, Passkey, webhook and
  migration guides.

### Changed

- Instance listing now uses tenant-scoped keyset pagination with an opaque
  `nextCursor`, a default page size of 200 and a hard maximum of 200.
- Phone-code pairing now accepts `phoneNumber` in a validated JSON body at
  `POST /instances/{instance}/connection/phone`; the phone-in-path route was
  removed.

### Fixed

- Public service and repository errors now expose stable package sentinels for
  `errors.Is` without requiring consumers to import internal packages.

Written by the release, not by hand. `.github/workflows/release.yml` publishes
the release a tag names and prepends what it said, so the tag, the release and
this file cannot disagree.

Tags cut before that workflow existed have no release and are not listed here.
`git tag` is where they are.
