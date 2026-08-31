# 11 — Feature Flag and Rollout Model

## Section 8H flags

| Flag | Disabled behaviour |
|---|---|
| `VENDOR_CRM_INTELLIGENCE_ENABLED` | No intelligence route is registered — requests 404 rather than 503. The Customer 360 module reports `NOT_IMPLEMENTED`, and the segment field registry omits the eight intelligence fields entirely, so a rule cannot reference a score this deployment does not compute. |
| `VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED` | Existing scores, analytics and Customer 360 stay readable; nothing new is computed and nothing already current is disturbed. Turning the worker off makes numbers stale, never absent. |
| `VENDOR_CRM_EXPORTS_ENABLED` | No export route is registered and the generator claims nothing. |
| `VENDOR_CRM_WRITES_ENABLED` | Reads continue; rebuild requests and export commands are refused, so the whole section can be exposed read-only during a rollout. |
| `COMMERCE_GLOBAL_KILL_SWITCH` | Both workers stand down. Existing scores and completed exports remain readable. |

All three default false. Intelligence requires `VENDOR_CRM_ENABLED`; the worker requires intelligence; exports require `VENDOR_CRM_ENABLED` **and** configured Media credentials. That last one is the important prerequisite: an export with no private artifact store cannot produce a file, and the only alternative to a private store is not writing one at all — so it is a boot failure rather than a queue of jobs nobody can finish.

## Section 8G flags

| Flag | Disabled behaviour |
|---|---|
| `VENDOR_CRM_CORPORATE_ENABLED` | No corporate route is registered at all — requests 404 rather than 503, so a disabled feature has no attack surface. |
| `VENDOR_CRM_INTERACTIONS_ENABLED` / `VENDOR_CRM_TASKS_ENABLED` | The corporate activity routes that delegate to the Section 8C engine are not registered either. A route that could only ever answer "writes disabled" would be worse than not existing. |
| `VENDOR_CRM_WRITES_ENABLED` | Reads continue; every corporate command is refused, so the whole section can be exposed read-only during a rollout. |

`VENDOR_CRM_CORPORATE_ENABLED` defaults false and requires `VENDOR_CRM_ENABLED`, checked in `Config.Validate` so a half-enabled configuration is a boot failure rather than a runtime surprise: a corporate contact references a Section 8A relationship, and an account whose contacts could not be defined would be a surface with no meaning.

Section 8G adds no integration flag, because it integrates with nothing.

## Section 8E flags

`VENDOR_CRM_CAMPAIGNS_ENABLED` and `VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED` both default false. Campaigns require `VENDOR_CRM_ENABLED` **and** `VENDOR_CRM_SEGMENTATION_ENABLED`, validated at startup: the audience has no source other than the Section 8B segment engine, so campaigns without segmentation would be a surface that can define a campaign it can never populate. Preparation in turn requires campaigns.

The split mirrors Section 8B's segmentation/materialization pair and exists for the same reason: preparation is the expensive half, and an operator needs to be able to freeze it during an incident without taking the rest away. Turning preparation off leaves definitions readable and editable, previews answering, and — critically — every already-prepared snapshot intact and readable. A flag that made an existing audience look empty would be worse than the incident it was reacting to.

Commands additionally obey `VENDOR_CRM_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`; reads and previews survive a write freeze so a rollout does not blind operators.

There is no delivery flag, no notification flag and no scheduler flag, because Section 8E adds none of those capabilities. Adding one now would imply a rollout switch for behaviour that does not exist.

## Section 8C flags

