---
name: skeleton-module
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
	_ foundation.Migratable = (*Module)(nil)
)
```

Those two lines are compile-time proof and they belong at the top of
`module.go`. Add one for every contract the module takes on, so the failure lands
here rather than at the registration in somebody else's repository.

## Taking on more than routes

Six interfaces sit beside `Module` in `framework/foundation`, one of which this
package already implements, and each is opted into the same way — by
implementing it. Confirm the set for the version in `go.mod` with:

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

**1. Register it in `Routes`, with a name.**

```go
	r.Action(stdhttp.MethodDelete, m.cfg.Prefix+"/{id}", m.destroy).Name("skeleton.destroy")
```

The name is what a URL is built from. Two spellings of one address disagree, and
the failure when they do is a link to a 404. The prefix is `m.cfg.Prefix` and
never a literal: the application decides where the package is mounted, and
`TestTheModuleRegistersItsRoutesUnderItsPrefix` at
`tests/Feature/routes_test.go:94` mounts it at `/widgets` and fails if any route
came out anywhere else. It also asserts every route is tagged with the module
name, which is what `aru route:list` groups by.

**2. Write the handler thin.** Read the input, ask the service, answer:

```go
func (m *Module) destroy(ctx *fhttp.Context) error {
	if err := m.svc.Delete(ctx.Ctx(), m.subject(ctx.Request), ctx.Param("id")); err != nil {
		return m.answer(ctx, err)
	}
	return ctx.Status(stdhttp.StatusNoContent)
}
```

`ctx.Status` and not `ctx.JSON` for an empty answer: `JSON` calls `ToArray()` on
what it is handed, so a nil resource panics inside the framework rather than
answering 204.

No rule and no statement lives in a handler. A handler that reached the
repository would be a handler that skipped the policy — read `skeleton-policy`
before writing the service method it calls.

**3. Let `answer` translate the refusal.** It knows three: `security.ErrForbidden`
becomes 403, `ErrNotFound` becomes 404, and a `validation.Errors` becomes 422
with the rejected field names, which are the client's own and give nothing away.
Anything else is *returned*, not swallowed — the framework turns it into the
error page in development and a 500 in production, which is the honest outcome.
Answering 200 with an empty body is the failure nobody debugs.

**4. Add the case to the route test.** `TestAVisitorWithNoSessionReachesNothing`
at `tests/Feature/routes_test.go:59` is a table of every route, and it asserts
403 for each. It runs against `data.Wrap(nil, data.DialectSQLite)` — a handle
over no database — so a route that got past the policy panics rather than
passes.

## Where the subject comes from

`m.subject(r)` loads the session and returns `security.Guest(m.cfg.Tenant)` when
there is none. Nothing else reads who is acting, and no handler takes a user id
from the request.

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
- **A default in `withDefaults`**, as a named constant beside `DefaultPrefix`
  and `DefaultPageSize`, if zero is meant to mean something. `withDefaults` runs
  *after* `Validate` and never before: filling a default in first hides the
  value somebody actually wrote from the check that would have refused it.

A value out of range is refused rather than clamped. A number somebody wrote and
did not get is worse than a number somebody wrote and was told about — that is
why `PageSize` above `MaxPageSize` is an error and not a silent 200.

Add the case to `TestTheConfigurationRefusesWhatCannotWork` at
`tests/Unit/policy_test.go:192`, which is a map of named bad configurations, and
to `TestNewRefusesAWiringThatCannotWork` at `tests/Feature/routes_test.go:119`
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

`With()` is what goes *beside* the fields at the top level. `ctx.JSON` puts what
`ToArray` returns under a fixed `"data"` key and merges what `With` returns next
to it, so `Collection` answers `{"data": {"items": [...]}, "next_cursor": "..."}`.
The cursor describes the answer rather than the things answered with. A resource
with nothing to add returns nil.

The cursor is only offered for a full page:

```go
	if len(records) == m.cfg.PageSize {
		cursor = records[len(records)-1].ID
	}
```

A short page is the last one, and offering a cursor for it offers a next page
that comes back empty.

## Adding a migration

Append it to the slice `Migrations()` returns, and give it a name that sorts
after the last one:

```go
func (createSkeletons) GetName() string { return "20260823_0001_create_skeletons" }
```

**The name carries the order and nothing else does.** `TestTheModuleDeclaresItsSchema`
at `tests/Feature/routes_test.go:140` requires the returned names to be sorted,
requires none to be empty, and requires every one to satisfy
`migrations.ReversibleMigration` — the migrator finds `Down` by type assertion,
so a `Down` with the wrong signature is a rollback that silently does nothing.

Three rules the existing migration keeps:

- **A name is fixed once the package is published.** Changing what an applied
  name means leaves the change missing everywhere it already ran, and nothing
  says so.
- **Types that spell the same in SQLite, PostgreSQL and MySQL.** Identifier
  columns are `VARCHAR(255)` rather than `TEXT` because they take part in a key
  and MySQL refuses `TEXT` in one without a prefix length. Timestamps get no
  database default: the value comes from Go, for the same reason ids do —
  `gen_random_uuid`, `UUID()` and `randomblob` are three spellings of one idea.
- **An index that matches the `ORDER BY` of the listing, tenant first.** Without
  it every page is a scan of every customer's rows.

Migrations do not run at boot. `aru migrate` is a step in the installer's
pipeline, and every migration has to be compatible with the previous binary
during a rollout: a new column is nullable or has a default, and removing one
takes two releases.
