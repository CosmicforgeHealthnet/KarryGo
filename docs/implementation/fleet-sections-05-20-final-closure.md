# Fleet Sections 5-20: closure report

Date 2026-11-14, final-closure pass 2026-11-16 (section 7), activation pass 2026-11-17 (section 8). Branch `staging` (never `main`, never production).
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
on staging, so they are reported here as live-certification blocked. Sections 5
and 6 need no tenant journey to certify: their evidence is structural,
contractual and runtime (running image, migrations, guards).

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
for the conventions. Sections 5 and 6 are closed including their runtime
evidence. Sections 7-20 are implemented and tested; their live certification
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

### 7.6 CI results on `3f8271af` (Customer V4 fix)

- Section 31 mandatory acceptance: success (run 43968504232).
- Customer V4 backend certification: **success** (run 43968504260), the
  first pass since at least 2026-11-13. All seven drills pass with the
  in-container `pg_dump` 16.16 (customer, taxi through 031, dispatch,
  hauling through 035, vendor, wallet through 000023, media).
- Deployment safety, Admin owner closure, Admin Sections 16-38, Vendor
  46-48, Partner/Gateway split, email branding: success.
- Admin bridge Sections 2-5: **failure**, `GO-2026-6859` only (7.5).
- Automatic deploy run 43968504452 selected no services; Fleet stays on
  `24f9b6a6`, which carries all service code at this HEAD.

## 8. Final activation pass (2026-11-17)

Starting point `38f71e73`. Commits: `ef9e5b39` (Go 1.26.11), `f0dbb379`
(Dispatch inventory in the evidence script), and this documentation commit.

### 8.1 Go security patch (done)

- Advisory `GO-2026-6859`, `net/http`, found in go1.26.10, fixed in
  go1.26.11 (confirmed from the failing scan, run 43972417436).
- Every remaining 1.26.10 pin moved to 1.26.11: `go.work`, Admin `go.mod`,
  `services/Dockerfile`, Notification and Vendor Dockerfiles, and seven CI
  workflows. Fleet was already on 1.26.11. No code change.
- Local: all 19 Go modules build and vet on 1.26.11; Admin `govulncheck`
  reports no vulnerable call path; Admin, Gateway, Partner Web and shared
  unit suites pass. (One Notification test fails only on a Windows
  CRLF checkout; it passes in CI.)
- CI on `f0dbb379`, all success: Admin bridge Sections 2-5 (44185272099,
  first pass since the advisory), Section 31 mandatory (44185272071),
  Customer V4 (44185272011), Deployment safety, Admin owner closure, Admin
  16-38, Vendor 46-48, Partner/Gateway split, email branding.
- Rollout (deploy run 44184998986): images for all ten affected services
  were built and pushed at `ef9e5b39`. Media, Notification, Verification and
  Support were deployed and reported healthy on Go 1.26.11. Wallet,
  Customer, Taxi, Hauling, Referral and Pricing were skipped by preflight
  (root-owned settings) and their patched images wait in GHCR; the run is
  red for exactly that reason. Dispatch, Admin, Gateway and Partner Web are
  excluded from automatic deploy by design and still run go1.26.10 builds.

### 8.2 Settings-file ownership (infrastructure access pending)

Evidence run 44185288548: `wallet.env`, `taxi.env`, `hauling.env`,
`customer.env`, `dispatch.env` (and `referral.env`, `pricing.env`) are
`root:root 600`. No sanctioned mechanism in the repository runs as root:
`server-setup.sh` requires an interactive root shell, and the deploy
account's Docker access is not used to change host file ownership (that
would bypass the permission control rather than satisfy it).

### 8.3 Dispatch 070 (human approval pending)

Prepared in `docs/deployment/dispatch-070-transition-package.md` from real
inventory: relay table has 0 rows, 0 rows would be fenced, one running
consumer, plus an exited old container with restart `unless-stopped` that
step 2 must neutralize. The staging environment has no required-reviewer
rule (admin bypass is now off), so the separate approval has no GitHub
mechanism yet and must be recorded in that package.

### 8.4 Status per service

