# Karryam vendor app

Flutter app under `apps/vendor`, with branded splash, three introductory onboarding pages, passwordless phone/email auth, OTP, conditional vendor choice and recovery states. Android, iOS and web runners are included. The existing backend integration foundation remains available through `lib/vendor.dart`. Business onboarding now includes setup forms, private document uploads, security checks and activation status. The activated-business workspace includes Home, Orders, Catalogue, Finance and More tabs.


## Run the app

```powershell
cd apps/vendor
flutter pub get
flutter run -d chrome
```

For an Android emulator/device, use `flutter run`. iOS requires macOS/Xcode. The app always starts in **mock mode** and makes no server calls in that mode. Use code **123456** with a valid phone/email to test the auth flow. Incorrect codes are rejected.

Tap the **tune icon** in the header to open **Data source**:

- **Mock**: choose multiple vendors, single vendor, expired code, no access, suspended access, connection failure or rate limiting. Selecting a scenario restarts the flow.
- **Live**: enter the explicit Vendor ingress origin, then apply. Phone/email, verification, resend, vendor selection, refresh and logout use the existing exact API contracts. No fallback to mock occurs on errors.

Optionally prefill the live origin:

```powershell
flutter run -d chrome --dart-define=VENDOR_API_ORIGIN=https://your-vendor-ingress
```

The flag prefills the origin; it does **not** change the default mock mode. Live mode uses secure sessions isolated by server origin. Mock tokens stay in memory and are never sent to the live server. Switching modes resets transient input/challenge state; live sessions remain securely stored for the same origin and are validated by refresh before the signed-in screen is shown. The server origin and local tour completion can persist, but the selected mode does not.

Code expiry and rate-limit waits follow server responses. Network retries are explicit; forms preserve the identifier and pending operation. Mock implements the same auth and onboarding contracts with local test fixtures. It never sends those fixtures to Live.

The implementation uses the original transparent logo, locally bundled Inter fonts and custom editorial food/shop photography. Onboarding shares one light canvas, layered compositions and teal primary actions; logo placement is limited to the splash and first intro. Image prompts and asset paths are documented in `design/ASSETS.md`. Layouts adapt to the actual device size and keyboard; OS status/navigation areas remain native.

## Mobile structure

- `lib/main.dart`, `lib/app/vendor_mobile_app.dart`: runnable Flutter shell and data-source controls.
- `lib/app/vendor_runtime.dart`: mode lifecycle, auth navigation, expiry, retry and session restoration.
- `lib/app/data/mock_vendor_client.dart`: local contract-shaped auth and onboarding responses and testing scenarios.
- `lib/features/onboarding/ui/`: intro pages and business onboarding forms.
- `lib/features/auth/ui/`: sign-in, OTP, vendor selection, success, access and connection states.
- `lib/features/settings/`: profile, brands, branches, hours, delivery/preferences, business types, team access and role permissions.
- `lib/core/theme/`, `lib/core/widgets/`: brand tokens and shared components.
- `assets/`: original logo, illustrations/photography and licensed local fonts.

## Configure and initialize

Use the certified Vendor ingress **origin**, without an API path. No guessed host, credentials or default deployment enablement is included.

```dart
import 'package:vendor/vendor.dart';

final vendor = VendorApp(
  config: VendorAppConfig.fromEnvironment(),
);
await vendor.initialize();
await vendor.auth.start(email: email);
final verification = await vendor.auth.verify(otp: otp);
if (verification.requiresVendorChoice) {
  // Present only returned availableVendorIds, then use the same challenge/OTP.
  await vendor.auth.chooseVendor(selectedVendorId);
}
```

Supply `VENDOR_API_ORIGIN` through `--dart-define` or the in-app data-source controls. For integration tests construct `VendorAppConfig(origin: ...)` and inject a MockClient and MemoryVendorSessionStore. Runtime storage defaults to Flutter Secure Storage.

## Feature files

