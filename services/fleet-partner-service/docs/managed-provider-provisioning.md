# Managed Provider Provisioning

## Resolve before provision

```text
normalize email + phone
        |
resolve in the OWNING provider service by BOTH contacts
        |
   +----+--------------------+-------------------------+
   |                         |                         |
MATCHED                  NOT_FOUND                 CONFLICT
link identity     request sponsored identity   fail atomically
                                               provider_identity_conflict
```

A provider identity is **never** created before resolving. Email and phone
resolving to different providers is a hard conflict — never an auto-merge.
An independent provider is never silently converted: they must accept the
association themselves.

Sponsored identities are created **by the owning provider service** with
`onboardingMode = ORGANIZATION_SPONSORED`, `providerMode =
ORGANIZATION_MANAGED`. This service stores only the opaque `provider_id`
with source metadata — deliberately not a foreign key.

## Consent

```text
INVITED -> PENDING_PROVIDER_ACCEPTANCE -> PENDING_SETUP -> ACTIVE
        -> DECLINED | EXPIRED | CANCELLED
ACTIVE <-> SUSPENDED ; ACTIVE|SUSPENDED -> ENDING -> REMOVED
```

The invitation email carries a link and passwordless sign-in instructions —
no password, no temporary credential. `INVITED -> ACTIVE` is rejected by the
state machine, and no corporate endpoint can accept on a provider's behalf.
See [provider-authentication-boundary.md](provider-authentication-boundary.md).

## Activation gates

Provider identity exists; association accepted; membership in
`PENDING_SETUP`; home branch assigned; organization `ACTIVE`; sub-wallet
confirmed `ACTIVE`; provider platform status `ACTIVE` (licence, vehicle and
document compliance still apply). A partially provisioned provider is never
treated as active.

## One settlement organization

A partial unique index on `(provider_service, provider_id)` over live
statuses enforces one live membership per provider per service category.
A provider cannot hold independent and organization-managed marketplace
settlement for the same category at the same time.

## Sub-wallets

`payment-wallet-service` creates them; this service stores
`wallet_account_id`, status and source metadata only. No balance, ever, and
customer trip settlement is never credited here directly. Creation is
idempotent and unknown outcomes are reconciled by read-back.

## Provisioning saga

`fleet_partner_provider_provisioning_sagas` tracks resolution, identity,
association, sub-wallet and notification steps with an overall status.
The reconciliation worker re-drives incomplete sagas and resolves unknown
outcomes by **reading back** rather than repeating writes — a second
provisioning call could otherwise create a duplicate provider identity.

## Offboarding

Removal sets `REMOVED`, revokes organization matching eligibility in the
owning service, and **retains the provider identity**. The settlement slot
frees up so the provider can work independently or join another organization.

## Data exposure

Provider-facing and organization-facing DTOs never carry customer fare,
company settlement, commission, margin or contract price — none of those
values exist in this service.
