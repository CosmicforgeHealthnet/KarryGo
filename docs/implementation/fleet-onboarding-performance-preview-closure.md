# Karryam Fleet onboarding performance and document preview: closure

Date: 2026-11-13. Branch: `staging`. Scope: `web/fleet` (setup flow) and `services/fleet-partner-service` (organization documents). No change to `main` or production.

## Summary

The documents step now shows its cards about **32% sooner** on a cold load and **33% sooner on Slow 4G**. Every document action costs half the requests or fewer and is 20–73% faster. Every attached document has a **Preview** control, which views PDFs, JPEGs, PNGs and WebPs through a new authenticated, read-only Fleet endpoint backed by the existing Media File Service grant and content operations. Desktop and mobile captures of the documents page are **pixel-identical** to the original once the Preview control is removed from layout. The control itself is the only visible difference.

Deployed to staging: backend revision `45ea321` (image `sha256:df053628…`, healthy) and the Vercel frontend bundle containing the change. **Authenticated live testing on staging is still blocked** because it needs an email OTP for a staging account (section 14).

## 1. Audit findings

How the step loaded before this change (`onboarding-context.tsx`, commit `6f5bd91`):

- The provider fetched identity and current organization (round 1), then **all five** organization records: profile, representatives, capabilities, documents and onboarding status (round 2). Nothing rendered until the slowest of the seven requests finished, even though the documents step needs only documents and status.
- Each upload, replacement or removal called `reload()`, which re-read **all seven** records. Save & continue did the same just to check one stage status.
- The upload checksum was computed after the upload authorization returned, adding avoidable serial time.
- Moving to the next step loaded that route's code on demand. The previous step was already prefetched by the Back link.
- The provider persists across step navigation, so navigation did not refetch data. The cost was in cold loads and mutations.
- Backend: `GET /onboarding-status` listed the organization's documents twice per request. Server time per request was 3–10 ms with no remote calls. **The bottleneck was serialized network round trips, not server work**: staging round trips measured 0.15–0.68 s from outside, even for a 401.
- Fleet had no endpoint to read document content. Media already provides object state, short-lived download grants and grant-bound content reads (`ReadObject`, `AuthorizeDownload`, `ReadContent`), implemented by `MediaFileClient` but not exposed through the narrower `MediaContract` that onboarding used.

## 2. Confirmed bottlenecks (measured)

Cold-load request timeline at staging latency, before (ms from navigation start):

```
347→646  identity/profile, organizations/current          (round 1)
654→958  profile, representatives, capabilities,
         documents, onboarding-status                     (round 2, all blocking)
visible at 968
```

After:

```
345→643  documents, onboarding-status, identity/profile,
         organizations/current                            (one round)
visible at 658
646→952  profile, representatives, capabilities           (background, not blocking)
```

A first attempt sent all seven requests at once. On HTTP/1.1, Chromium's 6-connections-per-host limit then queued `/organizations/current` for a whole extra round trip, so the change only requests what the current page needs up front.

## 3. Before and after timings

Method: `web/fleet/scripts/onboarding-perf.mjs` runs production builds of both revisions against the same disposable PostgreSQL API and signed Media test authority. Chromium network emulation adds **300 ms latency** (staging-like, from the measured staging round trips) or the **Slow 4G** preset (562.5 ms, 1.6 Mbps down). Times are measured in the page (MutationObserver + `performance.now()`) until the cards are visible. Cold loads are the median of 5. Raw data: `evidence/fleet-onboarding-performance-preview/perf-baseline.json` and `perf-after.json`.