`VENDOR_CRM_INTERACTIONS_ENABLED` and `VENDOR_CRM_TASKS_ENABLED` default false and require `VENDOR_CRM_ENABLED`. Disabled modules are not registered and are unavailable in Customer 360. All interaction/task commands additionally obey `VENDOR_CRM_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`; reads can remain available during a write freeze. There is no reminder-delivery or external-integration flag because Section 8C adds neither capability.

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
VENDOR_SHOP_CATALOGUE_WRITES_ENABLED
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
COMMERCE_QUOTE_LOCK_ENABLED
COMMERCE_GLOBAL_KILL_SWITCH
```

All default `false`. Every one is read, wired into `Config.Flags`, and unit-tested (`internal/config/config_test.go`).

## Validation rules enforced today (`Config.Validate`)

1. **No orphaned feature flag**: any `VENDOR_*`/`COMMERCE_*` feature flag enabled while `VENDOR_SERVICE_ENABLED=false` is a configuration error, not a valid "partially enabled" state — mirrors hauling's rule that `PrivateExecutionEnabled` requires `FleetIntegrationEnabled`.
2. **No flag without credentials**: `PAYMENT_WALLET_COMMERCE_ENABLED`, `VENDOR_VERIFICATION_INTEGRATION_ENABLED`, `DISPATCH_COMMERCE_INTEGRATION_ENABLED`, `TAXI_GOODS_DELIVERY_ENABLED`, and `HAULING_COMMERCE_INTEGRATION_ENABLED` each require their corresponding `VENDOR_<SERVICE>_URL`/`VENDOR_<SERVICE>_SECRET` pair — turning on an integration without configuring where it points to is refused at startup rather than silently no-op'ing or falling back to an insecure default, matching hauling's `Validate()` posture (no development-secret fallbacks; disabled integrations require no downstream credentials, enabled ones require explicit ones).

Section 17D additionally refuses `COMMERCE_QUOTE_LOCK_ENABLED=true` unless both
`COMMERCE_CHECKOUT_ENABLED` and `COMMERCE_FULFILMENT_PLANNING_ENABLED` are on,
and requires `COMMERCE_QUOTE_TTL_SECONDS` in `[60, 86400]`.

Section 17E refuses `COMMERCE_ORDERS_ENABLED=true` unless
`COMMERCE_QUOTE_LOCK_ENABLED` is on. An approved quote is the only thing an
order can be cut from, so without it the surface could only ever refuse. The
flag needs no credential: Section 17E creates orders, not payments.

These rules are exercised by `internal/config` tests.

## `COMMERCE_GLOBAL_KILL_SWITCH`

Reserved as the single flag that, once real routes exist, should gate every commerce-writing route behind a fast-fail middleware — matching hauling's `FleetIntegrationKillSwitch` middleware pattern (`c.AbortWithStatusJSON(503, ...)` for non-GET requests when tripped). Not wired to any middleware yet since no routes exist; the flag itself is loaded and validated now so the rollout story is fixed before the first feature needs it.

## Section 6 gating

Section 6 routes are registered behind the authenticated surface. Writes fast-fail unless `VENDOR_ONBOARDING_WRITES_ENABLED=true`; `COMMERCE_GLOBAL_KILL_SWITCH=true` also blocks them. Reads remain available for safe rollout and diagnosis. Defaults remain false and this implementation does not edit shared environment values.

## Rollout posture

Every flag defaults to disabled. A feature section is expected to: implement the feature behind its flag, verify it (tests, and where meaningful a real integration run), and only then document in [IMPLEMENTATION_STATUS.md](../IMPLEMENTATION_STATUS.md) that the flag is safe to enable in a given environment. No flag should be flipped to `true` in any shared `.env` as a side effect of scaffolding it — `.env.example` ships every flag as `false`.

## Section 8A flags

| Flag | Effect when off |
|---|---|
| `VENDOR_CRM_ENABLED` | No CRM route is registered at all — requests 404 rather than 503, so a disabled CRM has no attack surface. |
| `VENDOR_CRM_WRITES_ENABLED` | Reads continue; archive/restore return 503. Rollout can disable risky writes without blinding operators. |
| `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED` | Relationship, identity-link and order-fact ingestion rejected (503). |
| `VENDOR_CRM_CONSENT_INTEGRATION_ENABLED` | Consent ingestion rejected; every decision reads UNKNOWN and marketing is ineligible. |
| `VENDOR_CRM_PRIVACY_INTEGRATION_ENABLED` | Privacy ingestion rejected. |
| `COMMERCE_GLOBAL_KILL_SWITCH` | Disables CRM writes alongside every other commerce write. |

All default false, and all four integration flags are currently off in every environment because no service publishes those events.

**A disabled flag can only ever suppress marketing eligibility, never enable it.** `VENDOR_CRM_CONSENT_INTEGRATION_ENABLED=false` yields `CONSENT_SOURCE_UNAVAILABLE` and ineligible on every channel — the failure mode of a dead consent feed is "cannot target", never "target everyone with a cached ALLOW".

## Section 8B flags

| Flag | Effect when off |
|---|---|
| `VENDOR_CRM_SEGMENTATION_ENABLED` | No tag or segment route is registered at all — requests 404 rather than 503, so a disabled feature has no attack surface. Customer 360 omits its tag and segment blocks and reports both modules unavailable, rather than returning empty lists that would read as "this customer has none". |
| `VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED` | Definitions and previews still work; the worker claims nothing and `materialize` returns 503. No segment can newly report `CURRENT`, so a frozen audience is never presented as live. |

Both default false. `VENDOR_CRM_SEGMENTATION_ENABLED` requires `VENDOR_CRM_ENABLED`, and materialization in turn requires segmentation; both prerequisites are checked in `Config.Validate`, so a half-enabled configuration is a boot failure rather than a runtime surprise. Write commands additionally require `VENDOR_CRM_WRITES_ENABLED`, so the whole feature can be exposed read-only during a rollout.

Section 8B adds no integration flag, because it integrates with nothing.

## Section 9A flags

| Flag | Effect when off |
|---|---|
| `COMMERCE_FOOD_ENABLED` | No food catalogue route is registered at all — requests 404 rather than 503, so a disabled feature has no attack surface and does not confirm its own existence. |
| `VENDOR_FOOD_CATALOGUE_WRITES_ENABLED` | Every read still answers; every mutation returns 503 with a plain explanation. A rollout can expose the catalogue read-only without blinding operators. |
| `COMMERCE_GLOBAL_KILL_SWITCH` | Overrides the writes flag. It can only ever withhold a mutation, never permit one, which is why it is checked after the writes flag rather than combined with it. |

Both Section 9A flags default false. `VENDOR_FOOD_CATALOGUE_WRITES_ENABLED`
requires `COMMERCE_FOOD_ENABLED`, checked in `Config.Validate`, so a
half-enabled configuration is a boot failure rather than a runtime surprise.

Route registration depends only on `COMMERCE_FOOD_ENABLED` and on nothing
request-scoped, so the surface stays deterministic for the OpenAPI and route
register parity guards: the route set is a structural fact about the code, not a
function of the environment a test happens to run in.

Section 9A adds no integration flag. Its only outbound call is the existing
Media private ownership read, which is enabled by the presence of
`VENDOR_MEDIA_URL`/`VENDOR_MEDIA_SECRET` exactly as Section 7's is; with them
absent, media attachment fails closed rather than storing an unverified
reference.

Section 9B reuses `COMMERCE_FOOD_ENABLED`,
`VENDOR_FOOD_CATALOGUE_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`.
Schedules and operational reads disappear with the food surface; mutations are
withheld by the same writes/kill-switch gates. No inventory integration flag is
invented while `INVENTORY_AVAILABILITY_SUPPORT` is `NOT_PRESENT`.
