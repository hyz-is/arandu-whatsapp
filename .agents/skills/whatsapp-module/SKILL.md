---
name: whatsapp-module
description: Change what this Arandu package registers — its routes, handlers, configuration, response shape or schema. Use when the request is to "add a route", "add an endpoint", "add a handler", "add a config option", "change the prefix", "add a field to the response", "add a column", "write a migration", "return the tenant id too", "add a background job to the package", "make it run something at boot", or when a change touches module.go, config.go or model.go. Covers foundation.Module and the five optional interfaces beside it, why handlers are thin, the migration name that carries the order, and what Resource is for.
license: MIT
---

# The module contract

`foundation.Module` is `Name()` and `Routes()`, and nothing else. That pair is
the whole public contract between a package and the framework: an application
calls `New`, gets something that satisfies it, and registers it by hand.

```go
var (
	_ foundation.Module     = (*Module)(nil)
	_ foundation.Bootable   = (*Module)(nil)
	_ foundation.Background = (*Module)(nil)
	_ foundation.Health     = (*Module)(nil)
	_ foundation.Closable   = (*Module)(nil)
	_ foundation.Migratable = (*Module)(nil)
)
```

Those two lines are compile-time proof and they belong at the top of
`module.go`. Add one for every contract the module takes on, so the failure lands
here rather than at the registration in somebody else's repository.

## Taking on more than routes

Six optional interfaces sit beside `Module` in `framework/foundation`. This
package implements `Bootable`, `Background`, `Migratable`, `Health` and
`Closable`; only `Schedulable` is not part of its lifecycle. Each is opted into
by implementing it. Confirm the set for the version in `go.mod` with:

```sh
export GOWORK=off
go doc github.com/arandu-io/framework/foundation
```

| interface | what it is for |
| --- | --- |
| `Bootable` | prepare state once, at boot |
| `Background` | run a loop of the module's own |
| `Schedulable` | declare work for the scheduler |
| `Migratable` | own tables — this package implements it |
| `Health` | report on the storage it depends on |
| `Closable` | give resources back at shutdown |

Two of them change what `arandu.mod.toml` has to say. A `Background` loop that
calls out needs `network = true`; anything that writes a file needs
`filesystem = true`. Declare it in the same commit as the code, or the
application that installs this package fails `aru doctor` with
`permission-not-declared` and the person reading it has no idea why.

## Adding a route

**1. Register it in `registerRoutes`, with a name.**

```go
	register(stdhttp.MethodDelete, prefix+"/{instance}/resource/{id}", "resource.destroy", m.destroy)
```

The helper adds the stable `whatsapp.` name prefix. Paths derive from
`m.cfg.Prefix`; `TestCustomPrefixAppliesToEveryRoute` fails if one escapes it.

**2. Write the handler thin.** Read the input, ask the service, answer:

```go
func (m *Module) destroy(ctx *fhttp.Context) error {
	if _, err := m.service.DeleteInstance(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("instance"), false); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}
```

`ctx.Status` and not `ctx.JSON` for an empty answer: `JSON` calls `ToArray()` on
what it is handed, so a nil resource panics inside the framework rather than
answering 204.

No rule and no statement lives in a handler. A handler that reached the
repository would be a handler that skipped the policy — read `whatsapp-policy`
before writing the service method it calls.

**3. Let `answer` translate known domain refusals.** `statusForError` is the
single mapping for policy, validation, not-found, conflict, payload/media and
WhatsApp availability errors. Policy details are reduced to a generic 403;
known client errors keep safe domain messages, and 5xx responses use only the
HTTP status text. Unknown errors are *returned*, not swallowed, so the
framework owns the 500 response and logging.

**4. Add the case to both route-surface tests.**
`TestModuleRegistersCanonicalRouteSurface` asserts the exact method/path/name
set, and the full guest-refusal test exercises all 36 routes and expects 403
before persistence. Both live in `tests/Feature/routes_test.go`.

## Where the subject comes from

`m.subject(r)` loads the session and returns `security.Guest(m.cfg.Tenant)` when
there is none. The service then requires every authenticated subject to match
the configured runtime tenant. Nothing reads identity or tenant from request
fields.

The guest's tenant is the one place in this package where a tenant does not come
from a Grant, and it is because there is no Grant yet. It comes from
`Config.Tenant`, which is the application's own configuration — never from the
request.

## Adding a configuration field

`Config` is a typed struct, and that is the point: a misspelled key in a map is
a setting that silently keeps its default, and here a field that does not exist
does not compile.

Three things move together for every new field.

- **A doc comment on the field**, saying what it means and where it may come
  from.
- **A rule in `Validate`** if there is a value it cannot be. `New` calls
  `Validate` before anything else, so a setting that cannot work fails where it
  is wired rather than on the first request that needed it.
- **A default in `withDefaults`**, beside the related configuration, if zero is
  meant to mean something. `withDefaults` runs
  *after* `Validate` and never before: filling a default in first hides the
  value somebody actually wrote from the check that would have refused it.

A value out of range is refused rather than clamped. Keep timeout, queue, media
limit, color, URL and policy-action validation in `Config.Validate`.

Add the case to `TestConfigurationValidationAndDefaults`, which exercises bad
configurations, and to `TestNewRefusesInvalidWiringAndAcceptsStructuralHandle`
if the field can make `New` fail.

## What may leave in a response

`Resource` and `Collection` in `model.go` are declared lists of fields, not the
entity. An encoder handed the entity answers with whatever fields it happens to
have, including the ones somebody adds later without ever opening the handler —
and `TenantID` is exactly such a field. It names another customer's identifier
and belongs in no response.

So a new column that should be visible is added in two places: the struct in
`model.go`, and the map `ToArray` returns. A column that should not be visible
is added in one.

`With()` is what goes beside the fields at the top level. `ctx.JSON` puts what
`ToArray` returns under `data`. Payload adapters must propagate serialization
errors; they may not answer 200 with `{}` after a failed marshal.

## Adding a migration

Append it to the slice `Migrations()` returns, and give it a name that sorts
after the last one:

```go
func (addWhatsappFeature) GetName() string { return "20260825_0003_add_whatsapp_feature" }
```

**The name carries the order and nothing else does.**
`TestModuleDeclaresCanonicalMigrations` requires the exact ordered declaration.
A schema change implements `Down` only when it can genuinely reverse `Up`, and
then carries a compile-time `migrations.ReversibleMigration` assertion. A data
repair or delegated upstream upgrade with no inverse does not declare `Down`.

Three rules the existing migration keeps:

- **A name is fixed once the package is published.** Changing what an applied
  name means leaves the change missing everywhere it already ran, and nothing
  says so.
- **Types and statements supported by SQLite and PostgreSQL.** This module does
  not support MySQL because the WhatsMeow SQL store does not. Timestamps get no
  database default: the value comes from Go, for the same reason ids do —
  `gen_random_uuid`, `UUID()` and `randomblob` are three spellings of one idea.
- **An index that matches the `ORDER BY` of the listing, tenant first.** Without
  it every page is a scan of every customer's rows.
- **A WhatsMeow dependency bump gets a new delegated migration.** The upstream
  container remains the schema source, but an applied Foundation migration does
  not run again merely because the dependency changed.

Migrations do not run at boot. `aru migrate` is a step in the installer's
pipeline, and every migration has to be compatible with the previous binary
during a rollout: a new column is nullable or has a default, and removing one
takes two releases.