| Action | Before | After | Change |
|---|---|---|---|
| Cold load, all 4 documents (staging latency) | 962 ms | 664 ms | −31% |
| Cold load, all 4 documents (Slow 4G) | 1790 ms | 1206 ms | −33% |
| Cold load, no documents | 977 ms | 676 ms | −31% |
| Cold load after sign-out and sign-in | 983 ms | 678 ms | −31% |
| Upload one document | 1546 ms | 1240 ms | −20% |
| Replace a document | 1545 ms | 1238 ms | −20% |
| Remove a document | 974 ms | 656 ms | −33% |
| Save & continue | 1260 ms | 348 ms | −72% |
| Return to Documents from the previous step | 340 ms | 43 ms | −87% |
| Return to Documents (Slow 4G) | 620 ms | 39 ms | −94% |
| Open Preview (PDF) | n/a | 349 ms | new |
| Close Preview | n/a | 29 ms | new |

With already available document data, the step renders in about 0.65 s at staging latency and about 1.2 s on Slow 4G, both under the two-second target. These are measured locally under emulated latency, not on the live staging URL (section 14).

**One tradeoff, measured:** on Slow 4G, a click on the *previous* step within about 1.5 s of the documents page appearing took 641 ms (before: 235 ms). The deferred background reads share the slow link with Next's own Back-link prefetch. If the page has been idle a few seconds, the same click takes 73 ms (before: 63 ms; `perf-*-idle-before-navigation.json`). Net time to reach that step is still lower, because the documents page appears about 580 ms sooner.

## 4. API request counts

| Action | Before | After |
|---|---|---|
| Cold load | 7 (2 serial rounds, all blocking) | 7 (1 blocking round of 4; 3 in background) |
| Upload / replace | 10 | 5 (authorize, content, attach, documents, status) |
| Remove | 8 | 3 (delete, documents, status) |
| Save & continue | 7 | 1 (status) |
| Step navigation | 0 | 0 |
| Preview | n/a | 1 |

## 5. Frontend optimizations (no visual change)

- `onboarding-context.tsx`: per-record loading. Records a page needs (`src/lib/onboarding/onboardingData.ts`) are requested with identity and organization when the organization is known: explicitly selected, or this identity's last onboarding organization from the existing persisted organization store, which is cleared on sign-out, identity change and organization switch. They are **applied only after `/organizations/current` confirms the same organization**. The page renders once its own records arrive, using the existing loading indicator. Other records follow in the background.
- `refresh(keys)` for targeted re-reads. `reload()` keeps its full-snapshot contract for the other steps. A ref is the synchronous source of truth, so a caller awaiting a read gets a snapshot that includes it. This also fixed a stale-result bug the browser suite caught during development.
- Cancellation and races: every read takes an `AbortSignal` (aborted on unmount or organization switch). Per-record sequence numbers make the latest read win. A generation guard discards work from an ended organization scope. Existing click guards on the documents page are unchanged.
- In-memory cache: a single entry keyed by identity and *selected* organization, never persisted. It is evicted on scope change, cleared on sign-out and identity change, and revalidated on use. A failed background revalidation keeps visible data rather than replacing it with an error.
- Documents page: upload/replace/remove refresh only documents and status. Save & continue re-reads only status. The checksum is computed in parallel with the upload authorization.
- Setup layout: the next step is prefetched when the browser is idle. Also fixed the pre-existing `set-state-in-effect` lint error that was failing the Fleet CI frontend step on staging (it is now adjusted during render, as React recommends; same behavior).

## 6. Backend optimizations

- `GetOnboardingStatus` reuses the document list it already read instead of listing documents a second time inside stage assessment. Existing indexes on `fleet_partner_organization_documents (organization_id, …)` already cover listing and the new single-document read, so no migration was needed.

## 7. Preview endpoint and security contract

`GET /fleet-partner/v1/organizations/:organizationId/documents/:documentId/preview`, documented in `services/fleet-partner-service/docs/openapi/onboarding-v2.openapi.json`.

