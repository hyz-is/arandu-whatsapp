# Changelog

Everything worth knowing about a release of WhatsApp is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

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
