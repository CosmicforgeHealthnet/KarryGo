# KarryAm Dispatch Provider — Full UI/UX Master Specification

> Markdown mirror of the authoritative DOCX specification for Codex/Figma execution. Source: `KarryAm_Dispatch_Full_UIUX_Master_Specification.docx`.

**KARRYAM**

**Dispatch Provider Mobile App**

**FULL UI / UX MASTER SPECIFICATION**

Backend-audited • Frontend-audited • Mobile-first • Implementation-ready

| Document status | Master design baseline |
| --- | --- |
| Platform | Android + iOS mobile app |
| Reference frame | 390 × 844 logical design frame |
| Source branch | CosmicforgeHealthnet/KarryGo - staging |

| Purpose This document is the authoritative UI/UX design brief for the KarryAm Dispatch Provider mobile application. It reconciles the current Flutter frontend, dispatch backend, wallet/finance backend, notification infrastructure and support/safety backend so the redesign does not omit implemented or backend-supported product surfaces. |
| --- |

# 1. Document Control and Audit Basis

## 1.1 Audit scope

This specification is based on a product-level audit rather than a single-service audit. The Dispatch Provider app is a multi-service mobile client. The current Flutter app explicitly configures separate dispatch, payment-wallet, notification and support backends and instantiates controllers for all of those domains.

Dispatch Provider Flutter app: apps/dispatch_provider

Dispatch service: services/driver-dispatch-delivery-service

Payment / wallet service: services/payment-wallet-service

Notification service / realtime channel used by the app

Support / dispute service: services/support-dispute-service

Fleet / organization / company-assignment surfaces consumed by dispatch provider flows

Current staging branch is the implementation baseline unless a later product decision supersedes it.

## 1.2 Verified implementation evidence

| Area | Source | What it proves |
| --- | --- | --- |
| Frontend bootstrap | apps/dispatch_provider/lib/app/dispatch_provider_app.dart | Wires auth, profile, verification, vehicle, availability, requests, trips, wallet, notifications and support/disputes. |
| Frontend environment | apps/dispatch_provider/lib/core/config.dart | Separate dispatch, wallet, notification websocket and support base URLs. |
| Dashboard/navigation | apps/dispatch_provider/lib/features/home/ui/dashboard_screen.dart | Current five-destination structure: Home, Requests, Trips, Wallet, Profile; active trip restoration and canonical server-state syncing. |
| Provider wallet API | services/payment-wallet-service/internal/features/wallets/http/routes.go | Provider earnings, bank accounts, withdrawals and transaction PIN are real backend surfaces. |
| Provider finance operations | services/payment-wallet-service/internal/features/finance/http/provider_operations.go | Provider sub-wallet, allocations, withdrawals and statements exist for organization/fleet finance operations. |
| Support API | services/support-dispute-service/internal/features/support/http/routes.go | Complaints, evidence, support chat, messages, read state, FAQs/categories and SOS are backend-supported for provider caller groups. |
| Dispatch support client | apps/dispatch_provider/lib/features/disputes/data/rider_support_api.dart | Provider complaint list/create/detail/chat routes are already consumed by Flutter. |

## 1.3 Audit confidence labels

CONFIRMED - implemented in current frontend and/or directly supported by audited backend routes/models.

NEW REQUIRED UX - backend capability exists but current provider frontend does not yet expose the complete experience.

KEEP & REDESIGN - existing frontend feature should remain but needs new KarryAm visual/interaction treatment.

VERIFY / LEGACY - old frontend surface conflicts with current backend architecture or needs a product decision before redesign.

DO NOT INVENT - a feature must not be drawn as functional unless its backend owner or explicit product decision is established.

| Critical correction from the earlier V1 Figma The first 78-screen design represented only the core dispatch mission surface. It was not the full app. Wallet, earnings, withdrawals, support/chat, SOS, account-security and many setup states are part of the real product and must be represented in the new design. |
| --- |

# 2. Executive Product Picture

KarryAm Dispatch Provider is a field-operations application for independent and fleet-linked delivery providers. It must coordinate identity/readiness, online availability, dispatch opportunities, real-time delivery missions, parcel custody, secure handovers, evidence, recovery, financial earnings/withdrawals, notifications, support and safety.

## 2.1 Product systems

### Trust & Readiness

Authentication, onboarding, identity, face, licence, guarantor, emergency contact, vehicle and account readiness.

### Operations

Availability, location, opportunities, assignment sources, trips, mission actions, multi-stop work, parcels and route integrity.

### Custody & Proof

Pickup/delivery credentials, OTP/QR, manifest/version control, scans, evidence sessions, delivery proof and recipient confirmation.

### Recovery & Resilience

Receiver unavailable, breakdown, retry/reschedule, return-to-sender, hub hold, provider transfer, offline replay and conflict recovery.

### Money

Wallet, earnings, transactions, bank accounts, withdrawals and transaction PIN.

### Support & Safety

FAQs, support cases, evidence, chat, SOS, notifications and account support identity.

