# Provider Authentication Boundary

```text
Managed-provider authentication remains in the owning provider service.
No temporary provider password is sent.
```

## Who owns provider identity

| Service category | Owning service | Owns |
| --- | --- | --- |
| `DISPATCH` | `driver-dispatch-delivery-service` | provider ID, authentication, platform status, verification |
| `TAXI` | `driver-taxi-service` | same |
| `HAULING` | `driver-hauling-service` | same |

`fleet-partner-service` owns only the **organization's relationship** with a
provider: membership, invitation/consent state, organization-controlled
capabilities and policies, history, provisioning saga and a sub-wallet
identity projection. It never creates a competing provider identity and never
issues a provider credential.

## The provider services are already passwordless

The audit (`docs/auth-onboarding-iam-provider-audit.md` §1.4) verified this
directly. All three services already expose `/api/v1/auth/signup/start`,
`/login/start`, `/start`, `/verify`, `/refresh`, `/logout`, and their start
DTOs already carry **both** `phone_number` and `email`; verification is
`identifier` + `otp_code`. `driver-hauling-service` already uses the shared
`HashOTP` keyed digest.

A repository-wide grep for password fields in provider authentication returns
**only SMTP infrastructure credentials** (`clients/email_client.go`,
`smtp.PlainAuth`), which the specification explicitly permits.

**Therefore no provider-service source file was modified by this work
package.** The specification anticipated a possible "smallest
backward-compatible change" to bring provider auth to the passwordless
standard; that change was unnecessary because the standard was already met.

## How a managed provider signs in

1. The company creates or links the provider (see
   [managed-provider-provisioning.md](managed-provider-provisioning.md)).
2. The provider receives an **association invitation email** — a link and
   instructions, never a password or temporary credential. The
   `sign_in_method` field in that template is literally
   `email_and_phone_otp`.
3. The provider opens the provider app or approved provider web flow and
   authenticates **in the owning provider service** with email + phone +
   emailed OTP.
4. They review the organization association and accept or decline it
   themselves.
5. They continue to required setup without repeating independent-provider
   onboarding.

`POST /providers/{membershipId}/resend-login` means *resend secure
passwordless access instructions*. It re-sends the invitation email. It does
not send, generate or reset a password. A clearer alias
(`.../resend-access`) may be added later; the existing path stays until a
documented migration retires it.

## What organization sponsorship does not bypass

Sponsorship affects onboarding convenience only. It never bypasses platform
safety:

- required licence status may still block work;
- required vehicle status may still block work;
- expired documents may still block work;
- compliance restrictions may still block work;
- the provider's platform status is read from the owning service and is an
  activation gate (`provider_platform_status_active`).

## Consent is not delegable

Membership cannot reach `ACTIVE` without the provider's own acceptance:

```text
INVITED -> PENDING_PROVIDER_ACCEPTANCE -> PENDING_SETUP -> ACTIVE
```

`INVITED -> ACTIVE` is rejected by the state machine, and
`RecordAssociationOutcome` is driven by the owning provider service (event or
internal callback) — there is no corporate endpoint that accepts on a
provider's behalf. `TestProviderStateMachineRejectsIllegalTransitions` proves
the skip is impossible.

## Decline is permanent

`provider_decline_required` is `true` in the model, written as `true` by the
repository, and constrained to `true` by the database:

```sql
provider_decline_required boolean NOT NULL DEFAULT true
    CHECK (provider_decline_required = true)
```

`TestProviderDeclineRightCannotBeDisabled` asserts that a direct SQL attempt
to set it `false` is rejected by PostgreSQL itself.

## Identity survives offboarding

Removing a membership sets `REMOVED` and revokes organization matching
eligibility in the owning service. The `provider_id` reference is retained,
the provider's platform account is untouched, and the settlement slot frees
up so the provider can work independently or join another organization.

## Contract status

The resolve / sponsored-provision / association endpoints do **not** exist in
the provider services yet (audit §1.5). They are implemented here as typed
interfaces with explicitly unconfigured adapters that return
`ErrNotConfigured`, plus a reconciliation worker. No endpoint path is
fabricated as live, and no provider database is read as a fallback. See
[service-contracts.md](service-contracts.md).
