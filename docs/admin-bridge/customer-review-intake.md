# Customer-owned review intake

Status: implemented and tested owner contract; **BLOCKED_POLICY**, disabled by
default. A dedicated scoped Bridge submit/readback path is implemented. Existing
generic Bridge operation submission still durably rejects unsupported commands.

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
ownership/closure/retention rules; certify the real HQ integration and native owner
path in the target environment; provision private connectivity and separate keys;
verify real HQ and kill-switch/readback behavior. Do not merge/deploy this
privileged capability while approvals or engineering are outstanding.

## Verification

Real PostgreSQL tests exercise 12 concurrent duplicate requests, changed-intent
and fresh-key conflicts, real native review revisions, unchanged account status,
audit-failure rollback, immutable proof, current context revocation, signed HTTP
boundaries, key separation, durable nonce rejection, strict body parsing and the
actual typed client. Source grants and governance in these tests are ephemeral
TEST_ONLY approvals; they confer no production access.

## Dedicated Bridge orchestration

POST `/internal/karryam-admin/v1/customer/review-requests` accepts the exact
versioned Customer request DTO. GET the same path plus `/{operation_id}` resolves
only the original request. Both require the existing private mTLS, HQ HMAC v2,
delegated staff assertion, rate limit and audit boundaries. Query scope explicitly
includes market, zone, organization and tenant; product is fixed to Karryam.
Purpose, staff and the signed policy revision are bound to the operation. No
body actor, legacy two-dimensional command scope or arbitrary action is accepted.

Bridge migration 004 creates only orchestration and immutable audit metadata.
Apply it with the migration role and reapply the reviewed `admin-db-grants.sql`;
readiness requires version 4. Runtime can append audit and update only operation
status/receipt/time. It cannot change durable identity, delete operation evidence,
read the audit ledger or alter its guards. No Customer tables are accessed.

Before POST, the Bridge atomically commits EXECUTING plus audit and a unique
claim. A global original key, request ID and customer/review-revision fence prevent
parallel callers, changed intent or a fresh key from resubmitting that revision.
There is no automatic worker or transport POST retry. A process crash before POST
also remains fenced: safety takes precedence over silently replaying an action.
After any uncertain response, the state is UNKNOWN_OUTCOME (or remains EXECUTING
if persisting the status failed); either state permits only original GET recovery.
Absence/refusal is not a terminal failure and permits no second POST.

Current read delegation and source-owner authorization are checked on every GET,
including confirmed receipts. Readback must match original request/customer/key,
intent digest, native review revision and all five scope dimensions. Confirmation
and immutable audit commit together. Failed audit returns no receipt. Concurrent
readbacks cannot replace a previously confirmed receipt. This is immutable proof
of intake; account standing and fulfillment remain governed by native owners.

Bridge flags default false and read no secret files when both are off:

- `ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED`
- `ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED`

Writes require readback enabled. Set `ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE`
and `ADMIN_CUSTOMER_REVIEW_READBACK_SECRET_FILE` to the matching dedicated owner
keys. The origin comes from existing canonical Customer owner-resource wiring;
these keys must differ from each other, the resource key and HQ workload keys.
Readback-only startup needs no write key. Governance documents and independent
owner context keys are still required. Flags provision no policy, role or scope.

The compiled capability matrix marks submit AVAILABLE and readback READ_ONLY;
discovery changes either to DISABLED when flags, approved policy, delegated roles,
zones or tenants are missing. Production remains disabled and BLOCKED_POLICY.
Legacy generic operation/health contracts remain unchanged; their remote-write
indicator concerns the still-disabled generic intervention family.

Tests cover real PostgreSQL orchestration, 12 concurrent submissions, crash and
lost-response fences, exact proof validation, immutable evidence, least-privilege
runtime grants and current source denial after confirmation. A real mTLS Bridge
and typed HTTP owner transport test validates signed delegation and recovery;
the native Customer handler/SDK has separate actual-migration PostgreSQL tests.
Full real HQ to native owner target-environment certification remains outstanding.


The dedicated Bridge POST also requires both existing rollout switches
`ADMIN_REMOTE_WRITES_ENABLED` and `ADMIN_CUSTOMER_WRITES_ENABLED`, in addition to
the dedicated review-write/readback flags. Each is a kill gate, never an access
grant. Turning either off blocks new submissions and hides write availability;
current authorized original-identity GET readback remains independent. Regression
tests exercise both kill gates and readback after all write switches are off.


### Customer review queue lifecycle — 22 October 2026

Source-owned work-item ACKNOWLEDGE (`REQUESTED` → `ACKNOWLEDGED`) and CLOSE
(`ACKNOWLEDGED` → `CLOSED`) now have a separate typed contract
`customer.admin-review-work-item.v1`. Closure is explicitly
`REVIEW_CLOSED_WITHOUT_ACCOUNT_CHANGE`; it performs no account restriction,
consent decision, privacy fulfillment, Trip or Wallet mutation. Customer has no
canonical restriction/unrestriction or privacy/support fulfillment contract;
those actions remain unsupported, separately from initial policy approval.

Native POST `/internal/customer/v1/admin/review-work-item-transitions` and GET
`/{customer}/{key}` use the existing separate review write/readback audiences,
new delegated permissions `customer.review.work_item.transition` and
`customer.review.work_item.read`, current owner allocation, original intake scope,
real child work-item versions and immutable transition evidence. Only Customer
writes queue state. ACK does not close a case; CLOSE frees the pending review
aggregate and advances its revision, preserving every original intake receipt.

Private Bridge POST `/internal/karryam-admin/v1/customer/review-work-items` and
GET `/{operation_id}` commit a durable claim and immutable audit before one POST.
Case/customer/version fences prevent concurrent/fresh-key resubmission. All
uncertain/crashed outcomes recover using original GET only, never automatic POST
replay. Current owner authorization is rechecked even for confirmed receipts.

Normal test migrations are Customer 017 and Bridge 005. No live migration was
applied. Apply normal owner migration and updated least-privilege runtime grants
only during a separately authorized rollout; existing Bridge readiness still
supports migration4 when this optional feature is off. New flags default false:
`CUSTOMER_ADMIN_REVIEW_WORK_ITEMS_ENABLED`,
`ADMIN_CUSTOMER_REVIEW_WORK_ITEM_WRITES_ENABLED`,
`ADMIN_CUSTOMER_REVIEW_WORK_ITEM_READBACK_ENABLED`. Native transition requires
both existing review switches; Bridge writes require dedicated readback and both
existing remote/customer write kill gates. All disclosure/access remains
BLOCKED_POLICY until initial owner governance is approved.

The separate `review-work-items` list/detail resource exposes current native queue
state/version under `customer.review.work_item.state.read` and
`customer.admin-review-work-item-state.v1`. Its canonical scope is the original
immutable intake scope; a signed allocation cannot move the case to another
tenant. The finite resource dictionary is now 27 kinds / 54 paths.
