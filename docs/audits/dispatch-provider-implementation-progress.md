# Dispatch implementation progress

Updated: 15 October 2026. Tracks the fixed IDs in [the original audit](dispatch-provider-frontend-audit-2026-10-14.md). Changes are local and have not been deployed. Implemented means code exists; it does not mean a physical-phone or deployed integration check has passed.

| ID | State | Current implementation / outstanding work |
|---|---|---|
| D01 | Implemented | Change PIN sends current/new PIN and handles confirmed rejection/success. Focused regression passes. |
| D02 | Implemented; Live verification pending | Real phone-change start, verification, resend cooldown and expiry; refreshes profile after confirmation. |
| D03 | Implemented | Demo earnings and charts isolated to Mock; Live total uses Wallet available balance or unavailable. Regression passes. |
| D04 | Implemented; phone verification pending | Live rider/trip coordinates, recenter and missing/stale location states. No fabricated Live route geometry. |
| D05 | In progress | Source errors retained, partial results visible, failed-source cache preserved, retry and initial skeleton. Source outage and cached partial-refresh regressions pass; expired requests are hidden. Final lifecycle verification remains. |
| D06 | Implemented; lifecycle verification pending | Mode-scoped credentials, fresh app/controller session on mode switch, Mock-only transport, stopped Live realtime in Mock. Focused isolation regressions pass. |
| D07 | In progress | Canonical mission retained; stops/parcels/manifest/handover/recovery rendered. Assigned replacement riders now receive their own trip-bound commerce custody view; full route-state presentation remains. |
| D08 | In progress | Normalized delivery operations screen and representative Mock flow exist. Commerce pickup route now reads assigned stops, shows sender verification, confirms exact parcels/packages and advances through the ordered stops. A two-stop Mock scenario is available from App preferences. Saved returns, exact-parcel return completion, outgoing code retrieval and incoming custody acceptance now have UI/API flows and Mock scenarios. Commerce exception reports, standalone evidence, required damage/discrepancy photos, saved issue/evidence history and server-derived recovery options are implemented. Optional return-completion and incoming-transfer photo attachments are implemented through reusable scoped drafts. Deployed integration verification remains. |
| D09 | In progress | Stop credentials, scanning, session-bound photo/signature upload, verification and evidence progress implemented. Requires database-backed ownership/replay validation and deployed Media check. |
| D10 | In progress | Timed attempts, contact outcomes, failure reasons, breakdown and reroute wired. Approved hub arrival/release UI now requires the active manifest scans and a photo, with provider-scoped read/upload routes and Mock coverage. Durable account/mode-scoped permitted offline reconciliation implemented; database-backed and phone reconnect validation remain. |
| D11 | Implemented; integration verification pending | Server-side status/creation-date filters apply before pagination and count. Active group matches backend active-trip semantics. Filter sheet preserves selection, reset returns All time, refresh retains filters, stale responses cannot replace changed filters. Focused checks pass. |
| D12 | In progress | Own-provider paginated ledger endpoint, history details, saved banks/default/removal and honest withdrawal result implemented. Wallet refreshes after withdrawal and supports pull-to-refresh; Mock history reconciles with balance and idempotent withdrawal replay. Withdrawal bank selection now carries the exact chosen account through review/PIN authorization; uncertain retries preserve identity, and Mock bank save persists through the transport. Ledger scope integration validation remains. |
| D13 | Implemented; lifecycle verification pending | Bootstrap retry/sign-out recovery preserves session; operation draft persisted per provider/mode. |
| D14 | In progress | Preferences, notifications and earnings entries wired; unsupported security controls removed from active navigation. Remaining active-route sweep required. |
| D15 | Implemented | Global Inter font shared with Vendor. |
| D16 | In progress | Teal primary tokens, minimum button heights and theme surfaces updated. Remaining active screens require theme consistency review. |
| D17 | In progress | Shared skeleton, semantic notices, animated confirmation, per-action loading and honest pending states added. Apply remaining active flows. |
| D18 | In progress | Root transitions, accessible nav states and responsive operation content added. Narrow-phone, keyboard, large-text and navigation walkthrough remains. |

