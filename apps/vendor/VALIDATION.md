# Validation

## Premium UI polish - 9 October 2026

- Focused `test/premium_polish_test.dart`: all three checks passed for narrow-phone stock figures with 1.4 text scaling, archive menu interaction and accessible skeleton/success feedback.
- Page-entry reads use responsive skeleton cards across catalogue, inventory, settings and finance; dashboard placeholders use the same component. Compact refresh and action spinners remain, and determinate order-picking progress remains a progress bar.
- Shared details align left across the available width. Stock figures use an adaptive icon/card layout with quantities that scale down rather than split across lines.
- Archive inclusion remains the same backend query, accessed through a discreet filter menu with an active indicator.
- Shared success feedback uses a custom animated drawn check with a teal seal; loading and success animation honor reduced-motion settings.
- No backend field, route, workflow, Mock/Live behavior or dependency changed. The full suite was not rerun for this visual increment, as requested.

## Stock tracking empty-selector correction - 9 October 2026

- `flutter test --no-pub test/inventory_test.dart --reporter compact`: all 13 inventory tests passed, including the new already-tracked listing -> direct receiving regression.
- Empty stock-tracking choices explain eligibility, disable new tracking, hide unused configuration controls and offer the actual receipt flow for existing stock. No tracked listing is made eligible for an unsupported reconfiguration.
- This targeted correction follows the prior 171-test full regression; the full suite was not rerun for this local UI change.

## Reorder-rule validation - 9 October 2026

- `flutter test --no-pub --reporter compact`: all 171 tests passed, including eight reorder checks and the complete existing app regression suite.
- `flutter analyze --no-pub`: passed, no issues found.
- `python tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules. Exact reorder fields, nullable expiry hours, policy permission and branch-list pagination match Shop source; expiry limits also match the database constraint.
- Checks cover rule creation/upsert, stable identity and incremented versions, explicit null to clear expiry hours, zero minimum, invalid target/expiry rejection, unchanged stock/balance revisions and movement history, recommendation changes after quarantine/receiving, expired-lot exclusion, branch isolation and repeat-safe command replay.
- Live transport failures remain errors; uncertain retries preserve the exact payload, nullable field and idempotency key. Invalid or mismatched returned rules are not accepted as saved.
- Widget checks at 360 x 800 cover stock detail -> prefilled Mock sheet -> save -> success feedback -> refreshed server recommendation, plus 1.4 text scaling and a 300 px keyboard inset with target validation.
- Mock remains default; new Live rule fields stay empty, and existing values come from the backend. No new dependency, backend route, physical-device install or deployed-server mutation was added. Saving rules neither purchases nor changes stock. Alert generation remains a backend worker concern.

## Shop quarantine validation - 9 October 2026

- `flutter test --no-pub --reporter compact`: all 163 tests passed, including ten quarantine checks and the complete existing app regression suite.
- `flutter analyze --no-pub`: passed, no issues found.
- `python tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules. Quarantine creation/resolve fields, Shop permission metadata and branch list pagination match actual handler source.
- Checks cover stock-neutral on-hand holds, reduced sellable quantities, controlled reasons/quantities, branch isolation, complete-case release/disposal, terminal-state conflicts, same-action repeats and exact command-key replay. Quarantined lots remain protected from consuming adjustments and receipt reversal; expired units cannot be held or become sellable through release.
- Transport checks preserve Live permission/network errors and the identical action/key on uncertain retries. Returned cases are validated before success and against the selected branch.
- Phone widgets at 360 x 800 cover creation -> review sheet -> success -> case detail -> release, cancelled disposal confirmation, over-quantity validation and 1.4 text scaling with a 300 px keyboard inset. Only the action being submitted shows its loading indicator.
- Mock remains the default with prefilled creation values. Live uses existing Shop routes with no Mock fallback. No backend route, dependency, physical-device installation or deployed-server stock mutation was added.

## Branch transfers validation - 9 October 2026

