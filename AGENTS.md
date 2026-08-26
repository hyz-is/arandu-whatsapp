# Working on WhatsApp

This is an Arandu WhatsApp module with an explicit lifecycle, 36 HTTP routes,
tenant-scoped persistence and four Foundation migrations. It is a Go module
somebody `go get`s and registers by hand in `bootstrap/app.go`. There is no
service provider, container lookup or discovery: wiring is
`New(cfg, db, sessions)` followed by kernel registration. Applications that
publish OpenAPI construct `github.com/hyz-is/arandu-swagger`, pass it through
`NewWithDocumentation`, and register the Swagger module explicitly after this
module.

Read `.agents/skills/` before writing code. Each skill is a procedure, and the
one you need is named by the situation you are in.

## Shared workspace coordination

Substantial work should be decomposed proactively across multiple agents when
there are independent tracks. Give each agent exclusive file ownership where
possible; all agents still share one workspace, so they must preserve visible
changes outside that ownership. The root agent reconciles the combined tree and
runs the final gates.

## The gates

Nothing is finished until all four exit zero.

```sh
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

`GOWORK=off` is not borrowed from somewhere else, and here it is not a
preference either. This checkout may sit beside a Go workspace that lists the
framework repositories and does not list this one; when it does, every command
above fails before it compiles anything:

```
pattern ./...: directory prefix . does not contain modules listed in go.work
or their selected dependencies
```

With the workspace off, the module resolves the framework version in `go.mod` —
which is what CI compiles against, and what somebody's `go get` will get.

Both filters on `gofmt` are load-bearing in the toolchain even where this
repository has nothing for them to skip: `gofmt` is the only tool in the chain
that ignores build tags, and `testdata/` is where a fixture is allowed to be
invalid on purpose.

`aru doctor` is not one of the gates, and running it here costs a minute and
answers nothing:

```
this is not an Arandu project: no go.mod, main.go and arandu.toml together.
Run it from inside a project, or create one with `aru new`
```

It exits 1. It reads applications, and this is a library. The rules it would
apply to a package that is installed — `tenant-from-request`,
`system-grant-without-tenant`, `permission-not-declared` — run in the
application that installs this one, not here.

## What this repository holds

| | measured with |
| --- | --- |
| 36 named routes | `grep -c 'register(stdhttp' routes/web.go` |
| 36 documented operations | `grep -c '^\s*"whatsapp\.' app/Http/Documentation/OperationDefinitions.go` and the generated-contract parity test |
| 27 declared actions: 26 public plus the internal runtime action | `grep -c 'security.Action = "whatsapp' internal/authz/actions.go` |
| 4 ordered migrations, 3 reversible | `grep -Rh 'GetName() string' database/migrations/*.go \| wc -l` |
| PostgreSQL and SQLite runtime support | `New` dialect validation and migration tests |

The root is the package-skeleton interface; the implementation follows the
native Arandu application tree. Subpackages never import the root compatibility
facade:

```text
module.go                         Arandu wiring and lifecycle
contracts.go, errors.go, exports_*.go
                                  stable package-skeleton interface
config/whatsapp.go                typed configuration
app/Enums/                        action vocabulary
app/Models/                       entities, shared contracts and errors
app/Policies/                     default-deny policies
app/Repositories/                 public Grant-first repositories
app/Services/                     authorized domain rules
app/Http/Requests/                typed request contracts
app/Http/Resources/               explicit response fields
app/Http/Controllers/             thin request/response adapters
app/Http/Documentation/           native Arandu Swagger contract (`package apidocs`; `documentation` is reserved by Go tooling)
app/Jobs/                         native queue vocabulary
routes/web.go                     canonical named HTTP surface
database/migrations/              ordered Foundation migrations
internal/                         private WhatsApp and persistence runtime
```

## What does not exist here

Reaching for one of these is the most common way to write a package that is
rejected in review. None of them is missing by accident.

| A model reaches for | What is here instead |
| --- | --- |
| a service provider, a container, a `Register()` that discovers things | `New(cfg, db, sessions)`, called by hand in the installer's `bootstrap/app.go`. Everything the package touches is a parameter |
| a global `DB`, an `init()` that opens a connection | the `*data.DB` handed to `New`. A package that opened its own connection would be a package the application cannot point at a test database |
| a model with `Save()`, `Find()` or a database handle on it | `InstanceRepository`, which takes a `Grant` before it takes an identifier |
| a tenant read from the path, the body, the query or a header | `data.Tenant(g)`, from the Grant, which came from the session |
| a permit-all branch in the policy "for now" | nothing. An action opens only through an explicit `Config.Policy.Roles` entry |
| an `interface{}` config, a map of options, an env var read at call time | the typed `Config` struct, validated by `New` |
| a `panic` on bad wiring | an `error` from `New`. A wiring mistake found at boot costs one restart |
| a webhook cache, channel queue or module-owned delivery goroutine | tenant-scoped SQL snapshots plus Hesape's durable `DatabaseQueue` for webhooks and mention-all; the installer explicitly calls `RegisterJobHandlers` and owns `aru queue:work` |
| a static OpenAPI document or a route table copied into documentation | `routes/web.go` owns method, path and name; `app/Http/Documentation` adds the explicit operation and component contract through Arandu Swagger |

## The four properties

These are the reason the package is shaped the way it is. A change that breaks
one of them is not merged, whatever else it improves. `tests/Unit/policy_test.go`
checks all four against the code.

1. **The policy denies by default.** `TestPolicyIsDefaultDenyForEveryPublicAction`
   walks every public action with an administrator subject and requires
   `security.ErrForbidden` from each when the role map is empty.
2. **The repository takes the `Grant` before the identifier**, and opens with
   `g.Check`. `TestRepositoryRejectsInvalidGrantsBeforeDatabaseAccess` holds it
   there.
3. **The tenant comes from `data.Tenant(g)`**, on every SQL path, and the
   configured runtime scope is checked before policy and SQL.
4. **Nothing reaches the database without passing the first two.** The suite
   passes `data.Wrap(nil, data.DialectSQLite)` — a handle over no database — so
   a statement that ran would panic. Every refusal the suite asserts is
   therefore proof that the refusal came before the query.

`arandu.mod.toml` is the fifth thing to keep true, and it is checked in somebody
else's repository rather than here. It declares `network`, `filesystem`, `exec`
and `migrations`, and `aru doctor` in an application compares the declaration
against what the code *calls* — `os.WriteFile`, `exec.Command`, `http.Get`, a
method named `Migrations` — rather than against what it imports, because
`net/http` is imported by everything with a route and says nothing. Used and not
declared is `permission-not-declared`, an error. Declared and not used is
`permission-not-used`, a warning. Adding an outbound call, a file write or a
process means declaring it in the same commit.

A defect confirmed in Arandu, Hesape, Aru, Kyse or Joaju is not hidden behind a
local workaround. Record it in the Arandu Obsidian vault at
`/Users/paulorlima9/Developer/arandu-io`, following that vault's `GAP-` note and
linking conventions. The note must carry reproducible evidence, impact, the
probable owning component and a corrective proposal; search for an existing gap
before creating a new one.

## Writing code

Every copyright notice must use exactly
`Copyright (c) HYZIS - SERVICOS DIGITAIS LTDA - EPP`, without a year or an
abbreviated company name.

Everything in the source is in English: identifiers, doc comments, internal
comments, error messages, log messages, and the names and messages of tests.
`pkg.go.dev` publishes the doc comments and its readers are users of this
package.

Every exported symbol carries a doc comment, and the comment documents the
symbol and nothing else. Why a signature is what it is belongs there when it is
a fact about the code — *"the value is held because Go does not build a type
from a string"* stays. A date, an issue number, a version in progress or the
name of another repository does not.

Tests go under `tests/`, in a capitalized category directory declaring a
lowercase external package: `tests/Unit` holds `package unit_test`,
`tests/Feature` holds `package feature_test`. Both import the package by its
module path, which is what makes them see exactly what a caller sees. A test
that genuinely needs something unexported goes beside the code as
`*_internal_test.go`, and the suffix is how it says so.
