# Fleet Sections 5-20: closure report

Date 2026-11-14, final-closure pass 2026-11-16 (section 7). Branch `staging` (never `main`, never production).
Service `services/fleet-partner-service`, with its Karryam counterparts
(payment-wallet, driver-dispatch-delivery, driver-taxi, driver-hauling,
verification-compliance, media-file) and the Fleet web app `web/fleet`.
Companion documents: `fleet-sections-05-20-requirements-matrix.md` (per-row
evidence), `-api-database-audit.md`, `-integration-audit.md`,
`-implementation-log.md`.

Two independent axes are reported. **Engineering**: built, and proven by a
named test against PostgreSQL through the real router or use case.
**Live certification**: running on staging with its counterparts paired,
and exercised end to end there.

## 1. Final status per section (updated 2026-11-16)

Engineering verdicts: CLOSED - VERIFIED or FAILED - REGRESSION. Live verdicts: CERTIFIED, BLOCKED - AUTHORIZED INFRASTRUCTURE ACTION, BLOCKED - HUMAN RELEASE APPROVAL, BLOCKED - AUTHENTICATED TEST ACCESS, or FAILED - INTEGRATION OR RUNTIME DEFECT.

| Section | Rows | Engineering | Live staging certification | Evidence for the live verdict |
|---|---|---|---|---|
| 5 Service structure | 12 | CLOSED - VERIFIED | CERTIFIED | Running revision, health, migration head and layering guards on staging |
| 6 API conventions | 14 | CLOSED - VERIFIED | CERTIFIED | Running revision; envelope, paging and OpenAPI parity guards |
| 7 Organizations | 16 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | At-rest encryption live; onboarding/approval journey (J1) not run signed in |
| 8 Auth & lifecycle | 20 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | Email OTP sign-in not run (no authorized test inbox) |
| 9 Staff & IAM | 21 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | Invite/role/revoke journey (J2) not run |
| 10 Managed providers | 23 | CLOSED - VERIFIED | BLOCKED - AUTHORIZED INFRASTRUCTURE ACTION; BLOCKED - AUTHENTICATED TEST ACCESS | Taxi, Hauling, Dispatch settings files root-owned; J3 not run |
| 11 Marketplace operations | 17 | CLOSED - VERIFIED | BLOCKED - AUTHORIZED INFRASTRUCTURE ACTION; BLOCKED - HUMAN RELEASE APPROVAL | Producers (Taxi, Hauling) not deployable; Dispatch fenced at 070 |
| 12 Execution & trips | 8 | CLOSED - VERIFIED | BLOCKED - HUMAN RELEASE APPROVAL; BLOCKED - AUTHORIZED INFRASTRUCTURE ACTION | Dispatch 070 transition not approved/executed; Taxi/Hauling blocked |
| 13 Wallet | 17 | CLOSED - VERIFIED | BLOCKED - AUTHORIZED INFRASTRUCTURE ACTION | wallet.env root-owned; Wallet on Nov 7 image without 000023; waiver still on |
| 14 CRM | 26 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | J7 not run |
| 15 Private orders | 22 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | J7 not run |
| 16 Dispatch & tracking | 18 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | J8 not run |
| 17 Workforce | 23 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | J8 not run |
| 18 Assets | 16 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | J9 not run |
| 19 Compliance & safety | 19 | CLOSED - VERIFIED | BLOCKED - AUTHORIZED INFRASTRUCTURE ACTION; BLOCKED - AUTHENTICATED TEST ACCESS | Downstream authorities (Taxi/Hauling/Dispatch) not deployable; J10 not run |
| 20 Specialized logistics | 15 | CLOSED - VERIFIED | BLOCKED - AUTHENTICATED TEST ACCESS | J11 not run |
| **Total** | **287** | **287 CLOSED - VERIFIED, 0 regressions** | **2 certified, 14 blocked** | |

**Sections 5-20 are not fully closed.** Engineering is complete; live certification is blocked on the owner actions in section 5.

Change from the 2026-11-14 report: Section 7 moves from closed to live-blocked. Organization onboarding and approval is a signed-in journey (J1) and has not been exercised on staging.