## Verification already completed

- 19 foundation Flutter regressions: PIN rejection, Mock/Live credentials and transport, idempotent retry, Live earnings, existing PIN/model/compatibility behavior, source outage/cache handling and withdrawal replay consistency.
- Changed Dispatch secure-handover, trip and router packages compile; focused capture-content/provider checks pass.
- Wallet provider HTTP authorization-surface checks pass; affected repository/usecase packages compile.
- Dart library analysis reports no compile errors. Existing lint/warning cleanup remains.

## Next work, in order

1. Validate implemented mission, hub and permitted offline reconciliation contracts with a database and physical-phone reconnect walkthrough (D07-D10).
2. Finish remaining source lifecycle and active navigation gaps, plus ledger scope integration validation (D05/D12/D14). Activity filtering implementation is complete; final Live verification remains.
3. Complete shared states/themes and phone layout walkthrough (D16-D18).
4. Run focused integration validation for added backend contracts, then agreed Live walkthrough. No completed ID is reopened without new evidence.

Commerce pickup checks: three focused app checks pass (sender-code direction, exact parcel/manifest collection, and the complete two-stop Mock sequence). Provider read handler identity/private-field/not-found checks and malformed-trip checks pass. Commerce and router packages compile. Source analysis identified a constant-expression error in the demo completion message; corrected and verified by the passing app checks. Physical-phone and database-backed integration verification remain pending.

Return and transfer increment (15 October):

- Added saved return records, policy-specific request form and explicit completion confirmation. Product type, disposition, reason, parcel references, timestamp and optional notes use existing backend command fields.
- Added assignment-scoped transfer status for current and replacement riders; outgoing-only credential read uses no-store and rejects expired/inactive challenges. Incoming acceptance requires code, confirmed packages and a fresh location.
- Mock scenario supports replacement assignment and incoming-rider simulation. Ordinary mission reads exclude codes.
- Corrected return custody validation to require this provider's custodian reference. Corrected replacement trip startup to select the mission's exact trip after assignment changes, rather than joining every trip on the booking.
- Six focused commerce app checks pass. Focused Go commerce/provider/return/transfer checks and router compilation pass. Final analysis of mission screens and preferences reports no compile errors. Six focused commerce app checks passed after the custody-state adjustments. Return cards also show original vendor addresses from the saved stop/parcel projection.
- Physical-phone and database-backed multi-provider integration checks remain pending. Existing return policy/mission lifecycle behavior has not been redesigned in this increment.

Commerce exceptions/evidence increment (15 October):

- Assigned mission reads now include permitted reason/recovery choices and photo requirements derived from the backend RecoveryAllowed policy, plus saved exception/evidence summaries. Unavailable alternative-receiver and fee-approval paths are excluded.
- Full-screen report/evidence forms support exact parcel selection, optional notes, camera/gallery images, previews, required-photo gating, distinct upload/submission states and confirmed-result feedback. Upload and uncertain command retries retain their request context.
- Added provider-scoped, size/image-validated Commerce evidence upload through Dispatch to Media. Files are bound to the assigned mission, provider and exact parcel set; app holds no service credentials. Non-active uploads ask the user to retry processing instead of submitting an unconfirmed attachment.
- Pickup repository commands now reject PICKUP_BLOCKED after package discrepancy/prohibited/restricted reports, matching the existing fail-closed policy intent. The read projection and Mock show the same blocked state.
- Eight focused commerce app checks pass, including new exact-photo-scope/pickup-block and required-photo UI checks. Focused image/policy/provider Go checks and affected router compilation pass. Mission/network source analysis reports no compile errors. Final database-backed and physical-phone checks remain pending.

Custody photos and hubs increment (15 October):

