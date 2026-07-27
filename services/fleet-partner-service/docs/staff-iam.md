# Staff, Roles and Permissions

Corporate staff identities are separate from managed provider identities.
Corporate permissions are never provider permissions, corporate sessions are
never provider sessions, and no identity is merged automatically.

## Membership lifecycle

```text
INVITED -> ACTIVE -> SUSPENDED -> ACTIVE
ACTIVE|SUSPENDED -> ENDED
INVITED -> EXPIRED | CANCELLED
```

One identity holds at most one live membership per organization (partial
unique index), and may belong to many organizations.

## Invitations

An authorized user supplies full name, **email and phone** (both mandatory),
roles, branch/department scope and expiry. The invitation email is sent
through `notification-service` and contains **no password and no temporary
credential**.

Tokens are `crypto/rand`, stored only as HMAC hashes, single-use, expiring,
organization-scoped and bound to the exact normalized email + phone they were
issued for. Acceptance happens inside the OTP verification transaction, so a
half-accepted invitation cannot exist; a replayed token fails.

## Owner protection

Every active organization keeps at least one active owner. Suspending or
ending the last active owner returns `last_owner_required` — the check locks
the row and counts other active owners in the same transaction, so the count
cannot drift underneath it.

## Authorization: deny by default

Authority exists only where an unexpired, unrevoked role binding grants the
permission in a scope covering the target. Nothing is inferred from token
claims, ownership flags or membership alone.

| Scope | Reach |
| --- | --- |
| `ORGANIZATION` | every branch and department |
| `BRANCH` | only that branch |
| `DEPARTMENT` | only that department |

Expired and revoked bindings grant nothing (both covered by tests).

## Role-version cache invalidation

Cached decisions are TTL-bounded **and** keyed by the membership's
`role_version`. Any role change bumps that version, so stale entries belong
to a key nobody looks up again — no invalidation broadcast is needed. Session
rows also carry `role_version`, so refresh picks up the change.

## System roles

Twelve system role templates are seeded by migration `000005` with
`organization_id IS NULL`. They cannot be updated, archived or
re-interpreted by an organization (`role_is_system_managed`). Organizations
may create custom roles.

## No privilege escalation

`RequireGrantable` refuses to grant any permission the acting user does not
themselves hold at organization scope, returning
`permission_escalation_forbidden`.

## Suspension is immediate

Suspending or ending staff revokes that membership's organization-scoped
sessions **in the same transaction** and bumps the role version, so access
stops at once rather than at the next token expiry. The underlying corporate
identity is untouched — it may still belong to other organizations.