| Service | Engineering | Deployed (revision) | Paired with Fleet | Live journey | Pending |
|---|---|---|---|---|---|
| Fleet API + worker | Complete | Yes, `24f9b6a6`, Go 1.26.11, migrations 000099 | n/a | Not certified | Authenticated test access |
| Verification | Complete | Yes, `ef9e5b39` | Yes | Not certified | Authenticated test access |
| Media | Complete | Yes, `ef9e5b39` | Yes (at-rest keys) | Not certified | Authenticated test access |
| Notification | Complete | Yes, `ef9e5b39` | Yes | OTP email delivery proven earlier (run 42977917689) | - |
| Payment Wallet | Complete | No, still `4d90bfb6` (000023 missing); `ef9e5b39` image ready | No | No | Infrastructure access (A1); then waiver removal |
| Taxi | Complete | No, still `4d90bfb6` (029-031 missing); image ready | No | No | Infrastructure access (A1) |
| Hauling | Complete | No, still `a7e08d0e` (034-035 missing); image ready | No | No | Infrastructure access (A1) |
| Customer | Complete | No, still `4d90bfb6`; image ready | n/a | No | Infrastructure access (A1) |
| Dispatch | Complete | No, still `9caa16b6`, 000068 | No | No | Infrastructure access (A1) and human approval (A2) |

Wallet waiver: still on. Its removal conditions (Wallet deployed, a real
organization wallet read back ACTIVE, activation refused without it) cannot
be checked until Wallet deploys.

Live journeys (Section 70 list): none run. Signed-in journeys need an
authorized inbox to receive the email OTP; no inbox is available to this
environment, and no OTP bypass was added.

### 8.5 Next phase

`fleet-sections-21-72-dependency-map.md` lists every section of 21-72 as
"start now" (Fleet-internal, most of them) or "build now, certify after" an
activation (37-42, 60, 70, 71 and the cross-service halves of 26/61).

## 9. Final activation and certification (2026-11-19)

Starting point `8141e154`, after the owner corrected the settings-file
ownership on the staging host as root. Commits: `acc270ef`, `d0c100d0`,
`ef3a2f9a`, `c3c2ac13` (read-only evidence), `953bfb29` and `4389654b`
(Verification allowlist fix), `678e0695` and `8f992eee` (build-only Dispatch
image). Deployment safety passed on each.

### 9.1 What was done

- **Ownership verified** (evidence run 44474983116): `wallet.env`,
  `taxi.env`, `hauling.env`, `customer.env`, `dispatch.env`, `referral.env`,
  `pricing.env` are deploy-owned, mode 600, readable by deploy.
- **Deployed** Wallet, Taxi, Hauling, Customer at `8141e154` (run
  44475350790). Each database was backed up first; all four reported
  healthy. Migration heads now: Wallet 000023 (23/23 in the deploy-tracked
  table), Taxi 031, Hauling 035.
- **Paired** (run 44476027696): new server-generated HMAC secrets for
  Fleet -> Wallet, Wallet -> Fleet, Fleet -> Hauling, Fleet -> Taxi,
  Hauling -> Fleet, Taxi -> Fleet; finance private API and outbox, wallet
  read/projection/reconciliation, sub-wallets, managed providers and the
  marketplace feeds turned on. All touched services healthy. The wallet
  waiver stays on.
- **Defect found and fixed.** Verification refused every Fleet company
  submission with 403 (12 refusals in 2 hours, run 44476672814), so the one
  staging company was stuck in BUSINESS_SUBMITTED / PENDING_DISPATCH. Cause:
  Verification's organization routes also check
  `VERIFICATION_COMPLIANCE_ALLOWED_CALLERS`, which on staging listed only
  `vendor-service,admin-backoffice-service`. The secret was correct, so the
  earlier "paired" status hid it. The pairing file now keeps both callers and
  adds Fleet (`4389654b`, test-pinned); applied by run 44478136628. Result:
  Verification accepted the submission (202) and the company and its request
  are now UNDER_REVIEW, with Fleet reading the decision back (GET 200).
  One 500 preceded the 202, seconds after Verification restarted with
  migrations on; Fleet retried with the same idempotency key and one
  submission exists. The handler does not log database errors, so the exact
  cause is unconfirmed.
- **Evidence extended** (aggregate only, no ids or payloads): trusted caller
  names per service, received internal calls by route template and status,
  outbox delivery state, Fleet organization / verification / wallet counts,
  Wallet organization wallets. A log-line count of auth refusals was tried
  and removed: it read 0 even during the 403s, because services do not log
  refused service calls.