- Optional photos on saved return completion and incoming rider handover preserve capture time and reuse the uploaded reference only for the exact upload path, parcel set and context. Changed transfer bindings clear the draft. Photo selection does not spin unrelated controls.
- Shared multi-manifest transfer evidence now resolves the outgoing custodian, records each held manifest's exact parcel subset with the incoming rider as actor, and retains stable photo capture time for retries. Media metadata carries server-derived tenant/custody intent; the configured shared Media authorizer accepts the canonical Dispatch owner service alongside its existing service alias.
- Added assigned-provider approved hub read/photo upload routes. Arrival/release follows existing hub states; no new hub-selection or approval control exists in Live. Hub screen gates submission on exact scans and photo and shows confirmed checkpoint results. Mock exposes an explicit approved-hold simulation.
- Five focused app checks pass (photo scope/capture timestamp, full hub gate sequence, existing return/transfer checks). Delivery recovery, commerce and router packages compile and their focused checks pass. Final mission source analysis reports no compile errors. Physical-phone and database-backed shared-custody/Media integration checks remain pending.
- Offline reconciliation is the next unfinished part of D10. Its location acceptance/ingestion retry semantics must be checked before attaching a durable client queue.


Offline reconciliation increment (15 October):

- Added encrypted, account- and mode-scoped saved updates, Home notice and Preferences entry. Failed location sends retain actual capture timestamps; retries preserve operation identity across app restarts, use backoff and respect the existing 24-hour replay window.
- Only location history and media references enter this queue. Evidence references remain pending delivery confirmation; OTPs and delivery-completion commands are never replayed automatically. Uncertain evidence writes retain references without storing credentials or codes.
- Backend accepted-location ingestion and command recording now share a transaction. Historical observations retain observed time, avoid displacing newer tracking fixes, and cannot make stale tracking look fresh.
- Mock provides offline replay without network access. Two focused app checks pass (durable retry/account separation and evidence requiring confirmation); focused route-integrity/router Go checks pass. Library analysis found no compile errors before the final Mock addition; unrelated existing unused-import warnings remain.
- Deployment requires Dispatch migration 000069_offline_location_atomic before this backend build. Database-backed transaction validation and physical-phone reconnect walkthrough remain pending. No deployment or push performed.


Activity filters and notification navigation increment (15 October):

- Extended the existing own-provider trips endpoint with optional RFC3339 created_from/created_before bounds and the active status group. Dates filter trip creation time; both count and paginated rows share the provider/date/status predicates. Stable created_at/id ordering breaks timestamp ties. Invalid bounds fail validation.
- App sends selected status/date on initial filtered load and every subsequent page. Switching filters resets pagination and ignores outdated in-flight responses. Pull-to-refresh and retries retain filters; the sheet preserves current values and reset restores All time. Mock applies the same visible filters. Filtered empty states explain how to recover.
- Removed the non-working Activity Go offline action and unsupported notification archive/overflow controls. Notification assignment action opens the real opportunities list instead of falsely reporting acceptance; detail close action has an accurate label. Initial notification loading uses the shared skeleton.
- Five focused Flutter checks pass (filter race/pagination, sheet selection/reset, existing notification API/controller checks). The existing notification controller test now explicitly selects Live so it exercises the injected HTTP transport rather than default Mock data. Focused Go activity validation/query-scope and provider-route authorization checks pass. Database-backed date pagination and physical-phone walkthrough remain pending. Nothing pushed or deployed.


Wallet navigation and withdrawal increment (15 October):

