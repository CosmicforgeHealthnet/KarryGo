# Customer-owned review intake

Status: implemented and tested owner contract; **BLOCKED_POLICY**, disabled by
default. HQ command orchestration is not connected. Existing Bridge operation
submission still durably rejects unsupported commands.

The action records a Customer-owned work item and an immutable original-action
receipt. It changes no account status, verification, restriction, consent,
privacy fulfillment, bookings, Trip or Wallet state. `REVIEW_REQUEST_RECORDED`
means exactly that the owner persisted a pending request. It is not completed
review or account intervention.

## Source contracts and ownership

Customer migration 016 creates a separate native review aggregate with a real
stored revision, initially 1, and initializes it for existing/new Customer
records. `review-states` resource reads expose that aggregate only, under
`customer.admin-review-state.v1` and `customer.review.read`. The revision does
not version Customer profile/lifecycle updates and a read-model ETag is not a
mutation guard. Root records have actual Customer foreign keys; no geographic,
organization or tenant relationship is inferred.

POST `/internal/customer/v1/admin/account-review-requests` requires a separate
write key, exact signed delegated staff query, `customer.review.request` rule
for `customer.admin-review-request.v1`, and a current independently signed
source-owner allocation over the actual `review-states` root. Request bodies
cannot carry role, scope, status overrides or arbitrary owner destinations.
Unknown fields, duplicate keys and JSON case aliases are rejected.

The supplied `expected_review_revision` is checked under the native aggregate
row lock. Only an IDLE aggregate can accept a new request; an open native work
item has an additional unique constraint. Original identity includes target,
request ID, source revision, reason code, timestamp, staff, purpose and all five
scope dimensions. A changed intent under the same key conflicts. Fresh keys
cannot bypass a stale revision or pending review.

Case creation, native revision advance, immutable request proof and audit commit
atomically. Audit failure commits none of them. Evidence tables reject UPDATE,
DELETE and TRUNCATE. Owner retention/closure semantics still require review;
no destructive rollback or automatic case fulfillment is implemented.

GET `/internal/customer/v1/admin/account-review-requests/{customer}/{key}` uses
a distinct readback key and rechecks current read permission, source-owner
allocation and the exact original persisted scope. It commits disclosure audit
before returning proof. Turning off writes leaves readback available. Missing
or malformed proof never authorizes another POST or a new operation identity.
A 404 is not proof that a concurrent/uncertain request failed to commit.

The typed `shared/go/customerreview` client has fixed owner paths, separate keys,
no proxy or redirects, bounded strict responses, no browser credential forwarding
and no automatic POST retry. It validates target, original identity, native
revision, intent digest and five-dimensional scope. Readback issues GET only.

## Default-disabled configuration

Both switches default false and read no secret files or schema when off:

- `CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED`
- `CUSTOMER_ADMIN_REVIEW_READBACK_ENABLED`

When enabled, the existing Customer source-resource boundary must already be
initialized, with approved governance and independent source-owner keys. Mount
`CUSTOMER_ADMIN_REVIEW_WRITE_SECRET_FILE` and/or
`CUSTOMER_ADMIN_REVIEW_READBACK_SECRET_FILE` as raw-base64url secrets of at least
32 bytes. These keys must differ from the resource-read key and each other.
Switches cannot create policy grants. No production values or keys are supplied.

Before any production rollout: approve field/purpose/capability/scope and work-item
ownership/closure/retention rules; complete HQ versioned command transport and
uncertainty orchestration; provision private connectivity and separate keys;
verify real HQ and kill-switch/readback behavior. Do not merge/deploy this
privileged capability while approvals or engineering are outstanding.

## Verification

Real PostgreSQL tests exercise 12 concurrent duplicate requests, changed-intent
and fresh-key conflicts, real native review revisions, unchanged account status,
audit-failure rollback, immutable proof, current context revocation, signed HTTP
boundaries, key separation, durable nonce rejection, strict body parsing and the
actual typed client. Source grants and governance in these tests are ephemeral
TEST_ONLY approvals; they confer no production access.
