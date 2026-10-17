# Vendor release verification

This is the authoritative checklist for the current end-to-end release work.
Earlier mock, contract and build checks do not certify deployed user journeys.

## Requested product corrections

| Item | Status | Evidence |
| --- | --- | --- |
| Remove shipped Mock mode and source switch | Implemented; focused tests passed | Production runtime always selects Live, ignores old mode/origin preferences, and test transports now live under test/support. |
| Correct sign-in errors and semantic colours | Implemented; deployment pending | Missing business access has explicit copy and a create-account action; denied access uses an error colour. |
| Nigerian phone input and normalization | Implemented; focused tests passed | Nigerian flag and +234 prefix; local and pasted international forms normalize to E.164. |
| Branch location without manual coordinates | Implemented; phone verification pending | Branch and service-area forms use map selection and optional device location. |
| Bank setup after onboarding | Implemented; migration/deployment pending | Five onboarding steps; Finance opens bank management directly; migration 130 removes the onboarding BANK requirement. Wallet payout checks remain authoritative. |
| Enable and configure deployed user journeys | In progress | VPS output confirms missing rollout flags. Public reads return 404 for scheduling, CRM, purchasing, recipes, batches and costs. |
| Production release build and signing | Pending | Production origin, signing and integrations must be verified before the store artifact. |

## Workflow coverage

Each row requires authenticated Live reads, valid commands, authoritative read-back,
permission checks and recovery after rejected or uncertain writes. A 401 proves
only that a route requires authentication; it is not a passed workflow.

| Journey | Contract operations | Current status |
| --- | ---: | --- |
| Sign-up, sign-in, resend, vendor selection, session recovery and step-up | 7 | Under review |
| Staff, roles and effective permissions | 14 | Not yet certified |
| Business, brands, branches, hours and capabilities | 35 | Under review |
| Onboarding, documents, uploads, review and activation | 21 | Under review |
| Orders, fulfilment, transitions and evidence | 25 | Not yet certified |
| Scheduled, recurring and RFQ orders | 17 | Deployment and flow review |
| Food catalogue and menu operations | 61 | Not yet certified |
| Shop catalogue, picking and substitutions | 100 | Not yet certified |
| Taxonomy | 10 | Not yet certified |
| Inventory and reservations | 27 | Not yet certified |
| Purchasing, receiving and supplier workflows | 56 | Not yet certified |
| Production materials | 20 | Not yet certified |
| Recipes | 14 | Not yet certified |
| Production batches | 13 | Not yet certified |
| Production stock policies | 2 | Not yet certified |
| Operational costs | 7 | Not yet certified |
| Production costs | 5 | Not yet certified |
| Recipe costs | 5 | Not yet certified |
| Production jobs, capacity and reservations | 54 | Not yet certified |
| Finance, bank details and withdrawals | 6 | Not yet certified |
| CRM, corporate customers, campaigns and exports | 122 | Not yet certified |
| Cloud kitchen | 32 | Not yet certified |
| Pastry | 45 | Not yet certified |
| Promotions | 7 | Not yet certified |
| Delivery missions | 24 | Not yet certified |
| Readiness | 8 | Not yet certified |
| Orchestration | 3 | Not yet certified |
| Handover | 2 | Not yet certified |
| Order support | 2 | Not yet certified |

The manifest currently contains 744 operations. Not every operation is a separate
screen. Legitimate user ownership, permissions, verification, payment and lifecycle
requirements remain enforced; implementing features does not mean fabricating
customer approvals, payment outcomes or administrative verification.

## Evidence

- `tool/audit_live_routes.py` checks representative unauthenticated GET routes.
- Runtime deployment flags must be checked separately from repository defaults.
- Database-backed integration, authenticated deployed journeys, phone layouts,
  session expiry and interrupted mutations remain required.

## Latest verification

- Six focused phone and Live runtime tests passed, including the email default and delivery refusal staying on sign-in.
- The isolated bank-profile loading regression test passed.
- The branch-to-document navigation regression test passed.
- Scoped analysis of runtime, bank loading, onboarding form and the new tests passed with no issues.
- Public route mounting report: build/release-audit/live-route-mounts.json. Missing routes are not certified workflows.
- Finance projection Go tests passed. The auth test initially hit a cache permission error; its subsequent cache rebuild was stopped without a result. No backend auth test pass is claimed.

## Continuation on 17 October

- Email is now the default authentication method. Phone number normalization is implemented, but actual SMS delivery still needs a provider account; the owner confirmed no provider has been set up.
- Bank management requests only the bank profile, without requiring all onboarding reads to succeed. A missing profile is a valid empty state; real errors show the existing error feedback dialog.
- The public mount check still finds 404 responses for CRM, scheduling, purchasing, recipes, production batches and costs. The feature-flag update is not yet confirmed on the VPS.
- Local code has not been certified against authenticated deployed journeys, and these changes are not yet a production release.

## Local evidence: scheduling, inventory and orders

- Scheduled and recurring filters now run in the backend before pagination. Eight focused Flutter tests and the Go filter/scope tests passed. The RFQ branch selector now uses the backend display_name field.
- Stock updates persist their original request and idempotency key in secure storage before sending. Reopening the app restores unresolved updates and prevents a different update until recovery. Twenty-one inventory/transfer tests passed; the final recovery recheck also passed.
- Order details reject mismatched business/order IDs and invalid versions, clear inaccessible stale data, and disable actions until valid data is loaded. Thirteen order tests passed.
- Scoped analysis of these controllers, screens and tests passed with no issues. Contract verification passed for the 744-operation manifest.
- Vendor onboarding integration tests passed against an isolated local PostgreSQL database with 130 migrations. The existing local business database was not migrated or cleared.
- These results are local evidence. Authenticated deployed journeys and external provider outcomes remain unverified.
- The main order queue still loads the latest 50 orders and filters that subset locally. Complete history filtering and pagination need correction during the ongoing review.
- VPS changes and deployment are deferred until the local work is complete, as requested.