- Withdrawal account row now opens a scrollable chooser with masked account numbers, selection state and Add bank navigation. Choosing a destination does not change the saved default. The chosen account ID carries through review and PIN authorization; missing accounts and unresolved bank/balance loads cannot enable Continue.
- Banks and balance have independent loading/error/retry states; balance failure does not display a fabricated zero balance. Amount/account content scrolls independently above the keypad on short phones. Fixed a narrow-phone add-account label overflow and made wallet quick actions scrollable.
- Removed fixed zero-fee/net-payment claims from review because no fee quote is returned by this flow; review shows the requested amount. Uncertain withdrawal responses preserve the request identity across same-screen retries, including the actual network error code and server failures.
- Mock add-bank verification/save now uses the shared mode-isolated API transport instead of returning apparent success without saving. Inputs lock during verification/save. Wallet's empty-state opportunities button now switches to the real Opportunities tab.
- Three focused widget checks pass: selected-account propagation and uncertain retry identity, missing-bank gating/add navigation at 320x640 in dark mode, and Mock verify/save visible in saved accounts without network access. Changed-source analysis reports no compile errors. This increment uses existing backend contracts; no backend deployment change added here. Physical-phone and deployed Wallet validation remain pending. Nothing pushed or deployed.
- Remaining navigation sweep includes support/dispute controls and distinguishing inactive legacy screens from reachable flows. Shared states/theme/layout work and final integration validation remain on the established backlog. The existing static bank-name catalog also needs reconciliation against a supported bank-directory contract before claiming a complete bank catalog.


Support/dispute navigation increment (15 October), tracked remaining item 1:

- Help Centre chat opens the existing bearer-authorized POST /dispatch-provider/support-chat/start endpoint. The backend finds/creates the conversation; the app no longer infers agent availability or performs its own list-then-create fallback. Wallet Help now reaches this same support surface.
- New case form sends the entered subject and description with a category from the backend allow-list. General cases do not require a previously selected trip and cannot accidentally reuse stale dispute selections. Help topic rows prefill the subject instead of just echoing their label in a toast. Submission disables inputs and shows the shared confirmation only after a case ID is received.
- Removed the misleading attachment step that only gathered local paths without uploading them. This change does not claim support photo/file upload is implemented; the backend evidence-record API requires a real authorized Media asset, and the unused local picker is no longer in the create-case route. Media/integration work remains covered by final contract validation.
- Removed fake Reopen case success because no caller reopen route exists. Resolved/closed cases offer a new case; open cases link to the real conversation. Wallet dispute details share the themed case screen. Case labels distinguish exact backend statuses; progress no longer invents under-review timestamps or completed awaiting-information milestones.
- Message send failure retains the exact editable text and shows an error; no fake local message, synthetic retry text or sent confirmation. Failed reads show retry instead of empty success and preserve cached messages. Conversation generation prevents an older response entering another case; in-flight reads merge confirmed messages by ID. Controllers guard late notifications after disposal.
- Seven focused checks pass: existing complaint creation/message parsing, backend-owned chat start and failure, entered case data, failed message preservation, and in-flight conversation isolation. Changed support sources analyze without compile errors. Changes use existing backend routes; no backend edits, deployment or push in this increment.
- Remaining source/status refresh work, bank directory, shared presentation/layout polish, deployed integration and APK validation stay on the six-item list. Case detail freshness and physical-phone chat lifecycle remain for items 2/5; do not claim deployed verification.


Data-state and recovery increment (15 October), tracked remaining item 2:

- Temporary refresh/network/server failures preserve in-memory access/refresh/provider identity and saved credentials. Protected transport propagates temporary refresh failure instead of invoking session-expired handling; only confirmed unauthorized refresh rejection clears credentials. Restore routes temporary failure to connection recovery and preserves a newly refreshed session when subsequent account loading fails.
- Request acceptance checks actual expiry locally; detail actions switch to expired at the deadline and request lists schedule expiry notification without waiting for network polling. Expired-detail back actions no longer pop navigation twice. Accepted/declined offers are removed from per-source cache and excluded from outdated refresh data so a failed source cannot resurrect them. Refresh reads coalesce and disposal guards prevent late notifications.
- Acceptance/decline responses require a readable success envelope; malformed 2xx responses and explicit failures containing data cannot claim success. Empty opportunities offer a real Refresh action rather than promising unregistered notifications.
- Active-delivery refresh failures retain cached delivery details and expose a Home retry notice. Malformed current-delivery payloads fail instead of masquerading as no delivery. Confirmed no-active-delivery clears old dashboard activity; a different delivery with the same status updates its identity. Returning from background refreshes delivery and request state.
- Case details refresh through the existing owned GET route on entry, app resume, return from chat and explicit retry. A refresh error preserves the saved case with an accurate notice; status labels distinguish evidence requests, escalation, closure and unknown state.
- Eight focused checks pass: temporary versus confirmed refresh rejection, protected-refresh outage callback behavior, handled-offer cache protection, local expiry rejection, active-trip cache/error recovery, two existing source outage/cache checks, and refreshed case status surviving a subsequent outage. Full app library analysis reports no compile errors; existing unrelated warnings/lints remain. No backend edits, deployment or push in this increment. Physical-phone lifecycle and deployed integration remain under items 5/6.

