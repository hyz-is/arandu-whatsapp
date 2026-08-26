---
name: whatsapp-policy
description: Change or configure authorization in the Arandu WhatsApp module. Use for role mappings, 403 responses, new actions, Grant-first repository methods, tenant isolation, service authorization or access to Swagger UI/spec routes. Covers Config.Policy.Roles, default-deny behavior, Grant.Check, record checks, the fixed runtime tenant boundary and the host-owned documentation middleware boundary.
license: MIT
---

# Opening the policy

The package ships denying every action. Applications open only named actions by
mapping them to roles in `Config.Policy.Roles`. There is no permit-all fallback,
and actions absent from the map remain closed.

## How a request actually gets through

Four things happen, in this order, and each one is a different file.

1. `app/Services/WhatsAppService.go` verifies that the subject belongs to `Config.Tenant`.
2. It validates input and calls `security.Authorize` for the operation.
3. `InstancePolicy.Can` checks record tenant and configured roles.
4. Only then does a `security.Grant` exist, and repositories will not run a
   statement without one.

The Grant is the mechanism, and it is worth knowing exactly how much it
guarantees. `Authorize` is the only function in the framework that returns a
valid one; `Grant`'s fields are unexported, so a handler cannot assemble one.
`security.SystemGrant` is reserved for module-owned callbacks, reconnection and
queues. It always uses `ActionRuntime` and the validated configured tenant. It
must never appear in an HTTP handler or accept a tenant from a request.

## The procedure

**1. Configure the smallest role map in the host application.**

```go
	Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{
		whatsapp.ActionInstanceList: {"admin"},
		whatsapp.ActionInstanceView: {"admin"},
		whatsapp.ActionMessageSend:  {"admin", "operator"},
	}},
```

Do not build this map by iterating `whatsapp.Actions`; that would silently open
new capabilities after an upgrade.

Swagger UI and specification access are a separate host-application surface.
Do not add a WhatsApp action, policy branch or Grant for `/docs`: protect the
configured Swagger `UIPath` and `SpecPath` with the application's
`UIMiddleware` and `SpecMiddleware`. WhatsApp policies and Grants continue to
authorize the 36 API operations described by that document.

**2. Guests remain denied unless a configured role can actually match.** `security.Guest(tenant)` is the
subject a visitor with no readable session gets, and it is the only subject that
arrives with an empty `ID` and is still authorized at all. The zero `Subject` —
an empty `ID` with no guest marker — is refused by `Authorize` before `Can` is
consulted, because it is almost always a session that failed to load, and a
policy asked about nobody answers about nobody.
The route suite proves a visitor with no session is refused before SQL.

**3. Leave tenant checks fail-closed.**

```go
	if instance.TenantID != "" && instance.TenantID != subject.Tenant {
		return fmt.Errorf("whatsapp instance belongs to another tenant")
	}
```

It runs before role matching. Separately, `Service.checkTenant` refuses a
session outside the registration's complete runtime scope before policy and SQL.
Public 403 responses must not expose either internal reason.

**4. Add a new public action to `Actions`.**

`TestPolicyIsDefaultDenyForEveryPublicAction` walks that list. Add route tests
for the exact action and keep `ActionRuntime` out of the public list.

**5. Run the gates.**

```sh
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go') \
  && go build ./... && go vet ./... && go test -race ./...
```

Add tests for an allowed configured role, a refused role, a wrong action, a zero
Grant and a cross-tenant record.

## Adding a repository method

Every method opens with `g.Check(THE_ACTION)`, and it is not a formality. The
Grant records which action it was issued for, so `Check` fails on three
different mistakes with three different messages: the zero Grant (`missing grant
for whatsapp.view (call auth.Authorize first)`), a `SystemGrant` that was
refused, and a Grant issued for another action (`grant issued for
whatsapp.delete, used on whatsapp.view`). The third is the copy-paste between
two repository methods, and `TestRepositoryRejectsInvalidGrantsBeforeDatabaseAccess`
catches it before SQL.

The signature is `(ctx, g, id)` and the order is the mechanism. A caller cannot
name a record without first holding a decision about it, and cannot leave the
decision out, because the call does not compile without it.

`data.Repository[T, ID]` requires all five methods — `Find`, `List`, `Create`,
`Update`, `Delete` — so the compile-time assertion in `app/Repositories` fails if
one is dropped, even one the package has no route for. Keep the method and its
`g.Check`; a repository with a hole in it is not a smaller repository.

## Reads are authorized like writes

There is no listing that skips the policy, and no read model, projection,
report or export that does. A read path with no check is a leak between
customers with a technical name.

Record reads authorize collection access, load through a tenant-scoped Grant,
then authorize the loaded record. The second authorization is the one people
delete:

```go
	g, err := security.Authorize(ctx, s.policy, actor, ActionInstanceView, Instance{TenantID: actor.Tenant})
	record, err := s.repository.Find(ctx, g, id)
	_, err = security.Authorize(ctx, s.policy, actor, ActionInstanceView, record)
```

The first call sees an empty value, so every rule about the record itself —
who owns the row, whether it is published — never ran. Without the second call a
policy can be written that looks correct, reads correctly, and is never
consulted about the thing it protects. The read is already scoped by
`data.Tenant(g)`, so the second call is not what keeps customers apart; it is
what keeps the policy honest.

`List` authorizes once, on the empty candidate, and does not authorize per row.
A policy call per record would be one call per row of a page and would still not
narrow the query — a listing that has to read a customer's rows in order to
decide it may not read them has already read them. **A rule that hides
individual records from a listing belongs in the statement, as a predicate**,
beside the tenant filter. The action decides whether the listing runs at all.

## Three shapes that look like authorization and are not

- **A tenant taken from the request.** The path, the body, the query string, a
  header — all of them are values the caller chose. `data.Tenant(g)` is the only
  source, and in an application `aru doctor` reports the others as
  `tenant-from-request` and `tenant-from-header`. The one place a tenant does
  not come from a Grant is validated `Config.Tenant`, used for the guest and the
  module-owned runtime scope.
- **A check in the controller.** `app/Http/Controllers` reads the input, asks the
  service and answers. A controller that reached the repository would be a controller
  that skipped the policy, and nothing in the type system would object — the
  Grant is what objects, and only if it is absent.
- **A refusal that explains itself to the client.** `answer` in
  `app/Http/Controllers/Controller.go` sends a generic 403. Telling the client why a policy said no is
  telling them what exists and what does not, one request at a time. The reason
  is in the log, where the person operating the system reads it and the person
  probing it does not.