- `flutter test --no-pub --reporter compact`: all 153 tests passed, including seven transfer checks and the complete existing regression suite.
- `flutter analyze --no-pub`: passed, no issues found.
- `python tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules, with exact transfer creation/arrival fields and transfer list pagination checked against backend source.
- Checks cover stock-neutral drafts, visibility at both branches, draft cancellation, repeat-safe dispatch, current-stock validation, incremental partial arrivals, damaged/missing accounting, over-receipt rejection and exact command-key replay.
- Transport checks verify permission/network failures remain errors and uncertain arrival retries preserve the original payload and command key.
- Widget checks at 360 x 800 cover transfer creation, review, success feedback, full-screen detail and partial arrival validation. Loading failures offer retry; discrepancy badges use amber.
- Mock remains the default. Live uses existing Shop endpoints with no Mock fallback. No deployed-server stock mutation or physical-device installation was performed.

## Shop inventory validation - 9 October 2026

- `flutter test --no-pub --reporter compact`: all 146 tests passed, including 12 new inventory tests and the existing app regression suite.
- `flutter analyze --no-pub`: passed, no issues found.
- `python tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules. Critical checks include source-exact receipt/line and adjustment fields, list pagination parameters and command headers.
- Workflow checks cover draft receipts leaving stock unchanged, repeat-safe posting, branch isolation, independent balance revisions, insufficient-stock rejection, complete stocktake counts including zero, stale snapshots, alert state changes, exact command-key replay, intact-lot reversal and refusal after consumption, expired lot availability and approximate-measure rejection.
- A server-origin transport probe verifies failures do not produce Mock stock and uncertain adjustment retries preserve the same body and idempotency key.
- Phone checks at 360 x 800 cover stock -> full-screen detail, existing-stock receiving selectors and prefilled Mock values, and draft save -> success feedback -> receipt review without posting stock.
- Stock detail and creation forms keep their main actions accessible at the bottom. Small counts and confirmations use sheets; complex receipt/count review uses full screens. Inventory empty states use purpose-specific icons.
- Mock remains the default; Live uses the existing Shop inventory routes. No backend route, package dependency, physical-device installation or real stock movement was performed. These are local contract, Mock and transport checks, not certification of the deployed server.

Implemented flows and remaining operational surfaces are documented in [docs/inventory.md](docs/inventory.md).

## Catalogue read-tools validation - 9 October 2026

- `flutter analyze --no-pub`: passed, no issues found.
- `flutter test --no-pub --reporter compact`: all 134 tests passed, including 10 new read-tool checks and the existing app regression suite.
- `python tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules. Critical checks now assert food preview's exact selection fields, read permission and absence of an Idempotency-Key.
- Contract checks cover variant-over-branch price precedence, attached modifier limits and detach-after-selection rejection, independent entity versions, listing-unit overrides, wholesale quantity-band thresholds, channel rejection, nullable unpriced values, order rules, case-insensitive typed identifier lookup and identifier archiving.
- Transport checks confirm the food preview POST sends no command header or caller-supplied prices, server failures never yield Mock prices, and generic missing routes are errors rather than lookup misses.
- Phone checks at 360 x 800 cover food choice -> returned breakdown, Shop quantity changes invalidating the old result, and lookup -> actual product detail, including 1.4 text scaling and a 300 px keyboard inset.
- Mock remains the default. Live uses existing public reads. The generated food-preview metadata and generator were corrected against the actual read-only handler; no backend route or package dependency was added. No physical-device install, real checkout or funds movement was performed.

Navigation, behaviors and remaining media/inventory work are documented in [docs/catalogue.md](docs/catalogue.md).

## Advanced catalogue validation - 9 October 2026

- `flutter analyze --no-pub`: passed, no issues found.
- `flutter test --no-pub --reporter compact`: all 124 tests passed, including 10 new advanced catalogue checks and the existing app regression suite.
- `python apps/vendor/tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules, with source fields and critical flows validated.
- New checks cover independent preparation/availability versions, profile inheritance, overnight schedules in the branch timezone, clearing schedule dates, explicit branch sell-unit selection, enabled-unit quantity bands, platform measure protection, owned aliases, class-specific fresh choices and restoring archived attributes through upsert.
- Every visible advanced action is checked against the generated backend operation catalogue. Phone checks at 360 x 800 cover native schedule editing, preparation sheet save -> success feedback, fixed fresh-attribute types, 1.4 text scaling and a 300 px keyboard inset.
- Mock remains the default; configuration changes share the existing catalogue records. Live uses only existing backend routes and actual related-record selectors, with no Mock fallback. No package dependency, backend route, physical-device installation or real business mutation was added in this increment.
- Inventory management, dedicated price-preview/identifier-lookup UI and authorized catalogue-media upload remain separate work. See [docs/catalogue.md](docs/catalogue.md).