### 9.2 Status per service (evidence run 44479278733)

| Service | Revision | Migrations | Trust (both ways) | Exercised live |
|---|---|---|---|---|
| Fleet API + worker | `24f9b6a6` | 000099 | trusts Hauling, Taxi, Wallet, Verification | Submission + decision read-back with Verification |
| Verification | `ef9e5b39` | current | trusts and allows Fleet | 202 / 200 to Fleet |
| Payment Wallet | `8141e154` | 000023 | trusts Fleet; finance callers default includes Fleet | No call yet: no organization wallet requested (0 wallets) |
| Taxi | `8141e154` | 031 | trusts Fleet; execution callers default Fleet | No call yet: no managed provider or corporate trip |
| Hauling | `8141e154` | 035 | trusts Fleet; Fleet callers default Fleet | No call yet: same |
| Customer | `8141e154` | startup-managed | n/a to Fleet | Healthy |
| Media / Notification / Support | `ef9e5b39` | current | trust Fleet | Unchanged |
| Dispatch | `9caa16b6` | 000068 | not paired (held) | Held behind the 070 transition |

Wallet waiver: on. 0 organizations ACTIVE, so no company is relying on it.

### 9.3 Live staging tests run without sign-in

- Fleet auth origin: the real web origin is trusted (422 on a deliberately
  invalid body, no code sent); a forged origin is refused (403
  untrusted_origin).
- Protected Fleet routes refuse anonymous calls: identity profile, sessions,
  wallet provisioning all 401; refresh without a cookie 401 session_expired;
  verify without a challenge 400 otp_invalid.
- Public gateway health 200; all staging containers healthy.
- Internal: Fleet -> Verification signed submission and read-back succeed.

### 9.4 Journeys that still need OTP sign-in

Every one starts with an email OTP (Fleet owner/staff), except item 1, which
needs an Admin operator (9.7):

1. Admin decision on the UNDER_REVIEW company (Admin bridge operator, two
   staff for approval).
2. Owner sign-in, then `POST /organizations/{id}/wallet-provisioning/retry`
   to provision the company wallet; read back ACTIVE. While the waiver is on,
   Fleet does not request a wallet on approval
   (`verification_outcome.go`), so this owner call is how a wallet exists
   before the waiver is removed.
3. Branches, staff and roles (role changes need step-up OTP).
4. CRM lead to order; private order with shipment; dispatch plan and alert.
5. Workforce shift, asset inspection, compliance check, specialized record.
6. Export, then confirm the stored object carries the encryption header.
7. Managed provider onboarding and marketplace offer / accept / cancel with
   Taxi and Hauling (also needs provider-app sign-in). This is the first
   journey that exercises Fleet <-> Wallet / Taxi / Hauling signed calls.

### 9.5 Certification

Engineering: 287/287 requirement rows built and proven by tests, 0
regressions. Runtime: every Fleet dependency except Dispatch now runs the
current code with its migrations, trusts Fleet in both directions and is
healthy; the one live integration path that real data reaches today
(company verification) works after the allowlist fix. Sections 5 and 6
remain closed with runtime evidence. Sections 7-20 are certified at the
infrastructure and integration level; their user-journey certification
needs the sign-ins in 9.4 and, for Dispatch-backed work, the reviewed 070
transition. Neither was claimed.

### 9.6 Remaining owner actions (complete list, superseded by 9.7.5)

Kept for history; the list in 9.7.5 replaces it.

### 9.7 Second closure pass (2026-11-19, evening)

Goal: remove the three blockers above without bypassing a control.

#### 9.7.1 Authenticated testing (email OTP)

- **Checked first.** Fleet sign-in is email OTP only. Fleet has no debug-code
  mode (`FakeSender` cannot be selected from configuration). There is no mail
  catcher, no test mailbox and no seeded test account on staging. Reading
  codes from the Notification database would be extracting authentication
  secrets, so it was not done.
- **The authorized inbox.** Staging test accounts are plus-addresses of the
  owner's Gmail (Fleet auth evidence run 42977917689 delivered an OTP there).
  The execution environment has a Gmail connector, but it refused with
  "needs to be connected in claude.ai". That is the only missing access. Once
  it is connected, the agent signs in as the test owner itself and runs 9.4
  items 2-7. No codes go into reports or CI.
