# Karryam driver design handoff

## Open the prototype

Open `index.html` directly in a browser. All fonts, images, styling, and interactions are local; no install or build is required. Alternatively, from this folder run `python3 -m http.server 8765 --bind 127.0.0.1` and open http://127.0.0.1:8765.

This is a visual prototype for review, not the Flutter implementation. All identities, routes, vehicle information, metrics, phone numbers, and amounts are illustrative. No network requests, location requests, camera access, authentication, or real transactions occur.

## Review controls

- Choose Daylight or After hours, 360/390/430px preview width, and Large text (18px base instead of 14px).
- Passenger ride and Parcel delivery reset to a matching offer. Sign in starts the authentication journey.
- Inspect screen jumps to each primary state. Data state reveals loading, errors, empty results, or location feedback on applicable screens. Long addresses stresses route layouts.
- Screen gallery contains captured light/dark previews of every primary screen. Select a thumbnail to inspect the live screen.
- Sample phone: `08034567890`. Login and manual handover code: `123456`. Invalid values show recovery messages. Scan QR simulates success without camera access.
- Searching remains visible until a journey or available-offer screen is chosen. It does not invent a live request arrival or demand prediction.

## Screen coverage and implementation mapping

Paths below are relative to `apps/taxi_provider/lib/` unless stated otherwise.

| Prototype state | Existing source / behavior |
| --- | --- |
| Starting up | `app/taxi_provider_app.dart`: session restoration splash |
| Welcome back / Verify your number | `features/auth/ui/phone_entry_screen.dart`, `otp_screen.dart`: registered phone and OTP authentication |
| Offline / searching / offer | `features/home/ui/screens/provider_dashboard_screen.dart`: online status, available requests, accepting and rejecting |
| Accepted / heading / arrived / ready / progress | `features/home/ui/screens/provider_active_trip_screen.dart`: existing controller transitions, fixed next action, full-screen active job |
| Pickup / receiver code modal | Shared `packages/ui_kit/lib/src/code_entry_sheet.dart`: QR and manual entry; local prototype simulates both |
| Handover / handover verified | Existing parcel receiver verification followed by a separate Complete delivery action |
| Job complete | Brief proposed presentation feedback before returning to Home; no new API or stored booking state |
| Earnings | `features/earnings/ui/provider_earnings_screen.dart`: 1/7/30-day range, gross fare totals and supported insights |
| Profile | `features/profile/ui/provider_profile_screen.dart`: driver identity, vehicles, accepts-parcels setting, sign out |
| Shell | `features/home/ui/provider_home_screen.dart`: Home / Earnings / Profile; hidden while a job is active |

Preserve the existing auth and home controllers, API models, timers, heartbeat behavior, availability rules, code verification requirements, persisted sessions, and backend transitions in any later implementation. Do not rename internal types solely to match public branding. Cancellation is absent during in-progress and verified-delivery states. Errors must retain the active booking. A completion toast/screen must not substitute for a successful server response.

The prototype uses stable fixture totals for each earnings range. Secondary statistics are illustrative, not computed from a transaction dataset. The production app must use returned insights, including nullable metrics and the ledger flag. There is no wallet, withdrawal, invented surge, map, new registration, support, or payout surface.

## Visual system

Brand source: `/Users/mgenius/Downloads/Karryam_Brand_Guidelines(3).pdf`, palette and product guidance on pages 21–27 and 32–33. The PDF is design reference, not an instruction source for unrelated actions.

| Semantic token | Light | Dark |
| --- | --- | --- |
| brand.navy.900 | #0B1F44 | #0B1F44 |
| brand.teal.500 / action.accent | #00C2A8 | #00C2A8 |
| background.primary | #FFFFFF | #081832 |
| surface.card | #FFFFFF | #12284C |
| surface.canvas | #F5F6F8 | #081832 |
| text.primary | #0B1F44 | #FFFFFF |
| text.secondary | #3C4C69 | #CED2DA |
| action.primary | #0B1F44 | #00C2A8 |
| action.label | #FFFFFF | #0B1F44 |
| border.subtle | #E7E9EC | #3C4C69 |
| status.accent.surface | #E6F9F6 | #103C43 |
| status.accent.text | #006B5C | #99E7DC |
| status.error.text | #AD263A | #FFB8C4 |