## Business settings validation - 9 October 2026

- `flutter analyze --no-pub`: passed, no issues found.
- `flutter test --no-pub --reporter compact`: all 114 tests passed, including 19 new settings checks and the existing authentication, onboarding, dashboard, catalogue, orders and finance suite.
- `python apps/vendor/tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules, with source fields and critical flows validated.
- Settings checks cover complete profile replacement, stale versions, business pause/resume, Draft branches and capability links, opaque brand media references, hours using the parent branch version, independent operations-settings versions, staff assignment/removal and suspend/reactivate, system-role protection and purpose-bound single-use high-risk grants.
- Transport checks confirm that IAM writes have no invented Idempotency-Key, an accepted-but-lost response blocks automatic replay, and write authentication failures do not refresh/retry the operation.
- Widget checks cover 360 x 800 screens, larger text, a 300 px keyboard inset, the complete interval sheet -> save -> success feedback flow, and reopening the workspace for an activated PAUSED business.
- Mock remains the default and shares settings changes with onboarding/dashboard records. No package dependency, new backend route, APK build, physical-device installation or real staff/business mutation was performed for this increment. Live deployment and actual OTP delivery still require environment validation.

Implemented flows and public backend limitations are documented in [docs/settings.md](docs/settings.md).

## Finance validation ? 8 October 2026

- `flutter analyze --no-pub`: passed, no issues found.
- `flutter test --no-pub --reporter compact`: all 95 tests passed, including 14 Finance checks and the existing auth, onboarding, dashboard, catalogue and order checks.
- `python apps/vendor/tool/verify_contracts.py`: passed; 724 public Vendor operations across 29 modules, with source fields and critical flows validated.
- Finance phone checks cover a 360 ? 800 withdrawal flow from the tab through review, session verification, request feedback and the canonical transfer-detail screen, plus 1.5? text scaling.
- Safety checks cover response loss after reservation, restoring and replaying the original command without a duplicate debit, wrong-purpose/rotated-session grants, server minimum, idempotency body conflicts, no automatic financial refresh, secure-journal failure handling, independent resource permissions and transfer-provider verification requirements.
- No package download, new APK build, physical-device installation or real payout was performed for this increment. Live transport/provider deployment and actual OTP delivery still require environment validation.

Implemented Finance flows and the two missing public backend recovery/verification endpoints are documented in [docs/finance.md](docs/finance.md). This section supersedes the earlier statement excluding dashboard and money UI from scope.

## Frontend review and order support - 9 October 2026

- Reviewed screen-level coverage against 724 public operations in 29 modules. The remaining API-only workspaces and backend read gaps are tracked in [docs/frontend-coverage.md](docs/frontend-coverage.md); this does not certify a complete frontend.
- Added the two public order-support flows with exact string categories, actual line references, required idempotency, secure Live recovery and isolated in-memory Mock recovery.
- Six support checks passed, including response loss/replay, read reconciliation, failed storage, independent permissions and the complete dark-phone review/submission/detail journey. The final focused run also passed the existing five-tab/branch/order navigation check. No full regression suite was rerun.
- `flutter analyze --no-pub`: passed, no issues. Contract verification passed for 724 operations, including the corrected support category schema.
- Actual deployed Live servers, provider dependencies, real OTP delivery and physical-device installation were not exercised. Live request behavior was checked with controlled HTTP transport.

## Appearance validation - 9 October 2026

- `flutter analyze --no-pub`: passed, no issues found.
- Four focused appearance checks passed: saved preference and write-failure recovery, System versus explicit mode, switching an open appearance sheet, and dark stock detail surfaces and readable status colors at phone width.
- No full regression suite, new dependency, APK build or physical-device installation was performed for this increment.
- Appearance does not change backend contracts or Mock/Live behavior.

Dark-mode polish corrects surface-versus-text color roles in filter chips, popup menus, catalogue tiles and finance links. Custom illustrations receive theme colors and repaint when those colors change. Success toasts use an explicit teal-tinted surface; feedback dialogs fade and scale with reduced-motion support. The color audit's temporary scripts were removed, without adding a package dependency.

- Appearance and premium-polish checks passed (seven checks); both new focused dark-polish checks passed after correcting the success surface's blue cast.
- Final static analysis passed with no issues. The full regression suite was not rerun.

The subsequent neutral-surface correction replaces broad tinted dark fills with slate, including selected filters and success toast surfaces. Catalogue screen backgrounds use the charcoal canvas. The sign-in method group uses neutral layers; Mock contact defaults are prefilled while Live receives no dummy identifiers. This supersedes the teal-tinted success surface described above.

## Earlier validation

7 October 2026.

- `flutter pub get --offline`: passed with locally cached dependencies.
- `flutter analyze --no-pub`: passed, no issues found.
- `flutter test --no-pub`: 31 tests passed (17 backend integration checks and 14 mobile/runtime checks).
- `flutter build web --release --no-pub`: passed, JavaScript release build.
- Installed Chrome, 393 × 852 viewport: actual Flutter onboarding → phone sign-in → OTP → vendor selection → signed-in flow passed, with no browser JavaScript errors. Eight refreshed screenshots are saved in `design/implemented/`, with additional first-onboarding and sign-in captures at 360 ? 800. The final redesigned app completed the full mock flow in Chrome without JavaScript errors.
- Widget checks at 360 × 800 cover onboarding, sign-in, OTP, a 300 px keyboard inset and vendor selection without layout exceptions.
- In-app mock/live controls tested for invalid origin handling and successful switching in both directions. Installed Chrome also confirmed switching from Mock to Live with a configured origin.
- Mock mode never calls the live transport; mock sessions never enter live secure storage. Live sessions are isolated by server origin, and restoring a live session calls token refresh before the signed-in screen appears.
- Code expiry, replacement challenge on resend, wrong codes, conditional vendor selection, single-vendor login, access errors, connection recovery and backend Retry-After waits are covered.
- Native Android/iOS and web icons/launch assets use the existing official logo, with the original symbol geometry preserved.

Live HTTP checks inject a controlled HTTP server response using MockClient and assert the exact origin, routes and payload fields. No deployed backend credentials or real OTP delivery were exercised. Dashboard, money, stock, custody, upload, realtime transport and deployed feature enablement remain outside this UI scope. iOS requires a macOS/Xcode build and has not been compiled on this Windows machine.

Before this visual redesign, the Android ARM64 debug build passed with Gradle 8.14 / AGP 8.11.1 / Kotlin 2.2.20 using cached dependencies. The installable APK is `build/app/outputs/flutter-apk/app-debug.apk`; its Flutter engine is ARM64. No physical-device run was performed. The saved APK predates the visual redesign; run Flutter from source to install the current UI. A universal build requires the missing ARMv7 and x86_64 Flutter engine artifacts.

The redesign keeps all auth/runtime contracts unchanged. Primary actions use teal throughout, the onboarding canvas remains consistent, and custom editorial images replace the older branded bag photography. Built-in image generation prompts and saved asset locations are in `design/ASSETS.md`. The final web release and 14 mobile/runtime tests were rerun after adding those assets; analysis remains clean.

The existing contract coverage remains 722 public Vendor operations in 29 feature areas: 711 snapshot routes plus 11 newer source routes, with critical request fields checked.


## Preparation and vendor delivery ? 9 October 2026

`flutter analyze --no-pub`: no issues. `flutter test --no-pub test/order_readiness_test.dart test/vendor_missions_test.dart`: six checks passed. Coverage includes active branch types, readiness-only versions/history, ready corrections, stale concurrent writes, protected rider transitions, required failure reasons, and separate responsive detail/form screens at 360 ? 800 in dark mode. This was a targeted increment check, not a full-suite or deployed Live certification.

## Scheduled management ? 10 October 2026

- Focused scheduling Flutter suite: 4 passed, including pagination/cancellation, version conflicts/preserved metadata and small-phone dark layout.
- Scheduling read authorization and embedded-input Go checks passed; production route count/documentation checks passed after documenting the two existing onboarding upload routes alongside the four new schedule reads.
- Contract verification passed: 728 generated Vendor operations, 29 modules, no unresolved top-level requests. Untagged embedded inputs retain actual exported Go JSON names.
- Scoped reads and management screens are implemented; approved/customer/corporate/RFQ creation contexts remain pending. Deployed Live and database-backed scheduling integration were not certified.

## Promotions — 10 October 2026

- App-wide Dart analysis: no issues.
- Five focused Flutter checks: duplicate-code rejection, durable identical coupon retry after reopening, version conflicts, quantity-tier create/edit and 360×800 dark-mode list/detail/editor layout.
- Backend Promotions package checks passed, including branch access boundaries and duplicate-code error translation.
- All seven existing public operations have workspace flows. This is not deployed Live, checkout/redemption or database-backed integration certification; see docs/promotions.md.

## Advanced delivery policy — 10 October 2026

- Changed-file Dart analysis: no issues.
- Six focused advanced/policy/fallback checks passed: immutable identical retry after reopening, advanced rule persistence, invalid-tier refusal before submission, explicit clearing, Mock rule sheets and 360×800 dark layout, alongside existing stale-policy and fallback recovery checks.
- Existing policy GET/PUT contracts are used without backend edits. Zone references, IANA timezone validation, service-area enforcement, funding and dispatch remain server responsibilities; deployed Live integration is not certified.

## Delivery decisions - 10 October 2026

- Changed-file Dart analysis: no issues.
- Four focused Flutter checks passed: lost-response audit recovery without duplicate POST, execution/version refusal, missing-plan state and 360x800 dark screen/override sheet.
- Focused backend vendor/branch scope and execution/terminal guard tests passed.
- Live requires the updated backend and enabled orchestration; deployed and database-backed integration remain unverified.

## Catalogue taxonomy - 10 October 2026

- Changed-file Dart analysis: no issues.
- Four focused Flutter checks passed: immutable identical retry after reopening, isolated facet clearing and primary validation, stale review/source refusal, and 360x800 dark classification/category editor layout.
- Existing public vendor routes are used; backend unchanged. Tag/claim assignments are read-only. Deployed Live and database-backed integration remain unverified.

## Purchasing - 10 October 2026

- Changed-file Dart analysis: no issues.
- Five focused Flutter checks passed: durable supplier retry/version conflict, receipt stock posting with exact replay, self-approval and inspection safeguards, read-only alert recovery, and a 360x800 dark hub/order-line editor.
- Focused backend TestSection15C_ checks passed.
- Approval policy reads gain rules_complete to prevent replacing hidden rules. Deploy the accompanying backend change for Live policy editing. Deployed Live and database-backed integration remain unverified.


## Ingredients and packaging - 10 October 2026

- Changed-file Dart analysis: no issues.
- No new test suite or broad regression run; final regression is deferred as requested.
- Existing production-master handlers/contracts used without backend changes. Deployed Live, production dependency guards and complete integration remain unverified.


## Conversions and branch mapping - 10 October 2026

- Changed-file Dart analysis: no issues.
- Full tests deferred as requested; no broad regression or deployed Live run performed.
- Existing production-master contracts used. Conversion updates use If-Match; branch configuration has no contractual version precondition. Mock references enforce ingredient/packaging branch mapping.


## Recipes and recipe costs - 10 October 2026

- Changed-file Dart analysis: no issues.
- One focused public-contract smoke check passed: draft validation/activation, incomplete cost preview, immutable snapshot creation, retirement and archive.
- Bodyless lifecycle and snapshot requests are correctly omitted; production-material archive uses the same correction.
- Broad tests deferred as requested. Actual Live, specialty catalogue access, database-backed arithmetic and concurrency remain unverified.


## Production stock policies and batches - 10 October 2026

- Changed-file Dart analysis: no issues.
- One focused stock-safety check passed: initial policy If-Match zero, active-recipe batch creation, plan/start, accepted completion with lost response, reopening/replay and exactly one prepared inventory posting.
- Broad tests deferred as requested. Deployed stock transactions, packaging policies, access and production-job integrations remain unverified.


## Production operations first stage - 10 October 2026

Focused command-recovery smoke passed: lost transition response, restored original key, exactly one queue history event and versioned station transition. Focused static analysis covers the production workspace and Mock wiring. Broad final tests and deployed Live verification remain pending.


## Production capacity and handoff - 10 October 2026

Focused production checks passed: lost partial-release response restored/replayed without double-release; remaining held quantity is correct; required steps block READY; ready evidence and completed job actions succeed through public contracts. Existing queue recovery smoke also passes. Broad final and deployed Live checks remain pending. ReleaseInput JSON tags and generated/OpenAPI release contract were aligned with the existing backend version/full/partial-release behavior.

Focused production analyzer completed without errors; the final redundant non-null assertion was removed. Local request validation happens before recovery journaling, so invalid form data cannot create a false uncertain-outcome lock.

Production and operational costs: focused cost_workspace_test.dart and analysis cover policy valuation, frozen batch evidence, retry/reopening, immutable snapshots, comparison and stale versions. See docs/production-costs.md; deployed Live verification remains pending.

## Core CRM ? 10 October 2026

Two focused crm_workspace_test.dart tests passed: accepted-write recovery without duplicate tasks, task history, interaction corrections/revisions, atomic tag/segment membership, relationship archive recovery, programme/tier replacement, loyalty adjustment/reversal, corporate contact/location isolation and registry-backed audience preview. These are local Mock/contract checks, not deployed Live certification. Final device, permission, theme and server verification remains pending.

## CRM campaigns ? 10 October 2026

Two focused campaign_workspace_test.dart scenarios passed for preview/preparation, lost accepted run recovery without duplicates, scheduled pause/resume/cancel with body-version recovery, scoped delivery/history, immutable definition scope, experiment allocation/holdouts and invalid variants. The two existing core CRM tests also passed after controller integration. Mock historical delivery fixtures are separate from new audience eligibility: UNKNOWN/ineligible snapshot channels create no new delivery rows. No deployed server, notification delivery or final phone certification is claimed.

## Cloud kitchen ? 10 October 2026

Three focused cloud_kitchen_workspace_test.dart checks passed: settings defaults/version 0 and first write, parent/band version checks and atomic overlap refusal, shared-capacity preview, duplicate-safe package creation, plan lifecycle/edit, exact success-only exception acknowledgement, occurrence retry/replay/reschedule/cancel, and package/component detail at 390 ? 844 in light and dark themes. Focused analyzer reports no issues. This is local contract/Mock validation; deployed workers, capacity, scope/permissions and physical-device verification remain pending.


## Pastry foundations - 10 October 2026

Two focused pastry_workspace_test.dart tests passed: product compare-and-swap refusal, attachment/option propagation of product versions, success-only group/option/submission acknowledgements, immutable specification revisions, request submit/cancel and lost accepted creation recovery with no duplicate request. Focused analyzer reports no issues. These are local Mock/contract checks. Quote/change/milestone/production flows and deployed/physical-device verification are not yet complete; see docs/pastry.md.


## Pastry workflows - 10 October 2026

Five focused tests passed across pastry_workspace_test.dart and pastry_workflows_test.dart: existing catalogue/request recovery, quote revision with valid hold reuse, real purpose-bound OTP verification, duplicate-safe approval after a lost response, acceptance/confirmed shared capacity, ordered milestones/history and production readiness, oversized batch refusal and duplicate-safe batch recovery, design comparison, immutable decisions and refusal to quote an unapproved specification. Approval grants are excluded from the durable command journal. Focused analyzer reports no issues. This is local contract/Mock verification; final physical-device, theme/layout, deployed permission, worker and external conversion checks remain pending.

## Pastry configuration - 11 October 2026

Focused analyzer reports no issues. Six Flutter tests passed across pastry_workspace_test.dart and pastry_workflows_test.dart, including scoped attachment discovery/removal and extended specification/configuration preservation with invalid Media reference refusal. Backend pastry and section10_11contract packages passed. These are local Mock/contract checks; updated-service deployment, Live access checks and final phone/layout verification remain pending.

## Recurring creation - 11 October 2026

Five focused scheduling_workspace_test.dart checks passed, including a lost accepted creation replay with unchanged IdempotencyKey, preserved corporate/menu/destination/capacity configuration and no duplicate schedule. Existing management/version/reconciliation and small dark-phone checks passed. No backend deployment changes were required. Final device and Live worker/capacity checks remain pending.

## RFQ quotations - 11 October 2026

Focused RFQ recovery/history test passed: lost accepted response recovered with frozen key/body, no duplicate revision, preserved historical pricing, own-vendor history and closed-request refusal. Five existing scheduling tests and focused backend Section21/scheduling/RFQ/read-denial tests passed. Contract verification reports 732 generated operations with validated source fields/critical flows. Final new-screen phone/layout and Live database/access/capacity/customer-conversion checks remain pending.

## Approved scheduling - 11 October 2026

Seven focused Flutter tests passed across scheduled_creation_test.dart, scheduling_workspace_test.dart and rfq_workspace_test.dart (the new creation test reran after correcting internal Mock reservation support). Recovery reuses the frozen body/key, creates one schedule/reservation, releases capacity on cancellation and refuses foreign-business context. Focused backend approval/branch projection/read-scope checks passed. Contract verification reports 734 operations. New-screen phone/theme/layout and Live database/deployment checks remain pending.

## First-time corporate meal setup - 11 October 2026

Seven focused Flutter checks passed across recurring_first_setup_test.dart and scheduling_workspace_test.dart, including corporate setup creation, frozen-key recovery without duplicates, immutable menu/address tampering refusal and a small dark-phone layout check. The new two-test file was rerun after final setup/review changes. Focused backend recurring/quote checks passed, including fixed-menu projection, corporate read denial, precise large-amount snapshot comparisons and per-occurrence timezone/capacity behavior. Contract verification reports 735 operations with source fields/critical flows validated. Deployed database join/privacy/scope/worker tests and physical-device verification remain pending.


## CRM business-wide insights - 11 October 2026

Implemented existing overview/RFM/churn reads and reviewed rebuild requests, branch/business scope selection, semantic freshness states, animated distributions, skeleton loading and durable original-key recovery. Focused analyzer passed. Two focused tests passed: interrupted rebuild recovery with unavailable scores and branch denial; 360 x 800 overview/RFM/churn layouts in light and dark themes. Backend unchanged. Live worker, permission, privacy and physical-device checks remain pending; see docs/crm-insights.md.


## Pastry verified references and saved requirements - 11 October 2026

Focused analyzer passed. Two focused Flutter checks passed: canonical customer references, foreign CRM-profile/stale archived selection refusal; saved requirement edits retain integer types, unknown/list/null fields and the unchanged source snapshot. Backend customer eligibility/scope checks passed and vendor command wiring compiled. Contract verification reports 737 operations. Live deployment, database privacy/branch races, real permission scope and physical-device verification remain pending. No new nested requirement keys were invented; creation schema remains a backend definition boundary.
