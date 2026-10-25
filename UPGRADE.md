# Upgrade guide

What changed in a way that stops your code compiling, and what to write
instead.

Additions are not listed. A new symbol breaks nothing, and a file that named
every one of them would be a changelog nobody reads to find the two lines that
matter.


## v0.8.0 -- Go 1.26.10 is the floor, on Framework 0.66 and Hesape 0.85

`go.mod` declares `go 1.26.10`. Every vulnerability an Arandu application
reaches is in the Go standard library (crypto/x509, the HTTP/2 and CONNECT
handling in net/http, html/template, net/textproto and crypto/tls) and is
fixed in Go 1.26.10, so a module that declared less let a build run on a
toolchain that still carries them. Build with Go 1.26.10 or later.

The package compiles against newer Arandu modules, and `arandu.mod.toml` now
declares `framework = ">= 0.66"`:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.57.0` | `v0.66.0` |
| `github.com/arandu-io/hesape` | `v0.59.0` | `v0.85.0` |
| `github.com/hyz-is/arandu-swagger` | `v0.4.5` | `v0.6.0` |

```bash
go get github.com/arandu-io/framework@v0.66.0
go get github.com/arandu-io/hesape@v0.85.0
go get github.com/hyz-is/arandu-swagger@v0.6.0
```

An application below those follows the Framework, Hesape and Swagger upgrade
guides between the versions it had and these first. No route, migration,
action, tenant rule or table of this package changes.

### Signatures that read the same and compile unchanged

`apidiff` against `v0.7.0` names six incompatible changes, and none of them
asks anything of a caller. Each one takes or returns a type the Framework now
declares as an alias of the Hesape type (`security.SessionStore` is Hesape's
`http.SessionStore`, `http.Router` is Hesape's controller router), so the
signature reads the same and the value an application already passes is the
same type:

- `New` and `NewWithDocumentation`, which take the `*security.SessionStore`.
- `(*Module).Routes`, which takes the `*http.Router`.
- `app/Http/Controllers.NewWhatsAppController`, which takes the
  `*security.SessionStore`.
- `routes.Web` and `routes.WebDocumented`, which take the `*http.Router`.

### `InstancePolicy` says which actions write

`InstancePolicy` implements `auth.Writes`: `whatsapp.instance.list`,
`whatsapp.instance.view` and `whatsapp.webhook.view` read, and a Grant issued
for any of them is refused by every write with `auth.ErrReadGrant`. Every other
action writes, including the ones named for reading (the connection state, the
message list, a media download, a contact check, a profile picture and a group
invite), because their flow reopens a logged-out instance or stores the address
mapping a lookup resolved. Nothing this package does writes with a read Grant,
so the routes and the service behave as they did.

## v0.7.0 — the Framework floor is 0.57, on Hesape 0.59

Nothing in this package's own API moved: `apidiff` against `v0.6.3` reports no
incompatible change. What moved is the minimum it compiles against, which
`go.mod` and `arandu.mod.toml` now declare:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.56.0` | `v0.57.0` |
| `github.com/arandu-io/hesape` | `v0.54.0` | `v0.59.0` |

```bash
go get github.com/arandu-io/framework@v0.57.0 github.com/arandu-io/hesape@v0.59.0
```

Framework v0.57.0 requires Hesape v0.59.0, so an application moves both
together, following the Framework and Hesape upgrade guides between the two
versions. Hesape v0.59.0 renames the value types of `hesape/webhook.Manager`
(`webhook.Endpoint` is `webhook.ManagerEndpoint`, `webhook.Delivery` is
`webhook.ManagerDelivery`, and the `Store`, `Claim` and `EndpointResolver`
signatures that named them follow); that reaches an application only where it
calls `hesape/webhook.Manager` itself. This package's delivery store is
internal and already follows the rename.

The webhooks this package sends are unchanged on the wire. They stay on the
`whatsapp-webhooks` queue as `whatsapp.webhook.deliver` jobs with the
`{"deliveryId"}` payload and five tries 5 s, 30 s, 2 min and 10 min apart, in
the `whatsapp_webhook_deliveries` table, with the same headers
(`X-Arandu-Delivery-Id`, `X-Arandu-Timestamp`, `X-Arandu-Signature`,
`x-request-id`, `x-owner-jid`, `x-instance-name`, `x-instance-id`,
`x-webhook-event`, `Content-Type`, `User-Agent`), the same body bytes and the
same signature. What changes is the attempt, which is now Hesape's delivery
engine, and a webhook target sees three differences:

- A URL that resolves into the shared address space 100.64.0.0/10 (carrier-grade
  NAT, and Tailscale addresses) is refused like a private address and recorded
  as `unsafe_destination`, permanently. Point the webhook at a public address.
- The transport no longer uses a proxy from `HTTP_PROXY`, `HTTPS_PROXY` or
  `NO_PROXY`. A deployment that reached its webhook targets only through an
  environment proxy no longer reaches them.
- An answer is read up to 1 MiB and decided by its status, so a 2xx with a
  longer body is delivered instead of failing `response_too_large`. A body
  that cannot be read after the status arrived keeps that status in
  `response_status`.

## v0.5.0 (multi-tenant)

- An application that registered one module per tenant can register one. A
  subject from any tenant reaches the instances of its own tenant; nothing
  else changes in the policy configuration.