## 2.2 Proposed primary navigation

| Home | Opportunities | Activity | Wallet | Account |
| --- | --- | --- | --- | --- |

Notifications remain globally accessible from the app bar. A persistent Active Mission affordance sits above normal navigation whenever a canonical active mission exists. The active mission always wins over ordinary browsing and must be recoverable after app restart.

# 3. UX Principles and Non-Negotiable Interaction Rules

1. Server state is authoritative. UI progression follows the backend state machine and allowed actions, not a hard-coded visual sequence.

2. Active mission is always recoverable. Reopening the app or changing tabs must not strand the provider away from the canonical active mission.

3. Protected custody fails closed. Secure handover, manifest and evidence rules may not silently fall back to a weaker legacy completion path.

## 4. Offline is explicit. “Saved on this device” is visually distinct from “Confirmed by KarryAm”.

5. One user intent equals one idempotent operation. Double taps and retries must not create duplicate backend operations.

6. Version conflict stops progression. On source-version or manifest-version conflict, show a refresh/reconcile state and refetch canonical state.

7. Operational UI is sunlight-readable and one-handed. Driving/field states use large targets, minimal text and reduced visual distraction.

8. Privacy by default. Receiver details, OTP/QR credentials and sensitive support/financial data must not leak into notification previews, logs or ordinary list cards.

9. Do not fabricate missing commercial data. Organization offers without a fare amount must not display ₦0 or synthetic earnings.

10. Every important state has a recovery path: loading, empty, permission-blocked, offline, rejected, expired, conflict, restricted and retry states are designed deliberately.

## 3.1 Accessibility baseline

Minimum interactive target: 44 × 44 logical points.

Do not communicate status by color alone; pair color with icon, label and/or shape.

Support dynamic text and text scaling without clipping primary actions.

Provide screen-reader labels for icons, scan controls, maps and status indicators.

Provide list equivalents for important map information.

Haptic/audio scan feedback must have a visual equivalent.

Avoid decorative or distracting animation during active driving/navigation states.

High-contrast outdoor readability is required for mission and handover screens.

# 4. Mobile Design System Requirements

## 4.1 Frame, grid and spacing

Reference frame: 390 × 844. Designs must remain responsive across common Android and iOS widths.

Use safe-area-aware top and bottom regions; never place critical CTAs under system gesture areas.

Primary horizontal content margin: 20-24 pt. Dense operational rows may use 16-20 pt where justified.

Use an 8 pt spacing rhythm with 4 pt micro-spacing for compact metadata.

Critical mission CTA stays reachable in the lower thumb zone whenever safe.

## 4.2 Visual hierarchy

KarryAm should feel premium, operational and trustworthy rather than decorative. Use the approved KarryAm brand system already established for the project. The UI should reserve the accent color for active movement, success, availability and primary action emphasis; dark structural tones carry authority and navigation; neutral surfaces carry information density.

## 4.3 Required component families

Buttons: primary, secondary, destructive, tertiary, loading, disabled

Text inputs, selectors, date fields, phone/email, OTP

App bar, bottom navigation, active mission pill

Status badges and operational state chips

Verification step card and readiness blocker

Vehicle card and document status row

Opportunity card variants and countdown

Mission action tray, stop card, mission timeline

Parcel row, scan state, manifest summary

Secure credential card, OTP/QR state

Evidence uploader, camera/preview/signature states

Route alert, map control, location health state

Recovery option, return/transfer/hub cards

Offline/sync banner and conflict sheet

Wallet balance/earnings card, transaction row

Bank account card, withdrawal summary

Transaction PIN keypad/state

Notification row and deep-link state

Support case card, chat bubble, evidence attachment

SOS control, emergency sheet and safety confirmation

Permission banner, empty state, error sheet, confirmation sheet

# 5. Global State Framework

Every major data-backed screen must be evaluated against the following state families. Not every combination requires a separate route, but every materially different experience requires a designed frame or reusable component state.

| State family | Examples | Design requirement |
| --- | --- | --- |
| Loading | initial load, background refresh, processing | Show progress without blocking unrelated actions; preserve prior canonical data when safe. |
| Empty | no opportunities, no trips, no notifications, no cases | Explain what the user can do next; avoid dead-end emptiness. |
| Permission | location, background location, camera, media/files, notifications | Explain why before system prompt; provide settings recovery after denial. |
| Connectivity | offline, degraded, websocket unavailable, polling fallback | Be explicit about local vs server-confirmed state. |
| Conflict | source version, manifest version, offer accepted elsewhere | Stop local progression; refetch and explain what changed. |
| Restriction | not verified, suspended, expired licence, no vehicle | State the blocker, impact and exact resolution CTA. |
| Expiry | OTP, QR, offer countdown, document expiry | Use countdown/expiry language and a valid recovery action. |
| Failure | network, validation, backend rejection, upload failure | Preserve user input where possible; provide retry or alternate action. |
| Success | submitted, verified, accepted, transferred, completed | Confirm the authoritative outcome, then route to the next meaningful state. |