## Fixed remaining-work list checkpoint

1. Support/dispute navigation: main fixes implemented locally; authorized file attachment integration remains with item 5.
2. Data states and recovery: core fixes implemented locally; phone/session lifecycle validation remains with item 5.
3. Bank directory: implemented locally; authenticated Wallet directory, searchable picker, retry/empty/error states and isolated Mock data. Requires Wallet deployment and Live verification.
4. UI/UX polish: remaining shared states, themes, small screens, keyboards and large text.
5. Integration verification: permissions, Wallet ledger, Media/evidence (including support attachments), mission/hub/transfer/return/offline and lifecycle checks against running services.
6. Deployment and final phone testing: authorized deployment/migration, full Live walkthrough and tester APK.

Completed implementation stays completed; final integration verification is tracked separately rather than silently reopening items.


Bank directory increment (15 October), tracked remaining item 3:

- Added authenticated GET /api/v1/payment-wallet/provider/banks. Wallet uses its existing private Paystack client credentials; the app receives bank name/code/currency only. No service secret is embedded in the app. Existing account-resolution and registration contracts remain.
- Upstream GET /bank is scoped to Nigeria/NGN/nuban and uses documented cursor pagination. All pages must succeed; repeated cursors, oversized/unreadable payloads and pagination limits fail without returning a partial directory. Only active, non-deleted supported banks are returned, deduplicated by code and sorted by name. The request has a 30-second overall deadline.
- Removed the app's static Nigerian-bank catalog. Add-account loads the server list and presents a searchable selection sheet. Loading, error/retry and empty states cannot enable verification. The chosen bank code retains leading zeroes, and registration uses the chosen directory name rather than a missing name in the resolve response.
- Mock transport exposes a small explicit demo directory and remains offline; no Live static fallback exists. Existing Mock verification/save continues through the shared transport.
- Four focused app checks pass (existing selected-account/retry, small-phone add navigation, Mock bank persistence, and new directory outage/retry/search/leading-zero flow). Go directory pagination/filter/partial-failure checks and provider-surface authentication/audit checks pass; affected clients/usecases/HTTP packages compile. Changed app sources analyze without compile errors.
- Based on the official Paystack List Banks contract: https://paystack.com/docs/api/miscellaneous/#bank-list . No real Paystack request was made during local checks. Wallet deployment is required for the new route; no database migration is needed for this increment. Nothing pushed or deployed.
- Next is established item 4: shared UI/UX states, themes and responsive layout polish. Items 5/6 retain deployed integration, authorized release and phone/APK verification.


Vendor-aligned design increment (15 October), tracked remaining item 4:

- Copied the exact Vendor Inter regular/bold font assets and aligned shared typography, teal button contrast, neutral dark surfaces, filter states, back controls and route transitions. New installations default to Light; saved appearance preferences remain respected.
- Rebuilt the three-page introduction using existing branded rider imagery and responsive compositions. First-visit introduction memory is mode-scoped; returning users skip it. Incomplete authenticated profiles resume the setup hub.
- Rebuilt the six-step setup hub and shared setup headers. Checkmarks reflect actual saved setup values, current/future steps have distinct states, and saved data is not described as verification approval. Removed the unsupported 24-hour review promise and corrected the pending-screen Continue action label.
- Added illustrated shared empty states for opportunities, activity, wallet and notifications, with real recovery actions. Shared skeletons now animate and respect reduced motion. Mock/Live settings apply only on confirmation, display the configured Live address, and offer appearance controls. Runtime address editing is not included.
- Eleven focused Flutter checks pass across design consistency, withdrawal navigation and support flows. Includes 320x640 introduction with larger text, dark setup, saved-step navigation, mode confirmation and introduction-memory separation. Library analysis reports no compile errors; existing warnings remain. Diff whitespace check passes.
- Phone-sized captures are available under apps/dispatch_provider/build/design-previews/. Physical-phone review and deployed integration checks remain items 5/6; this is not a claim that every reachable screen has passed a final visual walkthrough. No push, deployment or APK build performed.


