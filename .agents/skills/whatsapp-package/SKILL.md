---
name: whatsapp-package
description: Install, wire and use the WhatsApp package (Go, Arandu) in an application. Use when the request is to install the module, register it, configure its 36 routes, open explicit role actions, diagnose 403 responses or apply its schema. Covers New(cfg, db, sessions), lifecycle ownership, typed configuration, Arandu sessions, default-deny Policy.Roles and aru migrate.
license: MIT
---

# Using WhatsApp

An Arandu WhatsApp module with 36 routes, tenant-scoped domain tables, a
Foundation migration that delegates the WhatsMeow store schema to its upstream
container, and explicit lifecycle interfaces. It is registered by hand; there
is no service provider, container lookup or discovery.

## Install

```bash
go get github.com/hyz-is/arandu-whatsapp
```

## The three lines of wiring, and where each one goes

All three are in `bootstrap/app.go`. Nothing else in the application changes,
and `aru make:module` does not edit this file for you.

The import, with the other module imports:

```go
import (
	"github.com/arandu-io/framework/security"
	whatsapp "github.com/hyz-is/arandu-whatsapp"
)
```

The construction, in `Build`, after the session store exists and before
`k.Register`:

```go
	whatsappModule, err := whatsapp.New(whatsapp.Config{
		Tenant: cfg.Auth.Tenant,
		Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{
			whatsapp.ActionInstanceList: {"admin", "operator"},
			whatsapp.ActionInstanceView: {"admin", "operator"},
			whatsapp.ActionMessageSend:  {"admin", "operator"},
		}},
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

And the registration, inside the `k.Register(...)` call already there:

```go
		whatsappModule,
```

`New` returns an error rather than starting half-wired. It refuses a
configuration that cannot work, a nil database handle and a nil session store,
so a wiring mistake costs one restart instead of arriving as a nil dereference
on the first request that needed it.

## Then run the migrations

```bash
aru migrate
```

Not optional, and not automatic. The package owns its domain tables and
delegates the WhatsMeow store upgrade, which is why its
`arandu.mod.toml` declares `migrations = true` — that declaration is how an
installer finds out before deploying rather than from the first request.

`aru migrate` is a step in the pipeline and never a call at the start of the
process: with N replicas, N migrations race each other.

**Symptom to recognise:** `no such table`, or `relation does not exist`, from a
route that authorized correctly. The migration has not run.

## Configuration

| field | required | meaning |
| --- | --- | --- |
| `Tenant` | yes | complete tenant scope of this registration and guest fallback |
| `Prefix` | no | where the routes are mounted. Defaults to `/whatsapp` |
| `WhatsApp` | no | pairing, reconnect, device and address settings |
| `Persistence` | no | message, update and contact persistence switches |
| `Webhooks` | no | global destination; delivery uses the host's native database queue |
| `Processing` | no | asynchronous mention-all queue and timeouts |
| `Media` | no | input limits, temporary directory and ffmpeg paths |
| `Policy.Roles` | no | roles allowed for each action; empty denies everything |

The tenant never comes from a request field. Public SQL reads it from the Grant,
and the service refuses any authenticated subject whose tenant differs from
`Config.Tenant`. Register one module in a tenant-scoped application.

## Routes

All 36 named routes live below `/whatsapp/instances` by default. They cover
instances, connections and Passkey pairing, webhooks, messaging, contacts,
media, chats, calls and groups. Use `aru route:list` or `docs/openapi.yaml` for
the exact method, path and route name. A custom `Prefix` moves every path while
route names remain stable.

Authentication comes from the host Arandu session (`arandu_session`) and every
operation requires the Grant issued by the policy. JSON answers are Arandu
resources under `data`; media downloads may be binary or multipart.

`TenantID` is not in any response. That is deliberate: it names a customer's
identifier and the response is a declared list of fields rather than the entity.

## Every route refuses everybody until its action is configured

This is the state the package ships in, and it is not a bug to work around.
An empty `Config.Policy.Roles` denies all 26 public actions:

```
403 forbidden
```

is what a correctly installed, correctly migrated, correctly wired package
answers to an administrator on day one.

Open only the actions the application needs in its module configuration:

```go
	Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{
		whatsapp.ActionInstanceList: {"admin"},
		whatsapp.ActionInstanceView: {"admin"},
		whatsapp.ActionMessageSend:  {"admin", "operator"},
	}},
```

What is absent stays closed, including actions added by a future version. Never
populate the map by iterating `whatsapp.Actions`, because that would
automatically open future capabilities.

## What the answers mean

| status | what happened |
| --- | --- |
| `403` | the policy refused, or no rule allows the action yet |
| `400` | malformed input or an invalid operation |
| `404` | instance, message, media or pairing state not found |
| `409` | connection, pairing or dependency conflict |
| `422` | semantically invalid input or Passkey assertion |
| `503` | WhatsApp client or upstream service unavailable |

A refusal carries no detail beyond the status and a word. Telling a client why a
policy said no tells it what exists and what does not, one request at a time.
The reason is in the log.

Anything else is a 500 and the framework's error page in development. The
package returns unexpected errors rather than swallowing them, so a route that
answers 200 with an empty body is not something it does.

## Reporting a problem

Issues and pull requests go to the repository the module path names,
`github.com/hyz-is/arandu-whatsapp`, which belongs to `hyz-is`. A vulnerability goes to the
private advisory form named in that repository's `SECURITY.md`, and never into
an issue.

MIT licensed. Copyright (c) HYZIS - SERVICOS DIGITAIS LTDA - EPP.