Each implemented contract area has `lib/features/<feature>/data/<feature>_routes.dart`, `<feature>_api.dart`, `models/<feature>_models.dart`, and `state/<feature>_controller.dart`. The composition root exposes each API and controller. Specialized auth, order, pickup and withdrawal workflow classes sit alongside these generic feature load/command controllers.

The generated method names combine the actual HTTP method and path. For example:

```dart
final response = await vendor.financeApi.getFinanceOverview();
final data = response.object; // exact backend data, without invented defaults

final operation = VendorCommand.create(version: orderVersion);
final result = await vendor.ordersApi.postCommerceOrdersVendorOrderIdAccept(
  vendorOrderId: vendorOrderId,
  body: PostCommerceOrdersVendorOrderIdAcceptRequest(),
  command: operation,
);
// Re-read order if required. The result is backend authority, not a local transition.
```

Retain a command object on an identical retry. Body-level `expected_version` and `idempotency_key` remain in the exact DTO; do not move them to headers. `VendorCommand.ifMatch` is only for operations that declare `If-Match`. Commands without header-level idempotency must not receive an invented key header. Request `.fromJson` can represent explicit nulls where a backend nullable field supports them; named constructors omit absent nullable values.

`VendorResponse` retains status, headers, ETag, original envelope and raw `data`. Use `.object`, `.list` or `.decode(Model.fromJson)` according to the actual endpoint. Lists/pagination differ by contract; no global pagination shape is imposed. Empty 204 and accepted 202 responses are supported.

## Coverage and authority

Screen-level coverage and the remaining workspaces are tracked in [docs/frontend-coverage.md](docs/frontend-coverage.md). Generated API coverage is separate from completed app journeys. Order issue reporting is available from **Orders > order details > Order support > Reports and issues**; see [docs/support.md](docs/support.md).

`contracts/vendor_manifest.json` records methods, paths, source contracts/handlers, exact request fields, headers, query parameters and exclusions. The foundation covers all 711 public Vendor operations in the backend Section 43 parity snapshot, plus 11 later source-registered readiness/orchestration routes (724 total, 29 feature areas). Customer checkout/payment/quote approvals, participant decisions, internal HMAC callbacks and platform operator routes are excluded.

Request models use OpenAPI plus actual HTTP-bound Go JSON structs where OpenAPI omits a DTO. Backend enum/domain validation remains authoritative. Response objects are modeled where a schema exists; sparse response contracts stay raw through VendorResponse, rather than adding guessed entity fields. No unresolved top-level mutation DTO is knowingly sent. The source scan is conservative and its extraction is checked by the contract verifier; backend source changes require regeneration and review.

The backend still decides permissions, tenant/branch scope, stock, money, activation, accepted prices, substitutions, delivery and secure custody. There is no guessed role-to-permission table or fabricated current-effective-permissions endpoint. The access controller records returned permission catalogue/denied-action hints; it does not grant access.

Session refresh is serialized and rotates tokens. Stale refresh cannot restore a logged-out session; business/branch state is cleared on identity changes. A permission failure is recorded without silently logging out. Feature controller generations discard old responses after branch changes. Refresh does not discard the selected branch.

There is no automatic retry after timeout/503/429. Unknown financial/custody outcomes require operation read-back before a deliberate retry. Error details retain Retry-After when supplied. Handover credentials are transient arguments, not stored state. Server success/canonical consumption is necessary; no client completion fallback exists.

## Dependencies not callable by this frontend

- Reports: Analytics report/reliability handlers currently authorize HMAC services. A public Vendor facade/auth contract is needed before wiring them.
- Media: product/brand/pastry uploads use new scoped Vendor authorization/status/content routes and Media File Service byte storage. See docs/media.md for scanner/deployment setup. Other evidence domains still use their existing opaque-reference contracts.
- Realtime: existing semantic events inform read-back, but Vendor-compatible public subscription/auth details need confirmation. Fetch APIs are ready; no fabricated WebSocket stream is created.
- Enabled features: relays, readiness/orchestration and integrations depend on backend flags/certified authorities. Client generation does not enable them or prove live availability.
- Owner provisioning/effective access bootstrap: no guessed registration or session-access endpoint is introduced.

