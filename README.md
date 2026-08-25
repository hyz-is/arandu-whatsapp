<!-- configure:template-start -->
# Arandu package skeleton

A GitHub template for writing an Arandu package. Clone it, run one command, and
you have a module that compiles, tests, and is already wired the way the
framework requires.

## Use it

Press **Use this template** on GitHub, clone what it makes, and run:

```bash
go run ./configure.go
```

It asks five questions, shows what it will replace, rewrites every file, renames
the files and directories whose names carried a template value, formats the Go
it touched, removes this section, and deletes itself. Then:

```bash
go build ./... && go test ./...
```

A pipeline answers the same questions with flags:

```bash
go run ./configure.go --non-interactive \
  --module-path github.com/acme/arandu-widget \
  --module-slug widget \
  --package-name Widget \
  --author-name "Acme" \
  --author-username acme
```

Running it twice is refused rather than done: once the placeholders are gone
there is nothing left to replace, and a second pass would rewrite whatever now
happens to match.

## What it replaces

| placeholder | becomes | where it appears |
| --- | --- | --- |
| `:module_path` | `github.com/acme/arandu-widget` | `go.mod`, the import in every test, this file, `CONTRIBUTING.md` |
| `:module_slug` | `widget` | the package clause, `Name()`, every action name, the table name, `arandu.mod.toml`, this file |
| `:package_name` | `Widget` | this file, `CHANGELOG.md`, `SECURITY.md` |
| `:author_name` | `Acme` | `LICENSE.md`, this file |
| `:author_username` | `acme` | `arandu.mod.toml`, `SECURITY.md`, this file |
| `Skeleton` | `Widget` | the entity, the policy, the repository and the service |

The replacement runs over the contents of every file **and over the names of
files and directories**. One name in this tree is read as data:
`.agents/skills/skeleton-package/SKILL.md` declares `name: skeleton-package` in
its own frontmatter, and a tool that reads the two and finds them different
skips the skill. Renaming the contents alone would ship a package carrying a
skill nothing loads, and no gate would say so.

The Go sources and `go.mod` carry the values rather than the `:placeholder`
spelling, because a module path with a colon in it is not a module path and a
package clause with one is not Go. That is what lets the four gates below pass
before this command has ever run: a template that only compiles after being
configured is a template whose breakage is discovered by its first user.

## The four gates

```bash
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

CI runs them twice: once on the template, and once on a clean clone that has
been configured. A skeleton that passes before configuring and breaks after is
worse than none.

## One thing that is only true before you configure

`arandu.mod.toml` declares `filesystem = false`, and `configure.go` reads and
writes files. That is not a contradiction in the package: the manifest
describes what is published, and `configure.go` is deleted before anything is
published. It is worth knowing because a capability audit run on the template
itself will report it, and the fix is to configure the template rather than to
widen the declaration.

<!-- configure:template-end -->
# :package_name

An Arandu package. It registers its own routes, owns its own table, and decides
for itself who may reach either.

## Install

```bash
go get :module_path
```

## Wire it

An Arandu application registers a module explicitly. There is no service
provider, no container and no discovery, so these are the lines to paste into
`bootstrap/app.go` and there are no others.

The import, with the other module imports:

```go
import (
	:module_slug ":module_path"
)
```

The construction, in `Build`, after the session store exists and before
`k.Register`:

```go
	:module_slugModule, err := :module_slug.New(:module_slug.Config{
		Tenant: cfg.Auth.Tenant,
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

And the registration, inside the `k.Register(...)` call already there:

```go
		:module_slugModule,
```

Then, once, before the application serves:

```bash
aru migrate
```

This package owns a table, which is why the migration step is not optional and
why `arandu.mod.toml` says `migrations = true`.

## Configuration

| field | required | meaning |
| --- | --- | --- |
| `Tenant` | yes | the customer a visitor with no session is read as. From the application's configuration, never from the request. |
| `Prefix` | no | where the routes are mounted. Defaults to `/:module_slug`. |
| `PageSize` | no | how many records one page answers with. Defaults to 25, refused above 200. |

`New` returns an error rather than starting half-wired, so a setting that
cannot work fails where it is written instead of on the first request that
needed it.

## Routes

| method | path | name |
| --- | --- | --- |
| `GET` | `/:module_slug` | `:module_slug.index` |
| `GET` | `/:module_slug/{id}` | `:module_slug.show` |
| `POST` | `/:module_slug` | `:module_slug.store` |

Every one of them is refused until the policy is opened. That is the state the
package ships in, and it is deliberate.

## Open the policy

`policy.go` denies every action and has no branch that allows one. Open what
this package needs, one action at a time, inside the custom block:

```go
	// arandu:begin custom
	if a == SkeletonView && (s.ID == record.ID || s.HasRole("admin")) {
		return nil
	}
	// arandu:end custom
```

What is not written there stays closed, including every action added later.

## Layout

```
module.go      registration, routes, handlers and migrations
config.go      what the application passes in
model.go       the entity, and what it may answer with
policy.go      who may do what
repository.go  data access, which requires a Grant
service.go     the rules, and the only path a handler may take
```

## What is already correct, and has to stay that way

**The policy denies everything.** There is no permit-all branch to delete
later. A package that could reach the database without passing a policy would
not be insecure; it would not compile, because the repository takes a `Grant`
and a `Grant` is only produced by a policy that returned nil.

**The repository takes `(ctx, g, id)`, in that order.** The Grant comes before
the identifier so that naming a record without a decision about it is not
expressible. Every method opens with `g.Check`, which proves the Grant was
issued for that exact action rather than for another one.

**The tenant comes from `data.Tenant(g)`.** Never from the path, the body, the
query string or a header. The value on the Grant came from the session; a value
that arrived with the request is a value the caller chose.

**`arandu.mod.toml` declares what the package does** — network, filesystem,
exec, migrations — and `aru doctor` compares the declaration against the
imports. A package that says it makes no outbound calls and then holds an HTTP
client is rejected.

## Tests

```bash
go test -race ./...
```

The suite passes a database handle that wraps nothing, so any statement that
ran would panic. Every refusal it asserts is therefore proof that the refusal
happened before the query, and not after one.

## Licence

MIT. See [LICENSE.md](LICENSE.md). Copyright :author_name.
