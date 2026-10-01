# Version 1 policy bodies

These bodies reproduce **exactly** what Taxi, Dispatch and Haulage charge today,
so that moving a market onto the central engine changes no customer's price. They
are the migration source, not a commercial recommendation.

They are loaded as **drafts** and must be submitted and approved like any other
change — a migration never inserts a live rate, because every rate in production
must have an approver's name against it.

```bash
go run ./cmd/pricingctl import-draft --file seeds/v1/taxi_tariff_ng.json --by ada@karrygo --reason "V1 price-neutral baseline"
go run ./cmd/pricingctl submit  --version <id> --by ada@karrygo
go run ./cmd/pricingctl approve --version <id> --by bola@karrygo   # must be a different person
go run ./cmd/pricingctl activate --version <id> --by bola@karrygo --from now
```

## A warning about the taxi rates

`taxi_tariff_ng.json` currently holds the **development defaults** from
`driver-taxi-service/internal/config/config.go`. Production sets
`TAXI_PROVISIONAL_RATES` explicitly, and that value is what V1 must carry.
Capture production's value and replace the `classes` block before this is
approved anywhere that matters, or the first "price-neutral" cutover will move
every taxi fare.

## Why each odd-looking setting exists

The three services round differently, and V1 must match each one:

| Setting | Why |
|---|---|
| taxi `distance_rounding: TRUNCATE` | the legacy provider truncates `per_km * km` |
| taxi `duration_unit: WHOLE_MINUTES_FLOOR` | 30 extra seconds buys no further minute |
| taxi `total_rounding: 50 HALF_UP`, then the class minimum | matches the legacy order |
| dispatch `express_applied_to: SUBTOTAL_BEFORE_ROUNDING` | 1.35 multiplies the base too, before rounding |
| dispatch `minimum_after_rounding: true` | the legacy minimum is applied after the 50-kobo step |
| dispatch `distance_basis_by_channel` | the legacy surfaces genuinely disagree: road route for serviceability, straight line for the booking estimate, commerce and partner API |
| haulage `total_rounding.step_kobo: 0` and `minimum_kobo: 0` | haulage uniquely does not round its total and has no minimum |
| haulage `weight_surcharge.rounding: TRUNCATE` | the legacy code used integer division |
| haulage `threshold_grams_exclusive: 500000` | exactly 500kg is **not** surcharged |
| haulage `distance_basis: STRAIGHT_LINE` | the legacy formula never used road distance; changing that is a price change and needs its own approved version |
| commission `DISPATCH/COMMERCE: 0` | commerce deliveries pay the rider the whole delivery fee today; a flat 15% would cut rider pay |
