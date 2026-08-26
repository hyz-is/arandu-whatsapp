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
| `whatsapp-policy` | opening an action, adding a repository method, or anything answering 403 |
| `whatsapp-module` | adding a route, a handler, a config field, a response field or a migration |
| `whatsapp-release` | the gates, the manifest, a dependency, a version, a tag |
| `whatsapp-package` | installing and wiring this package **into an application** |

The last one has a different audience from the other three: it travels with the
package so an assistant installing it has the explicit wiring, `aru migrate`,
typed role policy and lifecycle requirements in front of it.

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
