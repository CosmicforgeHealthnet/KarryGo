# Karryam automated email branding audit

Audit completed before implementation on 8 November 2026, against staging
`78f46e94` (fast-forwarded from the local checkout before edits).

| Service / email family | Delivery route | Finding before changes |
| --- | --- | --- |
| Notification | Existing Resend-compatible SMTP; generic event and campaign renderer | Original logo already embedded in multipart/related, with quoted-printable text/plain and text/html alternatives. Only Vendor auth subjects normalized; SMTP From display name depended on environment. |
| Customer auth | shared/go/notifications -> Notification | Legacy Cosmicforge Logistics OTP subject; code and expiry text already present. |
| Vendor auth, step-up, CRM and business events | shared/go/notifications -> Notification | Legacy KarryGo Vendor OTP subject/body; step-up security copy must be preserved. CRM supplies its own campaign copy. |
| Dispatch auth and lifecycle | shared/go/notifications -> Notification at runtime | Runtime auth already Karryam. Deprecated cPanel SMTP adapter remains in code/tests with legacy OTP subject/body and bare From header. It is not wired into cmd/main.go. |
| Taxi and Hauling auth | shared/go/notifications -> Notification | Legacy KarryGo driver sign-in subjects. Codes and expiry/security instructions already present. Other event publishers use Notification. |
| Fleet auth, invitations, CRM | shared/go/notifications -> Notification | Latest staging already fixes corporate OTP title, template key and colon-separated code rendering. Invitation and CRM messages use central delivery. |
| Partner Web signup | Separate SMTPPartnerEmailSender | Plain text only; KarryGo default display name, subject and body. Supports configured bare/display-form sender address and address-shaped username fallback. |
| Payment, referral, support, verification and other event producers | Shared Notification clients / event notifications, where email requested | No additional SMTP transport or independent email provider found. Central renderer applies to all delivered event families, including campaigns and operational notifications. |

## Existing behavior to preserve

- Original landing logo and embedded logo have identical SHA-256:
  `35de10e0c2e3f4da2a82cf2a61d663c9a3901c3e0f1b854551fd3f3f412f127b`.
- Existing HTML typography, colors, dark/mobile styles and CID logo.
- Delivery-ID-derived Resend idempotency key, stable Date, Message-ID and MIME
  boundaries on queue retries. Internal `karrygo/` identifiers are contracts,
  not customer-facing brand copy, and must remain unchanged.
- SMTP TLS, bounded delivery, transient/permanent rejection handling, queue
  retry/dead-letter and duplicate suppression behavior.
- Partner SMTP final DATA acceptance remains success even if QUIT fails;
  signup challenge expiry, cooldown and invalidation logic remain unchanged.

## Implementation scope

Reuse the existing renderer and byte-identical assets in a shared Go package so
Partner Web can use the same design without another transport or service
migration. Keep Notification's adapter at its existing path. Use Karryam as the
display name while retaining the configured authorized envelope address; never
guess an address or change provider credentials. Fix email producer copy and
normalize legacy email copy at rendering time for already-persisted templates.
The old haulage/delivery/ride completion seeds also serve push/in-app; leave
their non-email behavior alone. Correct the email-only corporate seed.

## Avatar investigation and delivery limits

Google's [BIMI setup documentation](https://knowledge.workspace.google.com/admin/security/set-up-bimi)
requires a VMC or CMC for Gmail, together with domain authentication and BIMI
publication. Resend's [avatar guidance](https://resend.com/blog/email-changes-for-bimi)
distinguishes inbox avatars from in-message images. There is no SMTP field in
the existing integration that configures a recipient's Gmail avatar. Gmail
avatar completion is **not achievable or independently verified under the
no-certificate constraints**.

Resend also describes certificate-free BIMI for Yahoo/Fastmail, and free Apple
Branded Mail. These require external domain/provider verification and/or DNS
configuration, not merely this backend change. They are external follow-ups,
not configured here. Do not publish BIMI or alter DMARC/DNS, accounts or Resend.

Repository runbooks describe a dedicated approved staging account/owner inbox
but do not provide an approved concrete test recipient in this request. No
live email will be sent to an inferred personal address. Local SMTP capture
and MIME regression tests can verify generated messages, not Gmail/Outlook/
Apple Mail rendering, external authentication headers or inbox placement.