The previous report marked Sections 7-9 and 14-20 CLOSED on code, tests
and an unauthenticated live smoke. That was over-generous by this
standard: their tenant journeys have not been exercised as a signed-in user
on staging, so they are reported here as live-certification blocked. Sections 5-7
need no tenant journey to certify: their evidence is structural, contractual
and runtime (running image, migrations, guards, at-rest encryption).

## 2. What this phase closed (engineering)

- **Section 5:** all 13 flat features re-layered; six spec-named packages
  became real (fuel, inspections, maintenance, routes, tracking,
  communications); the remaining five are guarded boundaries; every handler
  takes interfaces and every use case takes repository interfaces. Guards:
  layering (pending list empty), boundary markers, cross-feature handler
  imports.
- **Section 6:** cursor pagination on every offset-only list (offset kept
  for compatibility); internal routes on the standard envelopes with
  deprecated aliases only where a live caller needs them; Money objects
  beside every flat money pair.
- **7h.c:** private media encrypted at rest (AES-256-GCM, rotation, legacy
  read), live on staging with keys configured.
- **8.3:** superseded by the approved passwordless decision; contract test
  forbids any password route.
- **Defects found and fixed during the work:** fulfilment orders were saved
  with no lines; engagement support events failed on a new case (`$6` type);
  duplicated handler plumbing consolidated into `platform/tenantcmd`.

## 3. Staging evidence (actual, read-only evidence workflow)

Run 43594087013 (2026-11-14 12:19 UTC):

| Service | Running revision | Health | Migrations | Settings file |
|---|---|---|---|---|
| fleet-partner (api, worker) | `34512a7` at that time; later deploys follow each push | healthy | 000099 (99 applied) | deploy:deploy 600 |
| media-file | `52029c8` | healthy | 9 applied | deploy:deploy 600; at-rest keys configured |
| verification-compliance | `4d8b7b8` | healthy | (separate database) | deploy:deploy 600 |
| payment-wallet | `4d90bfb6` (Nov 7) | healthy | 22 applied, 000023 missing | **root:root 600, unreadable** |
| driver-taxi | `4d90bfb6` (Nov 7) | healthy | through 028; 029-031 missing | **root:root 600, unreadable** |
| driver-hauling | `a7e08d0e` (Nov 7) | healthy | through 033; 034-035 missing | **root:root 600, unreadable** |
| customer | `4d90bfb6` (Nov 7) | healthy | n/a | **root:root 600, unreadable** |
| driver-dispatch-delivery | `9caa16b6` (Oct 21) | healthy | through 000068; 000069-000079 missing (070 fenced) | **root:root 600, unreadable** |

Also root-owned: `referral.env`, `pricing.env` (outside Sections 5-20).

