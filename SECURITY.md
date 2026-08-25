# Security policy

## Supported versions

The latest minor release of :package_name receives security fixes. Older minor
releases do not: a published Go module version is immutable, so a fix is a new
release rather than a moved tag.

## Reporting a vulnerability

Report it privately, through GitHub's advisory form:

<https://github.com/:author_username/:module_slug/security/advisories/new>

Do not open a public issue and do not describe the problem in a pull request.
A report that arrives in public is a report every reader of this repository can
act on before there is a release to upgrade to.

Include what you need to reproduce it: the version, the wiring, and the request
or call that triggers it.

Expect an acknowledgement within a few days. If the report is confirmed, the
fix, the release and the advisory are published together, and you are credited
unless you ask not to be.

## What is in scope

Anything that lets a caller reach data a policy did not authorize. In
particular:

- a path from a handler to the repository that does not pass through a policy;
- a statement that is not scoped by `data.Tenant(g)`, on any path, read or
  write;
- a `Grant` that can be produced without a policy returning nil;
- a field reaching a response that `Resource` does not list.

## What is not

- A vulnerability in the framework itself. Report that to the framework.
- A policy that is too permissive in an application that opened it. What this
  package ships denies everything; what an application opens is its own.
