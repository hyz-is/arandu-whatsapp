# Arandu WhatsApp

WhatsApp API module for Arandu applications. It provides instance management,
pairing, messaging, contacts, groups, media downloads and outbound webhooks on
top of `whatsmeow`.

The module uses Arandu sessions and Grants, typed configuration, host-owned
database handles and Foundation migrations. It does not read environment
variables or open a second application database.

## Requirements

- Go 1.26 or newer.
- Arandu Framework 0.42 or newer.
- PostgreSQL or SQLite with foreign keys enabled.
- `ffmpeg` and `ffprobe` for audio and thumbnail processing.

## Install

```bash
go get github.com/hyz-is/arandu-whatsapp
```

## Wire it

An Arandu application registers a module explicitly. There is no service
provider, no container and no discovery, so the base wiring stays visible in
`bootstrap/app.go`. The optional documentation wiring is shown separately
below.

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
		Webhooks: whatsapp.WebhookConfig{
			SigningSecret: cfg.WhatsApp.WebhookSigningSecret,
		},
		Policy: whatsapp.PolicyConfig{
			Roles: map[security.Action][]string{
				whatsapp.ActionInstanceCreate: {"admin"},
				whatsapp.ActionInstanceList:   {"admin", "operator"},
				whatsapp.ActionInstanceView:   {"admin", "operator"},
				whatsapp.ActionConnectionPair: {"admin", "operator"},
				whatsapp.ActionMessageSend:    {"admin", "operator"},
			},
		},
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

And the registration, inside the `k.Register(...)` call already there:

```go
		whatsappModule,
```

Webhook delivery and mention-all sends use the application's native database
queue. Keep the module in the generated `bootstrap.App` so worker processes
receive the same explicitly wired instance:

```go
type App struct {
	Kernel   *kernel.Kernel
	WhatsApp *whatsapp.Module
	// ...the generated fields remain here.
}
```

Include it in `Build`'s return value:

```go
return App{
	Kernel: k,
	WhatsApp: whatsappModule,
	// ...the generated fields remain here.
}, nil
```

Then pass `app.WhatsApp` from the `"work"` branch in
`bootstrap/console.go`:

```go
return work(ctx, k, app.Queue, app.WhatsApp, args)
```

Finally, adapt the generated functions in `bootstrap/background.go`:

```go
type jobHandlerRegistrar interface {
	RegisterJobHandlers(*queue.Worker) error
}

func work(ctx context.Context, k *kernel.Kernel, store queue.Queue,
	registrar jobHandlerRegistrar, args []string,
) error {
	flags := flag.NewFlagSet("work", flag.ContinueOnError)
	name := flags.String("queue", jobs.DefaultQueue, "which queue to drain")
	workers := flags.Int("workers", 4, "how many jobs to run at once")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := k.Boot(ctx); err != nil {
		return err
	}
	defer func() { _ = k.Shutdown() }()

	w := queue.NewWorker(store, queue.WorkerOptions{
		Queue:       *name,
		Concurrency: *workers,
		Recorder:    k.Recorder(),
	})
	if err := registerHandlers(w, registrar); err != nil {
		return err
	}
	if len(w.Names()) == 0 {
		return fmt.Errorf("no job handlers are registered")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	_, err := w.Daemon(ctx)
	return err
}

func registerHandlers(w *queue.Worker, registrar jobHandlerRegistrar) error {
	if err := registrar.RegisterJobHandlers(w); err != nil {
		return err
	}
	// arandu:begin custom
	// Register other application handlers here.
	// arandu:end custom
	return nil
}
```

This keeps handler registration explicit without a global or discovery hook.
The generated queue module supplies the native jobs-table migrations. Add the
WhatsApp queue to the queues it monitors, preserving every queue the application
already names:

```go
jobs.NewModule(
	queueStore,
	jobs.DefaultQueue,
	whatsapp.WebhookQueueName,
	whatsapp.MessageQueueName,
)
```

Without those names workers still drain jobs, but native health and diagnostic
checks watch only the default queue and cannot report WhatsApp backlog.

Then, once, before the application serves:

```bash
aru migrate
```

Run each dedicated queue in a worker process:

```bash
aru queue:work --queue=whatsapp-webhooks --workers=N
aru queue:work --queue=whatsapp-messages --workers=N
```

The policy is default-deny. Only actions present in `Policy.Roles` are opened;
unknown and newly added actions stay closed.

`Config.Tenant` is the complete runtime scope of one module registration. The
module refuses sessions from another tenant before authorization or SQL so that
WhatsApp callbacks, reconnection and durable jobs cannot cross tenant
boundaries. Register the package in a tenant-scoped Arandu application.

This package declares four ordered Foundation migrations: three reversible
package schemas and the upstream WhatsMeow store upgrade. They are exposed
through `foundation.Migratable`, which is why the migration step is not optional
and why `arandu.mod.toml` declares it. `New`, `Boot` and `Start` never apply
migrations.

## Configuration

| field | required | meaning |
| --- | --- | --- |
| `Tenant` | yes | tenant owned by this module registration and guest fallback. |
| `Prefix` | no | where the routes are mounted. Defaults to `/whatsapp`. |
| `WhatsApp` | no | pairing, device identity, reconnect and address-cache settings. |
| `Persistence` | no | enables persistence of messages, updates and contacts. |
| `Webhooks` | no | signed global/instance delivery, retention and native database queue. |
| `Processing` | no | mention-all operation timeouts and recoverable snapshot retention; worker concurrency belongs to `aru queue:work`. |
| `Media` | no | limits, temporary directory and ffmpeg/ffprobe paths. |
| `Policy.Roles` | no | roles allowed for each action; empty denies every action. |

