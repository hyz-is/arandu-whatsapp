# Skills

Procedures an assistant follows when working on this package.

They live in `.agents/skills/<name>/SKILL.md`, which is the path the coding
assistants read from — Cursor, Codex, Cline, Copilot, Gemini CLI, Amp, OpenCode,
Warp, Zed and the rest all look there. It is one directory rather than a file
per vendor, so a skill written once is read by whatever this package is being
written with.

Each file opens with frontmatter carrying a `name` and a `description`. The
`name` has to equal the directory name exactly, or the skill is not loaded. The
description is what a tool reads to decide whether the skill is relevant, so it
names the situation you are in rather than the topic it covers.

| skill | when it fires |
| --- | --- |
| `skeleton-policy` | opening an action, adding a repository method, or anything answering 403 |
| `skeleton-module` | adding a route, a handler, a config field, a response field or a migration |
| `skeleton-release` | the gates, the manifest, a dependency, a version, a tag |
| `skeleton-package` | installing and wiring this package **into an application** |

The last one has a different audience from the other three, and that is on
purpose: it travels with the package so that an assistant working in somebody
else's project — the one running `go get` — has the wiring, the migration step
and the closed policy in front of it instead of guessing.

<!-- configure:template-start -->
`skeleton-package` is also the one written as a template. It carries the
`:module_path`, `:module_slug`, `:package_name`, `:author_name` and
`:author_username` placeholders and the `Skeleton` identifier, and `configure.go`
rewrites all six — in the contents *and* in the directory name, which is what
keeps the frontmatter `name` and the directory in agreement.

The other three are rewritten too, because their names carry the slug: the
package that comes out of the template has `<slug>-policy`, `<slug>-module`,
`<slug>-release` and `<slug>-package`. The prefix is the slug rather than the
repository name for a mechanical reason — the slug is a value `configure.go`
knows, and the repository name is not.

<!-- configure:template-end -->
## Why these exist

The audience of the first three is somebody changing the package. A model asked
to work here fills the gap with the frameworks it does know, and produces a
service provider, a model with a `Save()` on it, a container lookup, a tenant
read out of the URL and a policy with a branch that returns nil for
administrators "for now". None of those is how this works, and the last one is
not a style disagreement — it is the hole every application installing the
package would inherit.

The rest of the answer is that the package is built to be checked rather than
trusted. The suite runs against a database handle that wraps nothing, so a
statement that ran would panic; every refusal it asserts is therefore proof that
the refusal happened before the query. An assistant that runs
`go test -race ./...` is not guessing.

## Adding your own

A skill in this directory is yours and travels with the repository. Keep it a
procedure rather than a description: a file that says "read the documentation"
never changes what anybody does.
