# Fleet Sections 5-20: integration and dependency report

Date 2026-11-14. Branch `staging`. All integrations below are internal
Karryam services reached over HMAC service authentication; none is treated
as external. Third-party dependencies are listed separately at the end.

## 1. Integration matrix

| Pair | Direction / contract | Code | Tests | Staging |
|---|---|---|---|---|
| Fleet -> Verification-compliance | submission, read-back, decisions (`platformclients` verification) | live | `company_activation_test.go`, verification outcome tests | Paired and healthy (run 43563718915) |
| Verification -> Fleet | verification events | live | same | Paired |
| Fleet -> Payment-wallet | organization wallet provisioning and read-back; provider sub-wallets; managed settlement instructions with payment binding | live (Wallet 000023, Fleet 000097) | `row_08_12_activation_wallet_gate_test.go`; Wallet `fleet_wire_managed_settlement_test.go` (8 tests incl. concurrent sends, reversal, ambiguity refusal) | **Blocked:** `wallet.env` root-owned; pairing skipped; gate waived on staging |
| Payment-wallet -> Fleet | finance events | live | finance ingestion tests | Blocked (same) |
| Fleet -> Dispatch | marketplace reservation/execution (native dialect), private executions, organization trip cancel (new), restrictions, rider provisioning | live | `trip_cancel_integration_test.go` (Dispatch), `marketplace_trip_cancel_test.go`, `dispatch_native_execution_test.go`, organization provider tests | **Blocked:** Dispatch deploy waits for the reviewed migration-070 transition |
| Dispatch -> Fleet | `platform.request.offered.v1`, trip events, completed work with payment reference | live | event ingestion and S1112 tests | Blocked (same) |
| Fleet -> Hauling | execution adapter, trip cancel, restrictions, provider consent | live | marketplace and restriction tests | **Blocked:** `hauling.env` root-owned |
| Hauling -> Fleet | marketplace feed (Hauling 035), trip events | live | Hauling suite (37 packages ok) | Blocked (same) |
| Fleet -> Taxi | execution adapter, trip cancel, restrictions (new: Taxi 031, Fleet 000098) | live | `taxi_restriction_propagation_test.go`; Taxi restriction tests | **Blocked:** `taxi.env` root-owned |
| Taxi -> Fleet | marketplace feed (Taxi 030), trip events | live | Taxi suite (45 packages ok) | Blocked (same) |
| Fleet -> Media-file | documents, exports, proof files | live | export and document tests | Live |
| Fleet -> Notification | operational alerts with notification intent | live | dispatch alert tests, 11.11c | Live |

Marketplace producers and consumers: Hauling, Taxi and Dispatch produce
`platform.request.offered.v1`; Fleet fans out one projection and offer per
eligible organization, accepts through the source reservation, assigns,
creates execution, projects the trip lifecycle, reconciles gaps by
read-back, cancels through the source, and raises a dispatcher alert on
expiry. Every leg has a PostgreSQL test; the live leg waits on the owner
actions below.

## 2. Duplication audit

- One Fleet service; no second implementation of any feature.
- Workers: one expiry worker, one reassignment worker, one command
  reconciler (now also resolving trip cancellations), one marketplace
  reconciliation worker, one alert SLA worker. The new expiry alert reuses
  the dispatch alert path rather than adding a notifier.
- Events: reused `operational_alert.created.v1`; no new event type was added
  for trip cancellation (the source's trip event moves the projection).
- Tables: no replacement tables. New tables only where none existed
  (migrations 000089-000099, listed in the API/database audit).
- APIs: the trip-cancel route is new because no cancel existed; Dispatch's
  private-execution cancellation stays for private orders and the new
  marketplace route serves marketplace trips (different keys, same trip
  state machine underneath).

## 3. Owner actions (inaccessible authorized infrastructure)

As root on the staging host, keeping mode 600:
`chown deploy:deploy /opt/karrygo/{payment-wallet-service/wallet.env,customer-service/customer.env,taxi-service/taxi.env,hauling-service/hauling.env}`.
Then re-run the deploy and pairing workflows. Dispatch needs its reviewed
migration-070 transition.

## 4. Third-party dependencies

- Paystack (Wallet payouts): not exercised; no real transfers were made.
- Firebase Storage (media at rest): encryption at rest is the provider's;
  Fleet does not encrypt exports itself (FLEET-7h.c).
- No SMS provider was introduced.
