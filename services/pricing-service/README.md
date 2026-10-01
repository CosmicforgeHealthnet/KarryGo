# pricing-service

KarryGo's central pricing authority. It decides the economic price of a trip,
delivery or haulage job, and how that amount is split between customer,
merchant, provider, KarryGo and tax.

It is the **only** place a marketplace price may be decided. Providers and
customers never set or negotiate a fare, and after cutover no other service
keeps a production fare formula.

| | |
|---|---|
| HTTP | `:8115` |
| Postgres | `5451` / `pricing_service` |
| Redis | none |
| Public API base | `/api/v1/pricing` (only `/meta`; customers never call pricing) |
| Internal API base | `/internal/pricing/v1` (HMAC-signed, per-group caller allowlist) |
| Env prefix | `PRICING_` |

## What it does not do

- **It does not hold or move money.** It returns economic instructions;
  `payment-wallet-service` executes them and must not recompute a fare or a fee.
- **It does not own bookings.** Taxi, Dispatch and Haulage keep the trip.
- **It does not do routing.** Callers supply a trusted route snapshot. Pricing
  validates it and records how good it was.
- **It does not call external data sources while pricing.** Fuel and cost data
  arrive through a governed import, so a source outage can never stop a customer
  getting a quote, and can never turn a fuel price into zero.

## The rules that shape the code

1. **Commercial values are data, never code.** No rate, minimum fare or
   commission appears in Go or in the environment. They are versioned policy
   rows, created as drafts and activated through the approval API, so every rate
   in production has an approver's name against it.
2. **Used records are immutable, enforced by the database.** Two triggers exist:
   `pricing_reject_mutation` is absolute and protects evidence (audit,
   activations, approval decisions); `pricing_reject_mutation_purgeable`
   additionally allows the retention purge to delete expired *unbound* quotes.
   They are separate functions because a shared escape hatch would let any
   session delete audit rows by setting a flag.
3. **A quote is reproducible.** Reconciliation uses the original quote's policy
   and index versions, even after newer ones are active. Records are sealed with
   a SHA-256 over a canonical encoding, so tampering is detectable.
4. **Arithmetic is exact.** Whole kobo, whole metres, whole seconds; rates are
   integers over an integer unit ("kobo per 1000 metres"), multipliers are exact
   rationals, and each rounding step names its mode. The architecture test
   forbids floating point in the engine. This is what makes a quote replayable:
   a float64 kilometre is why the legacy formulas cannot be.
5. **Four-eye approval.** The operator who proposes a material change may not
   approve it. Cannot be disabled in production.
6. **It fails safely.** If pricing cannot produce a trustworthy price it refuses,
   rather than guessing a cheap fare. There is deliberately no fallback formula,
   because a fallback formula is a second pricing authority.

## Running it locally

```bash
bash scripts/bootstrap_services/pricing-local-bootstrap.sh
cd services/pricing-service && go run ./cmd
```

```bash
curl localhost:8115/health
curl localhost:8115/ready
curl localhost:8115/api/v1/pricing/meta
```

`/ready` reports not-ready until every migration is applied, so a replica on
older schema than its policy data expects does not take quote traffic.

Apply migrations without starting the service:

```bash
cd services/pricing-service && go run ./cmd/migrate
```

## Tests

```bash
go test ./...
```

Database-backed tests skip unless a test database is configured:

```bash
PRICING_TEST_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5451/pricing_service?sslmode=disable' \
  go test ./... -count=1
```

Because several tables are append-only by trigger, those tests cannot delete
what they write: each uses unique identifiers and asserts on its own rows rather
than on table counts.

## Administration

Pricing has no login of its own. Administration arrives as an HMAC-signed request
from `admin-backoffice-service` (or the ops CLI) naming the person responsible in
`performed_by`, under `/internal/pricing/v1/admin`. Every signed request is
single-use, and a caller holding a quote secret still cannot change a rate.

| Route | Purpose |
|---|---|
| `POST /policies/drafts` | Create a new version as a draft |
| `PUT /policy-versions/:id` | Edit a draft (refused once it leaves DRAFT) |
| `POST /policy-versions/:id/submit` | Open an approval bound to the body's hash |
| `POST /policy-versions/:id/approve` \| `/reject` | Decide — never the submitter |
| `POST /policy-versions/:id/activate` | Make it live, now or at a scheduled moment |
| `POST /simulate` | Price a request against any version, storing nothing |
| `GET /resolve` | "Which rates price this place right now?" |
| `GET /approvals` | The review queue |
| `GET\|POST /rollouts` | The pricing half of the two-key switch |
| `POST /markets/:code/freeze` \| `/unfreeze` | Hold a market's prices still |

### The ops CLI

`cmd/pricingctl` exists because the admin screens do not yet. It goes through the
same signed API and the same four-eye checks an admin UI will — there is no back
door into the database.

```bash
export PRICING_OPS_SECRET=...            # the pricing-ops-cli secret
go run ./cmd/pricingctl import-draft --file seeds/v1/taxi_tariff_ng.json --by ada@karrygo --reason "V1 baseline"
go run ./cmd/pricingctl submit   --version pv_... --by ada@karrygo  --reason "ready for review"
go run ./cmd/pricingctl approve  --version pv_... --by bola@karrygo --reason "verified"   # a different person
go run ./cmd/pricingctl activate --version pv_... --by bola@karrygo --reason "go live"
go run ./cmd/pricingctl simulate --version pv_... --scope TAXI --class economy --metres 8000 --seconds 1230
```

Operators must exist in `pricing_operators` with the right roles. `EDITOR` may
draft and submit; `APPROVER` may decide and activate. The two are separate so that
proposing a price change never implies approving one.

## Version 1 is deliberately price-neutral

`seeds/v1/` reproduces exactly what Taxi, Dispatch and Haulage charge today, so
switching a market onto this engine changes no customer's price. The engine's own
tests prove it against all three legacy formulas, including the awkward parts:
dispatch's express multiplier applying to the whole subtotal before rounding,
its minimum applied *after* rounding, haulage's strict 500kg boundary and absence
of any total rounding, and taxi's whole-minute flooring.

One difference is expected and bounded: the legacy code multiplies money by
float64 kilometres, so it can be a kobo out where the exact integer result is a
whole number. `TestKnownQuantizationDifferenceFromLegacyFloats` pins a worked
example. The engine is the correct one.

**The taxi rates in `seeds/v1/taxi_tariff_ng.json` are the DEVELOPMENT defaults.**
Production sets `TAXI_PROVISIONAL_RATES` explicitly; capture that value and
replace the `classes` block before approving this anywhere real.

## Status

Geography, versioned typed policy, four-eye approval, scheduled activation,
rollback, market freeze, rollout switches, the calculator and simulation are
built and tested. The V1 rate books exist as loadable seeds.

There is still **no quote endpoint**: nothing yet issues, binds or reconciles a
price tag, so no consumer can be pointed at this service. That is the next
milestone, and it is deliberately after the foundations that cannot be retrofitted
once real quotes exist — immutability, audit, idempotency, exact arithmetic and
reversible migrations.

After that: the wallet learning to execute pricing's exact split, then Taxi in
shadow mode, then market-by-market cutover. The rollout gates and the open
commercial decisions are in the implementation plan that accompanies this work.