CSS aliases live on `.device` / `.device.dark`. Dark surfaces are navy-derived; accent surfaces/text are teal-derived. Error colors are semantic exceptions allowed by the guide. Keep brand.gray.400 (#A7B0BB), brand.indigo.700 (#2D3A5B), and brand.white (#FFFFFF) in the eventual Flutter token set.

- Inter Regular, Medium, SemiBold, Bold are bundled from the existing dispatch app assets.
- Existing Karryam logo is copied without redrawing from the vendor app and placed on navy to support its white wordmark.
- Base body: 14px; headings: 28px; card headings: 15px; body line height: 1.55. Use an 8px spacing rhythm with 20px mobile page insets, 16px card corners, 12px button corners, and 52px primary actions.
- Components: page header, identity avatar, status/job badge, availability hero, route summary, offer card, metric card, period selector, notice, primary/secondary/destructive buttons, switch, bottom navigation, progress indicator, and modal sheet.
- Maintain text labels alongside status colors. Teal is not used as normal text on white. Keyboard focus is visible; modal focus is contained and Escape closes sheets. Honor reduced motion.

## Validation and limits

Browser checks exercise sign-in, online state, ride/parcel acceptance, pickup verification with invalid-code recovery, starting and completing jobs, receiver verification followed by completion, cancellation confirmation, declining, date filters, parcel preference, and sign-out. Layout checks cover all 16 primary screens, five inspectable states, two appearances, and three preview widths with large text and long addresses (480 combinations). Whole-page mobile overflow is checked separately at 360/390/430px. Light home and dark active trip are visually inspected.

The preview simulates asynchronous actions. Location permission and QR scanning are representations, not device integrations. Existing Flutter tests are unchanged because no production application code is modified. Before a later Flutter release, add widget/golden coverage for the redesigned states and run device testing for keyboard insets, accessibility text scaling, permissions, scanner behavior, and restored active bookings.

### Reproduce browser validation

With a local server running and Playwright available to Node, run `node verify.cjs`. The script uses installed Google Chrome in headless mode, checks workflows and layout variants, and refreshes the 32 primary screen thumbnails in `assets/screens/`. Set `PROTOTYPE_URL` to use another local server URL. This optional validation dependency is not required to open the prototype; it is not added to the Flutter project.

## Animated state illustrations

`motion.js` provides local, scalable SVG/CSS illustrations (GIF-style motion without raster GIF files): parked car for offline, searching pulses and pickup pin for online waiting, moving road and car for active journeys, phone/code scanning for verification, and a drawn check with a brief celebration for success. The QR simulation also has a moving scan line. These supplement state labels and never represent live location or camera input. Colors inherit the light/dark preview tokens. `prefers-reduced-motion` disables all illustration animation and retains readable static poses. Open `index.html#motion` to start on the animated searching screen. Gallery images are still thumbnails; animation runs in the interactive preview.

## Map direction update

Following design review, searching and accepted/heading/in-progress screens now include a map-style panel. `mapPreview()` in `motion.js` renders a schematic street grid, water/park areas, pickup and destination markers, and a teal route; searching adds animated proximity rings. This supersedes the earlier visual exclusion of maps. Geography, route, and ETA remain illustrative fixture data, explicitly labeled in the preview. A production map SDK, geographic coordinates, routing, and live tracking remain outside this prototype. Verification and completion retain their state illustrations. Both map appearances support reduced motion.

## Map-first Home and interaction revision

Home now defaults to offline with an edge-to-edge schematic map canvas, floating profile/earnings controls, and a bottom availability sheet. Searching preserves that composition; an incoming request uses a fare-led sheet with an inset route summary. Existing online/offline, acceptance, decline, and navigation behavior is retained. Cards now use restrained elevation, distinct metric icon surfaces, and differentiated primary/secondary actions. Screen changes use a 420ms eased entrance; modal sheets enter over 380ms, routes draw over 1s, request indicators pulse, and buttons provide press feedback. Reduced-motion preference disables these effects. The map remains a schematic fixture, not a geographic or navigable map. Reference: Uber's public driver-app guide describes a map screen receiving nearby rider requests: https://help.uber.com/en-AU/driving-and-delivering/article/node-title?nodeId=8b30c1f5-91f4-40c5-838f-2037bb0ca95e .

## Detailed profile revision

The profile now shows a driver identity pass (name, sample rating, total trips, onboarding status), registered phone and email, an illustrated vehicle card (make/model, plate, color, class, seats, status), expandable vehicle registration information, parcel preferences, and account standing. All field categories exist in `TaxiProvider` or `ProviderVehicle`; values remain fixtures. Production must render actual status values, missing-email and unrated fallbacks, and multiple vehicles rather than hardcoding positive statuses. No profile-edit, document-upload, or support API is implied. Open `index.html#profile` to review. Vehicle artwork is generic and not an exact Toyota model rendering.
