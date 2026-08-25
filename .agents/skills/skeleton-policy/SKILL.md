---
name: skeleton-policy
description: Decide who may do what in this Arandu package, and open an action that is currently closed. Use when the request is to "open the policy", "let admins read this", "allow the owner to edit", "add an action", "add a permission", "everything returns 403", "the tests say ErrForbidden", "add a method to the repository", "why does Find authorize twice", "filter the listing by owner", or when a change touches policy.go, repository.go or service.go. Covers the Grant, the custom block that survives regeneration, what g.Check catches, why the tenant is never read from the request, and the three shapes that look like authorization and are not.
license: MIT
---

# Opening the policy

The package ships denying every action, and that is the state to start from
rather than a placeholder to delete. There is no permit-all branch to remove,
and adding one is the single change that would make every application installing
this package unsafe while looking like working code.

## How a request actually gets through

Four things happen, in this order, and each one is a different file.

1. `service.go` validates the input. A rejected input never reaches a policy.
2. `service.go` calls `security.Authorize(ctx, s.policy, actor, ACTION, record)`.
3. `policy.go` `Can` returns nil, or the reason it will not.
4. Only then does a `security.Grant` exist, and `repository.go` will not run a
   statement without one.

The Grant is the mechanism, and it is worth knowing exactly how much it
guarantees. `Authorize` is the only function in the framework that returns a
valid one; `Grant`'s fields are unexported, so a handler cannot assemble one.
What it does *not* stop is `security.SystemGrant`, which is exported and issues
a Grant with no policy involved. That is for work with no request behind it — a
job, a scheduled task — and in an application `aru doctor` reports it as
`system-grant-outside-scope` when it appears in a handler, and
`system-grant-without-tenant` when it is called with an empty tenant. A lint,
not the type system. Do not reach for it in this package.

## The procedure

**1. Write the rule inside the custom block.**

`policy.go` has one:

```go
	// arandu:begin custom
	if a == SkeletonView && (s.ID == record.ID || s.HasRole("admin")) {
		return nil
	}
	// arandu:end custom
```

Nothing regenerates this file in this repository, so here the markers cost
nothing and buy one thing: the same policy regenerated inside an application
keeps what is between them. `aru make:module --force` says so itself —
*"whatever sits between the arandu:begin custom markers is preserved"* — and a
rule written outside the pair is the rule that disappears.

What is not written inside the block stays closed, including every action added
later. The function ends in a refusal and there is no other exit.

**2. Say something about guests, or do not.** `security.Guest(tenant)` is the
subject a visitor with no readable session gets, and it is the only subject that
arrives with an empty `ID` and is still authorized at all. The zero `Subject` —
an empty `ID` with no guest marker — is refused by `Authorize` before `Can` is
consulted, because it is almost always a session that failed to load, and a
policy asked about nobody answers about nobody.
`TestAuthorizeRefusesASubjectThatIsNobody` at `tests/Unit/policy_test.go:91`
holds that apart from the guest case, and `TestThePolicyDeniesAGuest:79` proves
a guest is refused until somebody writes a rule for one.

**3. Leave the tenant check above the block alone.**

```go
	if record.ID != "" && record.TenantID != s.Tenant {
		return fmt.Errorf("skeleton belongs to another tenant")
	}
```

It runs before every rule, and it is the one refusal that has to survive
somebody opening the actions below it: a rule allowing an owner to read their
own record allows it across customers as soon as two customers have a record
with the same identifier. `TestThePolicyDeniesARecordOfAnotherTenant:62` asserts
the message contains `another tenant`, so the sentence is part of the contract.
The empty `ID` is exempt because it is the candidate that has not been stored
yet, and it belongs to nobody until it is written with the tenant off the Grant.

**4. Add the action to the test list if you added an action.** `everyAction` at
`tests/Unit/policy_test.go:31` is the whole set the default-deny tests walk. A
list holding four of five passes while the fifth is open, which is the only
state in which any of this matters.

**5. Run the gates.**

```sh
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go') \
  && go build ./... && go vet ./... && go test -race ./...
```

The default-deny tests will now fail for the action you opened, and that is the
suite doing its job. Change them to assert the rule you wrote — which subject is
allowed, which is still refused — rather than removing the action from
`everyAction`.

## Adding a repository method

Every method opens with `g.Check(THE_ACTION)`, and it is not a formality. The
Grant records which action it was issued for, so `Check` fails on three
different mistakes with three different messages: the zero Grant (`missing grant
for skeleton.view (call auth.Authorize first)`), a `SystemGrant` that was
refused, and a Grant issued for another action (`grant issued for
skeleton.delete, used on skeleton.view`). The third is the copy-paste between
two repository methods, and it is caught here rather than in review —
`TestTheRepositoryRefusesAGrantIssuedForAnotherAction:135`.

The signature is `(ctx, g, id)` and the order is the mechanism. A caller cannot
name a record without first holding a decision about it, and cannot leave the
decision out, because the call does not compile without it.

`data.Repository[T, ID]` requires all five methods — `Find`, `List`, `Create`,
`Update`, `Delete` — so the compile-time assertion in `repository.go` fails if
one is dropped, even one the package has no route for. Keep the method and its
`g.Check`; a repository with a hole in it is not a smaller repository.

## Reads are authorized like writes

There is no listing that skips the policy, and no read model, projection,
report or export that does. A read path with no check is a leak between
customers with a technical name.

`Find` in `service.go` calls `Authorize` twice, and the second call is the one
people delete:

```go
	g, err := security.Authorize(ctx, s.policy, actor, SkeletonView, Skeleton{})
	record, err := s.repo.Find(ctx, g, id)
	_, err = security.Authorize(ctx, s.policy, actor, SkeletonView, record)
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
  not come from a Grant is `Config.Tenant`, which is the customer a visitor with
  no session is read as, and it comes from the application's own configuration.
- **A check in the handler.** `module.go` handlers read the input, ask the
  service and answer. A handler that reached the repository would be a handler
  that skipped the policy, and nothing in the type system would object — the
  Grant is what objects, and only if it is absent.
- **A refusal that explains itself to the client.** `answer` in `module.go`
  sends a status and `"forbidden"`. Telling the client why a policy said no is
  telling them what exists and what does not, one request at a time. The reason
  is in the log, where the person operating the system reads it and the person
  probing it does not.
