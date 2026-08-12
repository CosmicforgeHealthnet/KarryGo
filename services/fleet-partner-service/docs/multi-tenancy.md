# Multi-Tenancy and Organization Isolation

Every corporate record is owned by an organization; branch-level records also
carry branch ownership. Isolation is enforced **in repositories and SQL**,
not only in handlers.

## Tenant context

`internal/platform/tenant.Context`:

```go
type Context struct {
    OrganizationID     string
    OrganizationUserID string
    ActorType          ActorType
    ActorID            string
    PermittedBranchIDs []string // empty = all branches of the organization
    RequestID          string
    CorrelationID      string
}
```

Rules (all enforced and tested):

- Resolved only from trusted authentication: verified access-token claims
  plus `MembershipResolver` lookup (corporate routes) or validated internal
  service claims. The staff/IAM work package will provide the real resolver;
  until then `UnconfiguredResolver` fails closed.
- Membership must be `ACTIVE`. A missing membership and a suspended
  membership both refuse tenant scope with `403` and the same public message,
  so membership state is not disclosed.
- Organization IDs in request bodies, query strings or ad-hoc headers never
  override the resolved scope (tested in `internal/app/app_test.go`).
- Missing or invalid tenant context fails closed (`tenant.Require`).
- Internal system operations explicitly state whether they are tenant-scoped
  or platform-scoped.
- Jobs and events reconstruct and revalidate tenant context: the worker
  compares the payload's `organizationId` with the durable job row before
  execution; a mismatch dead-letters with a security error code.

## Repository filtering

Organization-owned repository methods require the organization ID explicitly
or a tenant-scoped query object:

```go
GetByID(ctx, organizationID, id)
Archive(ctx, organizationID, id, expectedVersion)
List(ctx, tenant.Query{OrganizationID: ..., ...})
```

Every SQL statement on an organization-owned table includes
`organization_id = $n` (spot-checked by an architecture test). Branch-scoped
listings additionally enforce `PermittedBranchIDs` **in the SQL predicate**
(`cardinality($2::text[]) = 0 OR id = ANY($2::text[])`) — an empty list means
"all branches of this organization", a non-empty list is a hard restriction
that no user-supplied filter can widen. Resources are never loaded globally
and ownership-checked afterwards.

Durable job dedupe keys are organization-scoped too
(`COALESCE(organization_id,''), job_type, dedupe_key`), so one organization's
key can never suppress another organization's job.

## Uniqueness

Business uniqueness includes the organization:

```sql
UNIQUE (organization_id, code)          -- branches, departments
UNIQUE (country_code, registration_number) -- organizations (confirmed-platform rule)
```

Tenant-private identifiers are never globally unique.

## IDOR-safe access

Cross-organization reads and writes return the same `not_found` as a
genuinely missing resource — existence in another organization is never
revealed. Verified by integration tests (`TestBranchTenantFilteringAndOptimisticVersion`).

## Files

`fleet_partner_file_references` stores `organization_id`, `file_id`,
`purpose`, `subject_type`, `subject_id`, `retention_class` and
source/readiness metadata only. No binary data, no permanent public URLs;
`media-file-service` owns the files.

## Events

Every corporate event envelope carries `organizationId`, `correlationId`,
aggregate type/ID/version, globally unique `eventId`, versioned `eventType`,
`occurredAt` and `producer`. Organization-owned events without an
organization ID are rejected at construction; platform-scoped events must be
explicitly flagged and documented in [events-and-jobs.md](events-and-jobs.md).

## Jobs

Organization-owned job payloads include `organizationId`; the worker
revalidates it against the job row before execution (see above).

## Exports (foundation only)

Future export jobs must be tenant- and permission-scoped, protected at rest,
time-limited, audited, delivered through `media-file-service`, and
re-authorized at download time. Only this model is defined in the foundation.