1. Authenticated session (bearer header only; a token in the query string is ignored and gets 401).
2. Path organization must equal the session's tenant (otherwise 404, so other organizations' existence is never revealed).
3. `organization.read` permission, so read-only members can preview. Registered in the object-authorization and risk (`SENSITIVE`) registries and the Section 28 route coverage list.
4. The document must belong to the organization and be an active `READY` attachment. A removed or foreign document returns 404.
5. Media `ReadObject`: the object must be owned by this organization (`ORGANIZATION` owner type and reference), fully uploaded, passed safety scanning and not deleted (deleted: 404; pending, unsafe or quarantined: 409).
6. Media `AuthorizeDownload` issues a short-lived grant. Media's own refusal is final (409). Outage: 503.
7. Media `ReadContent` with the grant, bounded to 25 MB.
8. The detected type must be PDF, JPEG, PNG or WebP (otherwise 415). Bytes must match that type's signature (otherwise 415) and Media's SHA-256 checksum (otherwise 409).
9. Response: the bytes with the Media-detected MIME type, plus `Cache-Control: no-store, private`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; sandbox`, `X-Frame-Options: DENY`, `Cross-Origin-Resource-Policy: same-origin`, and `Content-Disposition: attachment`. No Media URL, object reference or grant is returned, and the grant is never stored or logged.
10. Each successful preview writes an audit record (`organization.document_previewed`). The document, its status and onboarding progress are unchanged (asserted in tests).

Frontend: `organizationService.previewDocument` fetches with the session header as a blob and restores the JSON error body for messages. `document-preview.tsx` checks the response type against the four allowed types, re-wraps the blob with that type and shows it from an object URL that is **revoked on close**. Requests are aborted on close.

## 8. Supported preview formats

PDF (browser's built-in viewer via `<object type="application/pdf">`, with pages, zoom and download; browsers without an inline viewer, such as most phones, show a "Download to view" fallback instead of an empty panel), JPEG, PNG and WebP (`<img>` at natural size, scaled to fit without distortion). Type is taken from Media's detection, not the file name.

## 9. Tests executed

All locally against PostgreSQL 16 and Redis, then in CI.

- **New HTTP integration test** `TestOnboardingDocumentPreviewContract`: all four formats return exact bytes, type and safe headers with no Media reference leaked; documents, statuses and stages unchanged; 4 audit records; unauthenticated, invalid-token, unknown, malformed and oversized IDs; cross-organization path and document; read-only member can preview but cannot remove; Media quarantine, pending scan, refusal, outage, deletion, checksum mismatch, unsupported type, signature mismatch; removed attachment. A mutation check (disabling Fleet's own readiness gate) makes the test fail as expected.
- **Onboarding browser journey** `TestOnboardingV2BrowserWorkflow` (CI), extended: PDF, PNG, JPEG and WebP previews from `blob:` URLs; Close and Escape; object URL revoked; preview after reload; network failure stays inside the viewer; mobile fit and no overflow; documents unchanged; read-only submitted company sees Preview without Replace or Remove. All 10 journey checks pass (`browser-results.json`), including organization switching without cross-tenant data.
- Auth browser workflow, auth/contact/Section 28 contracts, the CI PostgreSQL auth and organization regression set: pass.
- Full Fleet suite (`go test ./... -p 1`, as in Section 31 mandatory): 1447 passed, 0 failed. Its 10 skips need databases or containers CI provisions (backup restore, Dispatch/Hauling authorities, admin investigations) and are unrelated to this change.
- Frontend: `npm test` 29/29 (9 new tests for page requirements, cache scoping and eviction, preview type allowlist, file names and error messages); `npm run build`; `npm run typecheck`; the exact CI ESLint command (clean; it failed on staging before this change).
- Real-Chromium PDF check: a two-page PDF uploaded through the harness renders in the in-app viewer with page count, thumbnails and zoom (`screens/viewer-pdf-*.png`).

## 10. UI preservation verification

`web/fleet/scripts/onboarding-docs-screenshots.mjs` captured full-page desktop (1440 px) and mobile (390 px) documents pages, empty and with four ready documents, for both revisions against the same API. Per-run values (file reference, date) were masked identically. Results (`visual-regression.json`):

| Capture | Before vs after with Preview removed from layout | Before vs after as shipped |
|---|---|---|
| Empty, desktop | 0 pixels differ | 0 pixels differ |
| Empty, mobile | 0 | 0 |
| Four documents, desktop | **0** | 5,740 pixels, all within the action rows |
| Four documents, mobile | **0** | 5,740 pixels, all within the action rows |

So the only visible change is the Preview control, which uses the existing `btn btn-secondary` class and renders identically to Replace (`screens/before-*.png`, `screens/after-*.png`). The viewer uses existing styles only. No layout, header, sidebar, step indicator, card, typography, color, spacing or button style changed, and no dependency was added.

## 11. Git commits

- `2599593` feat(fleet): read-only company document preview through Media
- `4ed686d` perf(fleet-web): faster onboarding documents step and document Preview
- `45ea321` test(fleet): onboarding documents performance and visual regression harness
- this report and its evidence (documentation only)

## 12. Deployment evidence

- **Backend:** [Deploy staging (automatic) run 43340933612](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43340933612) succeeded. At 00:58:51 UTC both `karrygo-fleet-partner-api` and `karrygo-fleet-partner-worker` reported image `sha256:df053628bf5960053c0dea1ba902b42d26411640e0930746bc2236ac857ecfd0`, revision `45ea3211864bfe6f708138002478b6401ef662b6`, API health `healthy`, after the workflow's pre-deploy database backup. Before deployment the live preview route returned 404 (no such route). From 00:58:59 it returned 401 (route present, authentication enforced).
- **Frontend:** the live `fleet-karryam.vercel.app` documents-step bundle (`/_next/static/chunks/1ipwtka5q3w0f.js`) contains the Preview code (`data-document-preview`), so the Vercel production deployment for `staging` serves this change. The Vercel deployment ID was not read (no Vercel API credential in this session).
- **CI on `45ea321`:** [Fleet authentication v2](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43340933527): success (frontend tests and build, auth browser workflow, full onboarding browser workflow with preview). [Deployment safety acceptance](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43340933543): success. [Section 31 mandatory acceptance](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43340933526): every job succeeded, including `fleet-mandatory` (the full Fleet PostgreSQL/Redis suite with the skip gate), `static-and-contract`, `repository-security-scans`, all 17 `other-go-modules` builds and both `provider-apps`. Only the final aggregator job was still queued when this report was written. The workflows that were already failing on the previous head (Customer V4 certification, Admin bridge) do not cover these paths.

## 13. Live preview results

Live checks on `https://fleet-karryam.vercel.app` that do not need a session (`live-smoke-results.json`): the protected documents step redirects to sign-in on desktop and mobile with no horizontal overflow. The preview endpoint answers 401 `unauthorized` with `no-store` and `nosniff` when unauthenticated, with an invalid bearer token, or with a token in the query string. API round trips measured 149–527 ms.

## 14. Remaining blockers

- **Authenticated live verification is not done.** Signing in to staging needs an email OTP for an authorized staging account, which this session does not have (the same blocker recorded in `fleet-onboarding-final-closure.md`). Still pending on the live URL: authenticated cold-load timing, upload/replace/remove, PDF and image preview on a real company's stored documents, read-only preview, and cross-organization refusal with real sessions. Everything else is complete, and all of it is covered against PostgreSQL and the signed Media test authority. To finish: provide an authorized test inbox OTP (or sign in in a connected browser) and run the same steps.
- Physical Android testing was not performed. On phones without an inline PDF viewer, the viewer shows the "Download to view" fallback, which was verified in the headless shell, which has no viewer either.
- Other steps (company, address, representative, capabilities) still call the full `reload()` after saving. They now overlap their record reads with identity and organization, but targeted refreshes there were out of scope.
- The Section 31 final aggregator job was still queued when this report was written; every job it aggregates had succeeded (section 12).