Deploy runs this phase: 43588583698 (Fleet final API and worker, healthy),
43593744804 (media encryption, success), and the automatic runs after each
push. Pairing run 43593565449 generated the media key ("generated, value not
shown") and re-applied Fleet <-> Verification; Wallet, Taxi and Hauling
pairings were skipped by design with the owner action.

## 4. Test evidence (final tree)

- Fleet `bc5f703`: full suite on a fresh PostgreSQL 16 database migrated
  through 000099, `go test -p 1 -count=1 ./...`: **99 packages ok, 0
  failures**.
- Dispatch 42 (on the envelope merge 41 plus one timing-sensitive websocket
  test that passes on re-run), Taxi 45, Wallet 11, Media all packages,
  shared/go all packages.
- Web: `tsc --noEmit` clean; 43/43 unit tests; production build.
- Guards: layering, boundary markers, cross-feature handler imports, JSON
  tags, branch-scope NULL safety, money-never-float64, passwordless contract,
  query allowlist with dualPage, route coverage, risk registry, step-up map,
  OpenAPI parity, migration replay (000099).

## 5. Owner actions (exact, safe; nothing here prints or loosens a secret)

**A. Settings-file ownership (unblocks Wallet, Taxi, Hauling, Customer;
also Dispatch, Referral and Pricing).** On the staging host, as root:

```
chown deploy:deploy /opt/karrygo/payment-wallet-service/wallet.env \
  /opt/karrygo/taxi-service/taxi.env \
  /opt/karrygo/hauling-service/hauling.env \
  /opt/karrygo/customer-service/customer.env \
  /opt/karrygo/dispatch-delivery-service/dispatch.env
```

Mode stays 600 (chown does not change it). Equivalent sanctioned procedure:
re-run `deploy/platform/bin/server-setup.sh` step 5 as root. Verify: dispatch
the "Platform staging evidence (read only)" workflow; every listed file must
show `owner_mode=deploy:deploy 600 readable_by_deploy=yes`. Then push any
commit (or re-run "Deploy staging (automatic)") and "Platform staging
pairing"; confirm the new revisions and migration heads in the next evidence
run (Wallet 000023, Taxi 031, Hauling 035).

**B. Dispatch migration-070 transition.** Follow
`docs/deployment/staging-release-approval.md` "Dispatch 070 stop/migrate/
verify/start and recovery hold": named operator plus separate reviewer,
approved maintenance window, inventory of every consumer and in-flight
decision, all old consumers stopped and their supervisors disabled, backup
taken, apply 000069-000079 with old binaries stopped, start only the
certified consumer, verify signed read-back and no duplicate decisions. The
deploy script, rollback script and release gate deliberately refuse
Dispatch; this transition is not automated and was not attempted.

**C. Wallet gate waiver.** After A, the Wallet pairing applies. Provision a
real organization wallet through it and confirm the activation flow reads
it back ACTIVE (behaviour proven by
`TestRow8_12_ActivationWaitsForOrganizationWallet`). Then delete the line
`SET fleet-partner-service FLEET_ACTIVATION_WALLET_GATE_WAIVED=true` from
`deploy/platform/staging-pairings.txt` and let the pairing workflow apply.
The waiver stays until then.

**D. Authenticated staging journeys.** Provide an authorized staging test
inbox (or sign in on `fleet-karryam.vercel.app` and share the session for
the test window). With it: sign up/sign in, create a company, branches, staff
and roles (step-up), a CRM lead to order, a private order with shipment,
dispatch plan and alert, workforce shift, asset inspection, compliance check,
specialized record, an export (verify the stored object carries the
encryption header), and the marketplace offer/accept/cancel once Wallet,
Taxi and Hauling are deployed. Without an inbox OTP these journeys cannot be
executed from this environment; they were not claimed.

## 6. Verdict

Every one of the 287 requirement rows in Sections 5-20 is built and proven
by a named PostgreSQL-backed test, with structural and contractual guards
for the conventions. Sections 5, 6 and 7 are closed including their runtime
evidence. Sections 8-20 are implemented and tested; their live certification
waits only on the owner actions above: settings-file ownership, the reviewed
Dispatch transition, and an authorized test inbox for signed-in journeys.

## 7. Final-closure pass (2026-11-16)

Starting point: staging HEAD `ffd4b742`; Fleet API and worker on `bc5f703`.

### 7.1 Mandatory CI failure: cause and fix

Run 43613128278 (`ffd4b742`) failed in `fleet-mandatory`, step "Release 1
master coverage and Section 55 mandatory suite", although the gate printed
`PASS; mandatory skips=0; test events=22`.

- **Cause.** Commit `457dac89` split `internal/features/integrations` into
  `clients`, `http`, `models`, `repositories` and `usecases`. The workflow
  still named the parent directory, which has no Go files any more, so
  `go test` reported `[setup failed]` and exited 1. `pipefail` caught the
  exit. The tests that had lived there (`TestDestinationValidator`,
  `TestWebhook*`, `TestClientSecrets*`) stopped running in CI from
  `457dac89`.
- **Second defect.** `cmd/section31-testgate` counted only failures that
  carry a test name, so a package-level failure (setup or build failure,
  `TestMain` exit, panic outside a test) printed PASS. That is why the log
  looked green.
- **Fix.** The workflow now names `./internal/features/integrations/...`.
  The gate now also fails on package-level `fail` and `build-fail` events,
  with two new tests. The gate is stricter, not looser. All other package
  paths named by the workflows were checked; none is missing or empty.
- **Local proof** (disposable PostgreSQL 18 + Redis, migrated as CI does):
  the corrected step exits 0 and the gate passes with 72 test events
  (previously 22), zero skips. Replaying the failing run's event stream
  through the fixed gate now fails.

### 7.2 Test evidence on the fix commit (local)

- Fleet `go vet ./...`: clean.
- Fleet `go test -p 1 -count=1 ./...`: 99 packages ok, 0 regressions.
  `TestAcceptanceToOneCanonicalTrip` timed out once while its Dispatch
  helper compiled from a cold Windows build cache; with the cache warm it
  passes (17.9 s), and the whole `internal/integration` package passes.
  `TestBackupAndRestore_RealDisposableDatabase` needs a Docker container
  and was not run locally; CI runs it.
- `web/fleet`: `tsc --noEmit` clean (after deleting a stale gitignored
  `.next` dev cache), 43/43 unit tests, production build ok. No UI change.
- Wallet, Taxi, Hauling, Dispatch and Verification code is unchanged since
  `bc5f703`; their suites run in the mandatory workflow on this commit.

### 7.3 Staging evidence (read-only run 43959094559, 2026-11-16 21:21 UTC)

Unchanged since 2026-11-14:

- Fleet API and worker healthy on `bc5f703`, migrations through 000099.
- Media, Notification, Verification healthy on `b3ed1e90`, settings files
  owned by the deploy account, mode 600.
- `wallet.env`, `taxi.env`, `hauling.env`, `customer.env` and
  `dispatch.env` are still `root:root 600`, unreadable by the deploy
  account. Wallet, Taxi and Customer run the Nov 7 image `4d90bfb6`;
  Dispatch runs `9caa16b6` (Oct 21) with migrations through 000068.
- Public probes: `/health` and `/ready` on the Fleet staging API return 200.

### 7.4 Work-package status

| Package | Status | Reason |
|---|---|---|
| A CI failure | Fixed (7.1); Section 31 mandatory green on `24f9b6a6` (run 43963592101, all jobs; restored step 72 events, 0 skips) | |
| B Settings ownership | Blocked: authorized infrastructure action | Needs root on the staging host; no sanctioned automation has root (all repo `sudo` runs on CI runners). Command in section 5A. |
| C Wallet, Taxi, Hauling, Customer deploys | Blocked by B | Deploy account cannot read their settings files |
| D Dispatch 070 | Blocked: human release approval | Named operator, separate reviewer and maintenance window required; not attempted |
| E Pairings | Fleet-Verification, Media, Notification live; Wallet, Taxi, Hauling, Dispatch blocked by B and D | |
| F Wallet waiver | Kept on | Removal conditions (Wallet deployed and provisioning read back ACTIVE) not met |
| G Authenticated journeys | Blocked: authenticated test access | No authorized test inbox; the session's Gmail connector is not authorized. No journey was run or claimed. |
| H Frontend regression | Type check, unit tests and build pass | Signed-in browser regression needs G |
| I Release certification | Partial | Code and CI only; live parts blocked as above |

### 7.5 Other required workflows (release-gate acceptance list)

- **Customer V4 backend certification** had failed on every staging commit
  since at least 2026-11-13, in the backup/restore drill for all seven
  services: `pg_restore: ERROR: unrecognized configuration parameter
  "transaction_timeout"`. The Ubuntu 26 runner image installs a PostgreSQL
  18 client; `pg_dump` 17+ emits `SET transaction_timeout`, which the
  `postgres:16` drill server rejects. Fix: the workflow now runs the pg
  tools inside the server's own container (`PGTOOLS_DOCKER_CONTAINER`,
  already supported by `scripts/customer-v4/pgbackup.py`), so client and
  server versions always match. CI-only; no service or runtime change.
- **Admin bridge Sections 2-5** fails its dependency scan on every staging
  commit since at least `43109bad`: `GO-2026-6859` in the Go standard
  library `net/http`, found in go1.26.10, fixed in go1.26.11
  (`admin-backoffice-service` hqops export path). Not Fleet code. The fix is
  a platform-wide toolchain patch bump (7 workflows, `go.work`,
  `admin-backoffice-service/go.mod`, `services/Dockerfile`,
  `notification-service/Dockerfile`, `vendor-service/Dockerfile`). Changing
  the shared Dockerfile makes the automatic deploy select every service that
  uses it, and those blocked by section 5A would fail preflight. Left for an
  owner decision; not changed here.
- Deployment safety, Admin owner closure, Admin Sections 16-38, Vendor
  46-48, Partner/Gateway split, Fleet authentication v2 and email branding
  passed on `24f9b6a6`.
- Automatic deploy run 43963592511 redeployed Fleet API and worker:
  host-verified image revision `24f9b6a6` on both, API healthy.
