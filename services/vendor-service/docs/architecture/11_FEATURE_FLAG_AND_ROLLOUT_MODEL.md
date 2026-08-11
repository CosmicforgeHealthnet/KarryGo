# 11 — Feature Flag and Rollout Model

## Section 7 enforcement

Public onboarding mutations require `VENDOR_ONBOARDING_WRITES_ENABLED=true` and fail under `COMMERCE_GLOBAL_KILL_SWITCH=true`. External submissions additionally require `VENDOR_VERIFICATION_INTEGRATION_ENABLED=true`; validation refuses that flag without the verification URL, secret, and inbound callback allowlist secret. Wallet and Media clients require complete URL/secret pairs and fail closed when absent.

## Convention

`<PREFIX>_<FEATURE>_ENABLED` for opt-in gates, `<PREFIX>_<FEATURE>_KILL_SWITCH` for emergency shutoff of an otherwise-enabled feature — confirmed identical across `fleet-partner-service`, `driver-hauling-service`, `driver-dispatch-delivery-service`, `payment-wallet-service`, `media-file-service`, `verification-compliance-service` (see [00](00_REPOSITORY_AUDIT_BASELINE.md)). Vendor Service's flags, defined in `internal/config.FeatureFlags` and loaded in `Load()`, use exactly the names given in the implementation specification (which already match this platform convention):

```text
VENDOR_SERVICE_ENABLED
VENDOR_ONBOARDING_WRITES_ENABLED
VENDOR_CRM_ENABLED
VENDOR_INVENTORY_ENABLED
VENDOR_PURCHASING_ENABLED
VENDOR_RECIPES_ENABLED

COMMERCE_FOOD_ENABLED
COMMERCE_SHOP_ENABLED
COMMERCE_MULTI_VENDOR_ENABLED
COMMERCE_VARIABLE_WEIGHT_ENABLED
COMMERCE_BULK_RFQ_ENABLED

VENDOR_OWN_DELIVERY_ENABLED

DISPATCH_COMMERCE_INTEGRATION_ENABLED
TAXI_GOODS_DELIVERY_ENABLED
HAULING_COMMERCE_INTEGRATION_ENABLED

PAYMENT_WALLET_COMMERCE_ENABLED
VENDOR_VERIFICATION_INTEGRATION_ENABLED

COMMERCE_FULFILMENT_WORKERS_ENABLED
COMMERCE_GLOBAL_KILL_SWITCH
```

All default `false`. Every one is read, wired into `Config.Flags`, and unit-tested (`internal/config/config_test.go`).

## Validation rules enforced today (`Config.Validate`)

1. **No orphaned feature flag**: any `VENDOR_*`/`COMMERCE_*` feature flag enabled while `VENDOR_SERVICE_ENABLED=false` is a configuration error, not a valid "partially enabled" state — mirrors hauling's rule that `PrivateExecutionEnabled` requires `FleetIntegrationEnabled`.
2. **No flag without credentials**: `PAYMENT_WALLET_COMMERCE_ENABLED`, `VENDOR_VERIFICATION_INTEGRATION_ENABLED`, `DISPATCH_COMMERCE_INTEGRATION_ENABLED`, `TAXI_GOODS_DELIVERY_ENABLED`, and `HAULING_COMMERCE_INTEGRATION_ENABLED` each require their corresponding `VENDOR_<SERVICE>_URL`/`VENDOR_<SERVICE>_SECRET` pair — turning on an integration without configuring where it points to is refused at startup rather than silently no-op'ing or falling back to an insecure default, matching hauling's `Validate()` posture (no development-secret fallbacks; disabled integrations require no downstream credentials, enabled ones require explicit ones).

Both rules are exercised by `internal/config/config_test.go` and pass.

## `COMMERCE_GLOBAL_KILL_SWITCH`

Reserved as the single flag that, once real routes exist, should gate every commerce-writing route behind a fast-fail middleware — matching hauling's `FleetIntegrationKillSwitch` middleware pattern (`c.AbortWithStatusJSON(503, ...)` for non-GET requests when tripped). Not wired to any middleware yet since no routes exist; the flag itself is loaded and validated now so the rollout story is fixed before the first feature needs it.

## Section 6 gating

Section 6 routes are registered behind the authenticated surface. Writes fast-fail unless `VENDOR_ONBOARDING_WRITES_ENABLED=true`; `COMMERCE_GLOBAL_KILL_SWITCH=true` also blocks them. Reads remain available for safe rollout and diagnosis. Defaults remain false and this implementation does not edit shared environment values.

## Rollout posture

Every flag defaults to disabled. A feature section is expected to: implement the feature behind its flag, verify it (tests, and where meaningful a real integration run), and only then document in [IMPLEMENTATION_STATUS.md](../IMPLEMENTATION_STATUS.md) that the flag is safe to enable in a given environment. No flag should be flipped to `true` in any shared `.env` as a side effect of scaffolding it — `.env.example` ships every flag as `false`.