Mock preview and onboarding follow-up (15 October):

- Mock now defaults directly to Home with a seeded completed demo profile. Data source offers persisted Dashboard, Account setup, and Introduction/sign-in entry points. Selecting a different preview remounts the Mock session; Live authentication and server data remain separate. Continuing from completed Mock setup saves Dashboard as the next entry point.
- Setup fields, signup/login identifiers, OTP and a local demo photo prefill only in Mock. Account setup preview starts unsaved, so populated inputs still require explicit saving. Preview fixtures intentionally reset on launch; this is not persistence of every Mock mutation.
- Dashboard still defaults to Home; inactive tabs now mount only when first selected. A root-app regression check confirms Home is visible and Wallet is not mounted after Mock startup. This removes inactive tab initialization side effects without claiming a separate server-side routing issue.
- Added reusable custom line illustrations for introduction, operation choices and setup-step cards, replacing the bike glyphs. Auth/setup failures share a neutral floating notice with semantic error icon and readable text.
- Six design/configuration checks passed, followed by a focused full-root Home-versus-Wallet startup check. Library analysis before the last auth-notice substitution reported no compile errors. Nothing pushed or deployed.


Home-only redesign (15 October):

- Home is map-first with a compact floating branded header, neutral status surface and shared Inter text styles. Account avatar now opens the Account tab; notifications and active-delivery controls retain their real routes. Removed the oversized status/stat block and non-geographic Mock area labels.
- Unified Mock/Live map presentation, preserving real Live coordinates and pickup/dropoff markers. Added a custom depth-styled animated rider marker, animated recenter, bearing reset, dark tile rendering, visible map attribution, tile failure/retry and availability-load retry. Reduced-motion preferences stop decorative pulses and camera motion. No turn-by-turn geometry, 3D building renderer, demand overlay or invented Live coordinates added.
- Redesigned offline/online cards and Go online connecting, location and readiness sheets. Connecting now waits for the real task outcome and does not advance a timer-driven fake checklist or display successful checks for failed requests. Network failures use the shared semantic error notice. Existing availability/permission/readiness commands remain the authority.
- Eight focused design/startup/state checks pass, including Home versus Wallet startup and the Go online failure result. Fixed a lazy map-controller disposal error found during those checks. Home source analysis has no errors; minor existing/style lints remain. Final phone visual review remains pending. No push/deployment/APK build performed.


Home viewport correction after phone screenshot (15 October):

- The previous visual claim was incomplete: startup checks asserted routing but did not assert viewport geometry. Home's loosely sized Stack/IndexedStack shrank the map to header height, exposing the white Scaffold below it.
- Home and the tab stack now explicitly expand. Moved status cards into a bounded, scrollable body overlay instead of the persistent Scaffold bottom-sheet surface, preserving active delivery callbacks.
- Added root-layout regression assertions: map height exceeds 600 at a 390x844 viewport, offline card stays below 240, and the card occupies the bottom portion. The focused root check passes.


Home request/trip interaction redesign (15 October):

