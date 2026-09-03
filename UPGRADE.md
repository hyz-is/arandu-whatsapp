# Upgrade guide

What changed in a way that stops your code compiling, and what to write
instead.

Additions are not listed. A new symbol breaks nothing, and a file that named
every one of them would be a changelog nobody reads to find the two lines that
matter.

## Before v1.0.0

While the version starts with `v0.`, the API can break. That is what `v0.`
means in Go, and it is deliberate: the alternative is freezing a shape before
anybody has installed it. What is not deliberate is breaking it quietly, which
is what this file exists to stop.

Every release from here is compared against the one before it by `apidiff` in
CI. An incompatible change with no entry here fails the build, and the entry
has to name the symbol.

---

## Unreleased — the Framework floor is 0.42

Nothing in this package's own API moved. `apidiff` against `v0.1.0` reports no
incompatible change, so no call site here needs rewriting.

What moved is the minimum this package compiles against:

| | was | is |
|---|---|---|
| `github.com/arandu-io/framework` | `v0.35.0` | `v0.42.1` |
| `github.com/arandu-io/hesape` | `v0.12.0` | `v0.21.1` |

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
