<!-- configure:template-start -->
# Before this is a package

This repository is the Arandu package template, and it has not been configured
yet. Everything below the next heading is written as if it had been: the values
in it are placeholders that happen to compile, and one command turns them into
somebody's own.

If the task is to *start* a package, that command is the whole of it:

```sh
export GOWORK=off
go run ./configure.go
```

It asks five questions, rewrites every file, renames the files and directories
whose names carried a template value, formats the Go it touched, removes the
sections marked as belonging to the template — this one included — and deletes
itself.

Three things about the template state are worth knowing before you touch it.

**`configure.go` is not part of the package.** It opens with `//go:build ignore`
and declares `package main`, while every other file at the root declares
`package skeleton`. That is what lets `go build ./...`, `go vet ./...` and
`go test -race ./...` all pass before it has ever run — they never see it. It is
also what makes it invisible to the gates, so it is checked by name, and CI has
a step that does exactly that:

```sh
go vet configure.go
```

**The Go sources and `go.mod` carry real values, not `:placeholder` spellings.**
A module path with a colon in it is not a module path and a package clause with
one is not Go. So `skeleton`, `Skeleton` and
`github.com/arandu-io/package-skeleton` are what `configure.go` replaces there,
and the `:module_path` spelling appears only in prose — `README.md`,
`CONTRIBUTING.md`, `SECURITY.md`, `LICENSE.md`, `CHANGELOG.md`,
`arandu.mod.toml`, and the skill under `.agents/skills/skeleton-package/`.

**Running it twice is refused rather than done.** Once the placeholders are gone
there is nothing left to replace, and a second pass would rewrite whatever now
happens to match. The refusal changes nothing, and CI asserts that by hashing
the tree on both sides of the refused run.

That last point is why anything you add to this template has to survive the
substitution, in both directions. The words `skeleton` and `Skeleton` are
replaced wherever they appear, contents and names alike — so prose that uses
either as an English word comes out of `configure.go` saying `widget`. Write
around them.

<!-- configure:template-end -->
# Working on :package_name

This is an Arandu package: one entity, one policy that decides about it, one
repository that is the only door to its table, and the routes that reach them.
It is a Go module somebody `go get`s and registers by hand in their own
`bootstrap/app.go`, which is the whole difference from working in an
application. There is no service provider, no container and no discovery — if a
line of wiring is not written in the installer's repository, it does not happen.

Read `.agents/skills/` before writing code. Each skill is a procedure, and the
one you need is named by the situation you are in.

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
| 6 Go files, one per role, all in one package at the root | `grep -l '^package skeleton' *.go` |
| 2 test files, 15 tests | `find tests -name '*_test.go'` · `go test -count=1 ./... -v \| grep -c '^--- PASS'` |
| 3 routes | `grep -c 'r.Action' module.go` |
| 5 actions the policy answers about | `grep -cE '^\t[A-Za-z]+ security.Action = ' policy.go` |
| 2 direct dependencies, both under `arandu-io` | `go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}} {{.Version}}{{end}}' all` |

The layout is by role rather than by layer, so the package reads top to bottom:

```
module.go      registration, routes, handlers and migrations
config.go      what the application passes in
model.go       the entity, and what it may answer with
policy.go      who may do what
repository.go  data access, which requires a Grant
service.go     the rules, and the only path a handler may take
```

## What does not exist here

Reaching for one of these is the most common way to write a package that is
rejected in review. None of them is missing by accident.

| A model reaches for | What is here instead |
| --- | --- |
| a service provider, a container, a `Register()` that discovers things | `New(cfg, db, sessions)`, called by hand in the installer's `bootstrap/app.go`. Everything the package touches is a parameter |
| a global `DB`, an `init()` that opens a connection | the `*data.DB` handed to `New`. A package that opened its own connection would be a package the application cannot point at a test database |
| a model with `Save()`, `Find()` or a database handle on it | `SkeletonRepository`, which takes a `Grant` before it takes an identifier |
| a tenant read from the path, the body, the query or a header | `data.Tenant(g)`, from the Grant, which came from the session |
| a permit-all branch in the policy "for now" | nothing. The policy denies, and an action is opened by writing the rule that opens it |
| an `interface{}` config, a map of options, an env var read at call time | the typed `Config` struct, validated by `New` |
| a `panic` on bad wiring | an `error` from `New`. A wiring mistake found at boot costs one restart |
| a third dependency | an argument, first. This module is imported into other people's builds |

## The four properties

These are the reason the package is shaped the way it is. A change that breaks
one of them is not merged, whatever else it improves. `tests/Unit/policy_test.go`
checks all four against the code.

1. **The policy denies by default**, and has no branch that allows an action.
   `TestThePolicyDeniesEveryActionByDefault` at `tests/Unit/policy_test.go:46`
   walks every action with an administrator subject and requires
   `security.ErrForbidden` from each.
2. **The repository takes the `Grant` before the identifier**, and opens with
   `g.Check`. `TestTheRepositoryRefusesAGrantThatWasNeverIssued:111` and
   `TestTheRepositoryRefusesAGrantIssuedForAnotherAction:135` hold it there.
3. **The tenant comes from `data.Tenant(g)`**, on every path, read and write.
   `TestTheTenantComesFromTheGrant:162`.
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

## Writing code

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
