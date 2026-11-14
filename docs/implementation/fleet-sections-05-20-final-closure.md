# Fleet Sections 5-20: closure report

Date 2026-11-14. Branch `staging` (never `main`, never production).
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

## 1. Final status per section

| Section | Rows | Engineering | Live certification | Final status |
|---|---|---|---|---|
| 5 Service structure | 12 | 12 closed | Fleet running on staging | **CLOSED - VERIFIED** |
| 6 API conventions | 14 | 14 closed | Fleet running on staging | **CLOSED - VERIFIED** |
| 7 Organizations | 16 | 16 closed | Fleet and media running; at-rest encryption live | **CLOSED - VERIFIED** |
| 8 Auth & lifecycle | 20 | 20 closed (8.3 superseded by the approved passwordless decision, contract-pinned) | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 9 Staff & IAM | 21 | 21 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 10 Managed providers | 23 | 23 closed | Hauling/Taxi not deployable (root-owned settings) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 11 Marketplace operations | 17 | 17 closed | Producers not deployable (Hauling, Taxi, Dispatch) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 12 Execution & trips | 8 | 8 closed | Dispatch fenced (migration 070); Taxi/Hauling blocked | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 13 Wallet | 17 | 17 closed | Wallet not deployable (root-owned settings); gate waiver still on | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 14 CRM | 26 | 26 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 15 Private orders | 22 | 22 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 16 Dispatch & tracking | 18 | 18 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 17 Workforce | 23 | 23 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 18 Assets | 16 | 16 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 19 Compliance & safety | 19 | 19 closed | Taxi/Hauling/Dispatch authorities not deployable | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| 20 Specialized logistics | 15 | 15 closed | Authenticated journey not executed (OTP inbox) | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** |
| **Total** | **287** | **287 closed** | | |

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