- `Config.Tenant` stays required and keeps its validation. It is now the guest
  realm and the tenant the boot discovery runs under, not a gate.
- To receive inbound messages without a webhook, call `OnInbound` after `New`
  and before `Start`.

## Before v1.0.0

While the version starts with `v0.`, the API can break. That is what `v0.`
means in Go, and it is deliberate: the alternative is freezing a shape before
anybody has installed it. What is not deliberate is breaking it quietly, which
is what this file exists to stop.

Every release from here is compared against the one before it by `apidiff` in
CI. An incompatible change with no entry here fails the build, and the entry
has to name the symbol.

---

## v0.4.5 — the Framework floor is 0.56

Nothing in this package's own API moved: `apidiff` against `v0.4.4` reports no
incompatible change. What moved is the minimum it compiles against, which
`go.mod` and `arandu.mod.toml` now declare:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.55.1` | `v0.56.0` |
| `github.com/arandu-io/hesape` | `v0.52.0` | `v0.54.0` |

```bash
go get github.com/arandu-io/framework@v0.56.0 github.com/arandu-io/hesape@v0.54.0
```

An application below those upgrades them first, following the Framework and
Hesape upgrade guides between the two versions. The one incompatible change
upstream is in the Framework `config` bridge: `config.Config.SessionTTL` is
removed and `config.Load` no longer reads `SESSION_TTL`, so an application
that built its session store from it builds the store from
`bootstrap.LoadConfiguration` and `SESSION_LIFETIME`, in minutes. Every page
drawn through `view.New` now carries `APP_NAME` without being passed it; this
package draws no page, so nothing here changes with it.

## v0.4.4 — Arandu Swagger 0.4

Nothing in this package's own API moved: `apidiff` against `v0.4.3` reports no
incompatible change. `go.mod` now requires `github.com/hyz-is/arandu-swagger`
`v0.4.2`, was `v0.3.1`. The Framework floor stays at 0.55.

```bash
go get github.com/hyz-is/arandu-swagger@v0.4.2
```

Swagger's own API between the two versions only gained symbols, so the
`swagger.Config` in `bootstrap/app.go` compiles as written. Two things an
application sees at run time: the generated document declares
`https://spec.openapis.org/oas/3.1/dialect/base` as its `jsonSchemaDialect`
unless `Config.JSONSchemaDialect` names another, and the UI route first renders
the application's `docs.swagger` view, falling back to the embedded page when
the application has none.

## v0.4.3 — the Framework floor is 0.55

Nothing in this package's own API moved: `apidiff` against `v0.4.2` reports no
incompatible change. What moved is the minimum it compiles against, which
`go.mod` and `arandu.mod.toml` now declare:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.48.0` | `v0.55.1` |
| `github.com/arandu-io/hesape` | `v0.42.2` | `v0.52.0` |
| `golang.org/x/net` | `v0.58.0` | `v0.60.0` |

An application below those upgrades them first, following the Framework and
Hesape upgrade guides between the two versions: from Framework v0.55.0 the
session is configured only by what the session store reads, and from v0.54.0 a
boolean setting that does not read as one stops the boot. The `x/net` floor
closes GO-2026-6611, GO-2026-6612 and GO-2026-6617.

## v0.4.2 — the Framework floor is 0.48

Nothing in this package's own API moved. What moved is the minimum it compiles
against, which the dependency update after v0.4.1 raised in `go.mod` and
`arandu.mod.toml` now declares:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.47.1` | `v0.48.0` |
| `github.com/arandu-io/hesape` | `v0.41.1` | `v0.42.2` |

An application below those upgrades them first. `aru skills:sync` now offers
`whatsapp-package` to a project that requires this version.

## Unreleased — the Framework floor is 0.45

Nothing in this package's own API moved. `apidiff` against `v0.1.0` reports no
incompatible change, so no call site here needs rewriting.

What moved is the minimum this package compiles against:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.35.0` | `v0.45.0` |
| `github.com/arandu-io/hesape` | `v0.12.0` | `v0.24.0` |

An application below those cannot install this version: Go resolves one version
per module, so the floor here becomes the application's floor. Upgrade the
application first, work through the two upstream upgrade guides, then take this
release.

The one upstream change that reached this package was `hesape/image`, whose
`(*Image).ToBytes` and `(*Image).Dimensions` each take a `context.Context`
first. It is named here because an application that builds its own thumbnails
alongside this package hits it in its own code, not because anything in this
package's surface exposes it.

### The owned schema is built by the Blueprint

The three package-owned migrations compile their DDL through
`hesape/database/schema` rather than holding SQL strings. **The names `GetName`
returns did not change**, so a database that has already applied them has
nothing to do: the migrator matches on that name, and every one of the four is
the name it was.

A database migrated by this release rather than by `v0.1.0` differs in what the
engine chose for it, not in what it holds:

- Every unique constraint, index and foreign key is named. They were anonymous
  table constraints before, so Postgres named them itself; a runbook that drops
  one by the name Postgres generated names something that no longer exists.
- On SQLite the declared column types are the ones its own grammar writes —
  `integer` rather than `BIGINT`, `datetime` rather than `TIMESTAMP`. SQLite
  assigns both the same affinity, and no stored value reads back differently.

Timestamp precision, column widths, nullability and every foreign key's
composite tenant column are unchanged on both engines.
