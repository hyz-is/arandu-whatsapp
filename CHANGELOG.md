# Release Notes

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