See [CONTRACT_COVERAGE.md](CONTRACT_COVERAGE.md) and the [frontend guide](../../services/vendor-service/docs/frontend/VENDOR_FRONTEND_GUIDE.md).

## Regenerate and verify

From repository root:

```powershell
python apps/vendor/tool/extract_backend.py
python apps/vendor/tool/generate_contracts.py
python apps/vendor/tool/verify_contracts.py
cd apps/vendor
dart format lib test
flutter pub get --offline
flutter analyze --no-pub
flutter test --no-pub
```

Generated source-scan files are ignored. Handwritten runtime/workflows are not regenerated. The generator emits routes and DTOs without making backend calls or opening databases.

Validation: see [VALIDATION.md](VALIDATION.md) for clean analysis, 171 passing tests and contract coverage.

## Verification

Run `flutter analyze`, `flutter test`, and `flutter build web`. See `VALIDATION.md` for completed checks and `design/implemented/` for screenshots of the actual Flutter app. Android uses the same Gradle/AGP/Kotlin versions as the customer app. Release signing credentials must be configured before an Android store release.

Business setup and Live upload configuration: [ONBOARDING.md](ONBOARDING.md).


## Appearance

Choose **More > Appearance** for System, Light or Dark. Before signing in, Appearance is also available in the Data source sheet. System follows the phone setting; an explicit choice is saved securely and restored on launch. Changing appearance preserves the current route and Mock/Live state. Dark mode uses layered slate surfaces, teal actions and readable status colors across the existing screens.

Filter selections and archived-record menus use neutral adaptive surfaces. Custom catalogue, order, settings, wallet and intro illustrations repaint with the active palette. Success feedback uses neutral surfaces with teal accents and smooth entry/exit motion; reduced-motion settings are respected. Adaptive colors interpolate during theme changes instead of switching abruptly. Pale decorative backgrounds become slate in Dark mode, including the sign-in method group; catalogue screens use the charcoal canvas.

Mock sign-in prefills `owner@example.com` and `+2348012345678` while preserving an existing identifier. Live sign-in does not receive dummy values.

## Dashboard preview

Open Data source, select Mock business state **Activated business**, choose a Mock dashboard state, apply and sign in with **123456**. Available workspace fixtures: active, empty, restricted finance, unavailable orders, unavailable Finance, transfer verification required, settings access limited and settings unavailable. Mock remains the launch default.

Live routing requires onboarding `ACTIVATED` and business `ACTIVE` or `PAUSED`, allowing a paused business to reach Resume. Activation requests re-read these authoritative states before opening the workspace. Home shows wallet-authoritative available/pending balances and a scoped queue from the latest 50 orders, rather than invented daily sales or analytics. Finance is business-wide; orders follow the selected branch. Food menus and Shop products use their existing read contracts. Resource errors stay separate and never fall back to demo data.

Navigation and Home are implemented, together with the supported order actions, core and advanced catalogue configuration, food/Shop price previews, identifier lookup and all six public Finance routes. Shop inventory now includes branch stock, new stock tracking, receiving/reversal, branch transfers and arrivals, quarantine/release/disposal, reorder rules, adjustments, stocktakes, history and alerts. Business settings now include supported profile, brand, branch, business-type, staff and role workflows. Additional operational workspaces remain subsequent increments. See [Orders](docs/orders.md), [Catalogue](docs/catalogue.md), [Finance](docs/finance.md), [Business settings](docs/settings.md) and [Inventory](docs/inventory.md) for implemented flows and backend limitations.

The dashboard header uses the original Karryam monogram with a readable wordmark. Branch switching lives in More; the current branch remains visible beside order views. The header bell opens a recent Order activity sheet sourced from the same public order reads. It is not a notification inbox and has no fabricated unread count. Catalogue entries open public menu detail/sections or product detail/variants reads, including retry and permission states. Tabs and filters use explicit teal foregrounds and pale teal selection backgrounds.

Preparation and vendor delivery navigation and contract boundaries: [readiness-delivery.md](docs/readiness-delivery.md).