- Incoming opportunities now sit on a rounded opaque surface with stable ID-based paging, responsive custom parcel/route/wallet artwork, wrapped metadata and one-word Accept/Decline actions. Full offer details use a dimmed scrollable sheet with backend-provided notes/item/handling data.
- Accepted trips open a dedicated foreground route with one scroll owner and a compact map preview on Home. Pickup credentials and resend, arrival/start, cancellation reasons, issue reporting, handover proof and completion retain their existing backend commands. The Tasks action preserves the existing detailed delivery/evidence screen.
- Go offline uses semantic red and a confirmation decision. Acceptance, decline, arrival/start, cancellation/issue recording and completion have actual-result feedback; waiting for pickup verification is amber. Mutation failures use a dimmed error sheet; local proof input validation remains inline. Request and primary trip mutations lock while pending.
- Fixed initial availability updates to run after the first Home frame. The new small-phone end-to-end Mock interaction check caught an expiry timer surviving session disposal; the session owner now disposes its RequestsController.
- Nine focused design/state/root interaction checks pass, including going online, accepting a request, opening pickup and scrolling to cancellation at 320x640. Source analysis has no errors. New phone build/visual review follows. No push or server deployment.

- Follow-up visual review: generated and inspected 320x640 request and pickup previews under build/design-previews/home-request-small.png and home-pickup-small.png. Tightened preview spacing so Accept/Decline remain fully visible; full notes/items/handling stay in the details sheet. The focused accept/scroll check passes after this adjustment.
- Handover has a return-to-delivery action while not submitting; its focused view hides the tab bar. Submission failures retain form state and surface contextual error feedback. Source analysis reports no errors after these changes.
- Phone installation was attempted but device 15192375CF009258 was disconnected/not detected. This increment has not been installed or visually verified on the physical phone. No push/server deployment.


Home map, swipe and Mock verification follow-up (15 October):

- Incoming requests swipe horizontally with page snapping, counters and arrow alternatives, without accepting/declining on swipe. Natural card height and vertical parent scrolling are retained. Stable selected request is reconciled after list updates and viewport changes.
- Active trip preview now has Pickup/Verify/Delivery progress and an explicit View delivery action. Shared back controls retain a 48px target but have transparent backgrounds.
- Added a custom painted scooter marker, heading rotation, animated real GPS transitions, labelled endpoint markers, route overview fitting and stored-polyline drawing. Mock coordinate loss during trip creation/advance is fixed. Mock paths/motion are labelled demonstrations; no Live road geometry is invented when missing.
- Provider-owned trip active/detail reads now expose the existing stored booking route polylines. No migration or new external routing dependency. Live route visibility requires deploying this Dispatch change and having populated route geometry; absent geometry is explained on the map.
- Mock-only Simulate customer scan changes the local pickup credential to verified. Mock reads preserve this state and resending resets it. Live cannot invoke the simulation and still relies on backend verification.
- Eleven frontend checks passed, followed by a focused full go-online/accept/arrival/simulated-scan/start-delivery check. Backend provider-trip/route privacy and ownership-guard checks pass using the installed Go toolchain and cached dependencies. No push/deployment performed. Physical-phone review remains pending.


Handover picker-return correction (15 October):

- Refreshing an in-progress trip on app resume previously replaced the local handover view with the map. Both controller synchronization and explicit refresh now preserve handover for the same trip while its authoritative status remains in progress/proof submitted. Changed/terminal trips still reconcile normally.
- File-picker completion now checks mounted before changing form state. The existing bike-style marker is rendered at 80% scale.
- Focused Mock acceptance/arrival/verification/start/handover regression passes, including inactive/resumed lifecycle events while handover stays visible. Physical picker round-trip on the phone remains to be confirmed. Nothing pushed/deployed.


### Opportunities and Activity redesign ? 2026-10-15