- **Admin is not an OTP sign-in.** The Admin backoffice is the private mTLS
  bridge that only Cosmicforge HQ calls (`docs/admin-bridge/runbook.md`). It
  is switched off on staging (`check-admin-disabled.py`), and no operator is
  enrolled: there is no `trust.json`, no HQ certificate and no staff grants.
  `APPROVE_ORGANIZATION` requires a second staff approver, and grants cannot
  be self-issued. So a real company decision needs HQ to enrol two staging
  operators. It was not simulated.

#### 9.7.2 Security defect found and fixed: a company could be decided by its submitter

- **The defect.** Verification's organization routes used one caller list for
  submit, read and decide. The fix earlier today added Fleet to that list on
  staging, which also let Fleet record the decision on its own submission. The
  integration test did exactly that.
- **The fix** (`b2830b75`): decisions need
  `VERIFICATION_COMPLIANCE_ORGANIZATION_DECISION_CALLERS` (default
  `admin-backoffice-service`), checked before the nonce is consumed. Config
  refuses `fleet-partner-service` in it.
- **Tests:** a Fleet decision returns 403 (unit test and Postgres integration
  test), and the config tests cover the refusal. It deploys through the
  automatic staging deploy.

#### 9.7.3 Dispatch 070: reviewed execution path built and requested

- **Corrected inventory.** `migrate.sh` has recorded 000001-000068 as
  baseline and 000069 as applied, so exactly **000070-000079** are pending.
  The package said 000069-000079.
- **New dedicated environment** `dispatch-070-transition`: required reviewers
  ainzkoji and davidodimayo7, self-review prevented, no admin bypass, staging
  branch only. The shared `staging` environment is unchanged, so automatic
  deploys are not blocked.
- **New workflow** `dispatch-070-transition.yml` and script
  `scripts/platform/dispatch-070-transition.sh`. They implement the procedure
  and hold, never roll back, after migration. Tests in
  `scripts/release/test_dispatch_070_transition.py` run the real script
  against a simulated server: 107/107 release tests pass. Details are in
  package section 6.
- **Requested** (`eadb276a`). Run 44605516932 passed its plan job and is
  waiting at the review gate. GitHub reports that the operator account cannot
  approve it.

#### 9.7.4 CI on the final revisions

- **`eadb276a`:** every workflow passed. Section 31 and the four Admin and
  Customer suites were superseded by the next push and re-ran on `b2830b75`.
  The Dispatch 070 run is waiting for review, by design.
- **`b2830b75`:** every mandatory workflow passed, including the automatic
  deploy of the Verification fix.
  - Admin 16-38 and Customer V4 ran the Verification Postgres integration
    test, including the Fleet-decision 403.
  - Section 31 failed once, on notification-service
    `TestConsentWithdrawnAfterPlanningIsSuppressedAtSend`. That service has not
    changed since `401e4992`, where it passed. Re-running the failed job
    passed (run 44606253695), so this is a timing-sensitive test, recorded
    here as a flake.

#### 9.7.5 Remaining owner actions (complete list)

1. **Connect the Gmail connector** in claude.ai connector settings, for the
   account that receives the staging test plus-addresses. This unblocks owner
   sign-in, the company wallet (`wallet-provisioning/retry` works while the
   company is UNDER_REVIEW) and the waiver removal. It also unblocks 9.4 items
   3-7 except the parts that need an approved company or provider-app
   sign-in.
2. **Dispatch 070 approval:** ainzkoji or davidodimayo7 approves run
   44605516932 between 2026-11-20 08:00 and 2026-11-27 18:00 UTC. The run then
   executes and verifies itself. Fleet-Dispatch pairing follows the run.
3. **Company decision:** HQ enrols two staging Admin bridge operators (one
   requests, the other approves `APPROVE_ORGANIZATION`). Until then the company
   stays UNDER_REVIEW. Company activation and the journeys that need an active
   company cannot be certified, and nobody else may make that decision.

Not done, by design: no OTP bypass or debug mode, no reading codes from a
database, no decision recorded by a service identity, no self-approval of the
Dispatch run, and no change to main or production.
