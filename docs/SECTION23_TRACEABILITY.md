# Payment & Wallet Extension Traceability

Roadmap numbering is retained only in this certification document. Production
source and OpenAPI filenames are capability/domain named.

| Requirement | Authority / implementation | Evidence |
|---|---|---|
| Sole money ledger | payment-wallet-service double-entry ledger | `wallets/repositories/commerce.go` |
| One customer payment | Commerce Payment Intent bound to ParentOrder/quote | Wallet migration `000010`; Vendor `orders/payment_service.go` |
| Immutable multi-vendor allocation | DB checks + immutable snapshot triggers | migration `000010` |
| Final/max authorization | final and maximum stored separately; maximum held | Wallet commerce intent and variable finalization |
| Vendor/platform/provider separation | distinct payable accounts and release operations | `commerce.go` release paths |
| Sponsor attribution | Vendor/Platform/other funding fields and separate funding accounts | allocation model and ledger hold/refund |
| Partial refund isolation | VendorOrder + item identity + snapshot fingerprint + unique operation | commerce refund repository/API |
| Vendor payable/withdrawal | Vendor owner type, atomic account locking, existing provider withdrawal state machine | Wallet repository/usecase |
| Vendor security | session-derived Vendor, existing IAM, step-up and command idempotency | `finance_projection` |
| Recovery/read-back | intent, operation, withdrawal authoritative GET routes | Wallet and shared client contracts |
| Audit/outbox atomicity | evidence appended inside the same pgx transaction | `recordFinancialEvidence` |
| Frontend handoff | no frontend implementation; safe minor-unit projections only | Vendor `FRONTEND_REQUIREMENTS.md` |

PostgreSQL migration replay and database concurrency/isolation certification
must run with `PAYMENT_WALLET_TEST_DATABASE_URL` and
`VENDOR_SERVICE_TEST_DATABASE_URL` pointing to disposable test databases.