- Replaced both tab layouts with shared Inter typography, custom journey icons, neutral light/dark panels, teal filters and semantic status colours.
- Opportunities now distinguish updated, expired and taken offers; missing route/fare fields remain absent. The short-distance filter is labelled Under 5 km rather than implying geographic proximity.
- Request details use a separate scrollable screen with fixed decisions. Decline reasons use a focused sheet and only supported reason codes. Acceptance feedback opens the actual current delivery.
- Activity groups records by date, keeps pagination below the list, and preserves backend status/date filtering. Delivery details show recorded events, available recipient details, saved route geometry and the current delivery action.
- Proof and rating screens distinguish unavailable previews, unverified evidence and missing ratings. Removed the simulated download action. Private media references still need authorized viewable URLs before they can render as images.
- Focused verification: the existing 20 filter, request-loading, model and Home/design checks passed. Additional narrow-screen action and unrated-state checks cover the new layouts. No deployment performed.


### Wallet and Account redesign ? 2026-10-16

- Wallet now uses a responsive balance panel, immediate amount masking, direct bank/earnings/history actions and shared ledger cards. Missing balances show unavailable rather than a fabricated zero; credits and debits have distinct neutral/success presentation.
- Bank selection is searchable and server-driven. Registration uses a verified-holder review panel and animated success sheet. Withdrawal selection, amount review, PIN setup/change and paid/pending/failed results preserve existing wallet calls and idempotency behavior.
- Account now displays the signed-in rider name and groups personal details, rider tools and settings. Profile, photo, contacts, preferences, phone verification, documents, deactivation and vehicle management use common Inter typography and custom vector icons.
- Removed the fabricated fallback email, unsaved date-of-birth and guarantor-relationship inputs, inert identity Help action, misleading earnings period controls and unconfirmed reactivation promise. Phone edits use the actual verification flow.
- Document review uses server steps, progress, reasons and state gates. Vehicle details stay accessible for every review state. Upload selection supports the advertised image/PDF types and 5 MB limit.
- Deferred initial form reads until after the first frame to prevent ancestor-notification build errors. Busy withdrawal submission blocks back navigation.
- Verification: nine focused Wallet/Account, bank directory, PIN and withdrawal checks passed, including narrow-phone light/dark navigation and retry identity. Generated local Wallet and Account previews. No deployment or push performed.


### Delivery task action hierarchy ? 2026-10-16

- Replaced the delivery Tasks stack of equally prominent filled buttons with one state-aware next-step CTA and grouped secondary task rows. All original backend-authorized actions remain accessible.
- Kept delivery help, route changes and recipient-unavailable handling separate from routine handover tasks. Starting the recipient-unavailable waiting flow now requires an explicit explanatory decision.
- Moved Mock hub simulation into a Demo controls expander. Consolidated commerce recovery reporting/evidence into compact support rows and reduced pickup options button emphasis.
- Next-step selection accounts for the current handover, missing parcel scans/evidence, pickup/delivery/return stage, recovery waiting period and backend completion state; this changes presentation rather than backend permissions.
- Verification: seven focused delivery-task, commerce pickup and recovery checks passed, including one-primary-action hierarchy and cancelling recipient-unavailable confirmation without a write.


### Central email delivery ? 2026-10-16

- Dispatch runtime email now uses authenticated Notification requests through the existing DISPATCH_NOTIFICATION_URL/SECRET. SMTP credentials were removed from its configuration; the legacy SMTP adapter is not selected at startup.
- Notification SMTP has a configurable whole-operation timeout, context cancellation, strict STARTTLS, checked sender/recipient addresses, sanitized headers and permanent/transient SMTP error classification.
- Missing SMTP configuration rejects email-only requests before persistence/queueing. Explicit logging email is limited to development/test; staging/production do not report logging as real delivery. Existing multi-channel push behavior is preserved.
- Vendor and Dispatch generate a fresh notification identity for each OTP send, preventing an earlier code's delivery record from suppressing a new code. Authentication OTPs use email only; SMS remains a separate integration.
- Focused Go checks passed for Notification clients/usecases/cmd, Dispatch auth clients/usecases/config (cmd compiled), and Vendor auth clients. Tests use loopback SMTP/HTTP fixtures, not live email accounts.
- Deployment guide: deploy/notification-service/SMTP_STAGING.md. Sender address and privately entered VPS credentials are still needed. No push/deploy, real SMTP authentication, inbox test or debug-setting change performed.