`New` returns an error rather than starting half-wired, so a setting that
cannot work fails where it is written instead of on the first request that
needed it.

`Webhooks.SigningSecret` must contain at least 32 bytes before a global or
instance webhook can be enabled. Every attempt carries `X-Arandu-Timestamp`
and `X-Arandu-Signature`; the signature is HMAC-SHA256 over
`timestamp.deliveryId.body`. Delivery snapshots default to 30 days of
retention and successful snapshots are compacted immediately.

`Processing.Retention` also defaults to 30 days. A second native job removes
an abandoned mention-all snapshot together with its processing job, and an
expired processing job is discarded before any WhatsApp access.

## HTTP API

The default base path is `/whatsapp/instances`. The module registers 36 named
routes for instances, pairing, connection state, webhooks, messages, contacts,
media, chats, calls and groups. Requests authenticate exclusively through the
Arandu session store.

When the host installs and enables Arandu Swagger, the exact paths, request
bodies, response envelopes and status codes are published from the same named
routes that serve requests. Message, Passkey and webhook behavior is documented
under [docs](docs).

JSON responses use Arandu resources and are wrapped in `data`. Tenant IDs and
the WhatsMeow device-store key are never exposed. Authorization failures use a
generic public message; internal policy details stay on the server.

## Layout

```text
module.go                         Arandu wiring and lifecycle
contracts.go, errors.go, exports_*.go
                                  stable root package interface
config/whatsapp.go                typed configuration
app/Enums/                        action vocabulary
app/Models/                       entities and shared contracts
app/Policies/                     default-deny authorization
app/Repositories/                 Grant-first persistence facade
app/Services/                     authorized domain rules
app/Http/Requests/                typed request contracts
app/Http/Resources/               explicit response fields
app/Http/Controllers/             thin HTTP adapters
app/Http/Documentation/           native Arandu Swagger contract
app/Jobs/                         durable queue vocabulary
routes/web.go                     canonical named HTTP surface
database/migrations/              ordered Foundation migrations
internal/                         private persistence and WhatsApp runtime
```

The root aliases preserve the package-skeleton installation contract
(`whatsapp.New`, `whatsapp.Config`, `whatsapp.Service`). Implementation packages
follow the same responsibility tree as an Arandu application, and none imports
the root facade.

## What is already correct, and has to stay that way

**The policy denies everything by default.** An application opens actions with
explicit entries in `Config.Policy.Roles`.

**The repository takes `(ctx, g, id)`, in that order.** The Grant comes before
the identifier so that naming a record without a decision about it is not
expressible. Every method opens with `g.Check`, which proves the Grant was
issued for that exact action rather than for another one.

**The tenant comes from `data.Tenant(g)`.** Never from the path, body, query or
header. The service also verifies that the session tenant matches the configured
runtime scope before authorization or SQL.

**`arandu.mod.toml` declares what the package does** — network, filesystem,
exec, migrations — and `aru doctor` compares the declaration against the
calls found by its capability analysis. A package that performs an outbound
request while declaring `network = false` is rejected.

## Lifecycle and ownership

- `New` validates and assembles collaborators without I/O or goroutines.
- `Boot` configures the process-wide WhatsMeow device descriptor.
- `Start` verifies schema state and restores configured-tenant connections
  asynchronously.
- `Health` checks the borrowed database and connection-restore result.
- `Close` stops module-owned work and disconnects managed clients. The host
  application owns all native queue workers.

The host retains ownership of the database, SQL pool and session store.

## Documentation

Documentation is optional and owned by the host application. Build one
`arandu-swagger` module in `bootstrap/app.go`, using an application setting
loaded from configuration or dotenv for `Enabled`, and register it last:

```go
docs, err := swagger.New(swagger.Config{
	Enabled:  documentationEnabled,
	Title:    "WhatsApp API",
	Version:  "1.0.0",
	UIPath:   "/docs",
	SpecPath: "/docs/openapi.json",
	Filter: swagger.RouteFilter{
		IncludeModules: []string{"whatsapp"},
	},
	UIMiddleware:   []swagger.Middleware{requireDocumentationAccess},
	SpecMiddleware: []swagger.Middleware{requireDocumentationAccess},
})
if err != nil {
	return App{}, err
}

whatsappModule, err := whatsapp.NewWithDocumentation(
	whatsappConfig,
	db,
	sessions,
	docs,
)
if err != nil {
	return App{}, err
}

k.Register(whatsappModule, docs)
```

`Enabled: false` registers neither the UI nor the specification route. When
enabled, access to the configured `UIPath` and `SpecPath` is controlled by the
application middleware supplied through `UIMiddleware` and `SpecMiddleware`.
The package never reads environment variables itself. Applications that do not
publish OpenAPI continue to use `whatsapp.New` unchanged.

- [Message sending](docs/send-messages.md)
- [Passkey pairing](docs/passkey-pairing.md)
- [Webhooks](docs/webhooks.md)
- [Migrations](docs/migrations.md)
- Runtime OpenAPI contract: configured `SpecPath` (`/docs/openapi.json` by
  default)

## Tests

```bash
go test -race ./...
```

The suite includes policy-before-database checks, tenant isolation, migrations
through Hesape's migrator, domain-schema rollback and lifecycle ownership.

## Licence

MIT. See [LICENSE.md](LICENSE.md).

Copyright (c) HYZIS - SERVICOS DIGITAIS LTDA - EPP
