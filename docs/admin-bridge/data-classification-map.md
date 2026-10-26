# Karryam Admin data classification and HQ disclosure map (Section 30)

Classes: `PUBLIC`, `INTERNAL`, `OPERATIONS_MINIMIZED`, `RESTRICTED_PII`, `FINANCIAL_SENSITIVE`,
`HEALTH/SPECIAL_CATEGORY_IF_INTEGRATED`, `SAFETY_EVIDENCE`, `CREDENTIAL`.

Enforcement: the event adapter registry (`shared/go/adminevents/adapters.go`) refuses to register
any export carrying `RESTRICTED_PII`, `CREDENTIAL` or `HEALTH…`; adapters have no free-text field
kind; owner resource definitions are compiled allowlists. KarryGo holds no HealthNet medical data
and exports none.

| Data | Owner | Class | Reaches HQ? | How |
|---|---|---|---|---|
| Support case status, priority, category, counts, assignee ref | support-dispute-service | OPERATIONS_MINIMIZED | Yes | Event feed / support queue |
| Support subject preview | support-dispute-service | OPERATIONS_MINIMIZED (redacted ≤240) | On-demand only | Owner resource label, context-scoped, audited |
| Chat message bodies | support-dispute-service | RESTRICTED_PII | No (preview label only, redacted) | — |
| Complainant name/phone snapshot, incident lat/lng | support-dispute-service | RESTRICTED_PII / SAFETY_EVIDENCE | No | — |
| SOS case existence, priority, age | support-dispute-service | SAFETY_EVIDENCE (minimized) | Yes | Event feed, metrics |
| Verification case status, dimension, recommendation, reason code | verification-compliance-service | OPERATIONS_MINIMIZED | Yes | Event feed / owner resources |
| Document refs, NIN, bank data, identity templates | verification / wallet / media | RESTRICTED_PII / FINANCIAL_SENSITIVE | No | — |
| Campaign state and counts | notification-service | INTERNAL | Yes | Event feed / campaign read-back |
| Recipient ids, emails, device tokens, message bodies | notification / customer | RESTRICTED_PII / CREDENTIAL | No | — |
| Marketing consent status per channel | customer-service | RESTRICTED_PII | Aggregate dimension only | Owner resource dimension |
| Finance aggregates per currency | payment-wallet-service | FINANCIAL_SENSITIVE (aggregate) | Yes | Finance report (no per-customer/provider rows) |
| Ledger rows, bank coordinates, provider tokens | payment-wallet-service | FINANCIAL_SENSITIVE / CREDENTIAL | No | — |
| Evidence bytes | media-file-service | SAFETY_EVIDENCE / RESTRICTED_PII | Only via single-use, staff-bound, case-bound grant | Media case grants |
| Legacy media mapping | media-file-service | INTERNAL | Opaque ids + digest | `hq.media.legacy-mapping` |
| Privacy request subject | Bridge | RESTRICTED_PII → stored as SHA-256 digest only | No raw value | Privacy register |
| Support-queue export | Bridge | OPERATIONS_MINIMIZED | Approved, AES-256-GCM at rest, single-use 15-min download | `hq.exports.*` |

## Caller → owner permission matrix (private service calls)

| Caller (HMAC name) | Callee | Route family | Secret variable (Bridge side / owner side) |
|---|---|---|---|
| admin-backoffice-service | support-dispute-service | admin resources, commands, event snapshot | `ADMIN_SUPPORT_DISPUTE_*_SECRET_FILE` / `SUPPORT_DISPUTE_ADMIN_*_SECRET_FILE` |
| admin-backoffice-service | verification-compliance-service | admin resources, commands, event snapshot | `ADMIN_VERIFICATION_COMPLIANCE_*` / `VERIFICATION_COMPLIANCE_ADMIN_*` |
| admin-backoffice-service | notification-service | campaigns, event snapshot | `ADMIN_NOTIFICATION_CAMPAIGN_SECRET_FILE`, `ADMIN_NOTIFICATION_EVENT_SNAPSHOT_SECRET_FILE` / `NOTIFICATION_CAMPAIGN_SECRET_FILE`, `NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE` |
| admin-backoffice-service | payment-wallet-service | finance reports | `ADMIN_FINANCE_REPORT_SECRET_FILE` / `PAYMENT_WALLET_ADMIN_FINANCE_REPORT_SECRET_FILE` |
| admin-backoffice-service | media-file-service | case grants, legacy mapping | `ADMIN_MEDIA_CASE_SECRET_FILE` / `MEDIA_ADMIN_CASE_SECRET_FILE` |
| support / verification / notification | admin-backoffice-service (event ingress) | `POST /internal/karryam-admin-events/v1/events` | `<PREFIX>_ADMIN_EVENTS_SECRET_FILE` / `ADMIN_EVENT_SOURCE_SECRETS_FILE` |
| notification-service | customer-service | marketing audience + consent check | `NOTIFICATION_CUSTOMER_AUDIENCE_SECRET_FILE` / `CUSTOMER_SERVICE_SECRETS` |

Every secret is per-audience; the Bridge refuses to start if any two loaded audience keys are equal
or equal an HQ workload key.

## Retention

No retention period is invented. Bridge evidence tables are append-only; projections hold no
subject identifiers; exports expire after 15 minutes and are single-delivery. Owner retention and
erasure for holders other than customer-service remain `POLICY_REQUIRED` in the privacy register.
