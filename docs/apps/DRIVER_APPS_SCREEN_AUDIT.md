# Driver apps: screen audit and live-test findings

Audit of the two driver apps against what their servers already support, plus
everything found while testing both apps end to end on the live staging server
(`https://api-gateway.staging.karryam.com`, 18 October 2026).

- **Taxi driver app** (`apps/taxi_provider`): **7 screens.**
- **Truck driver app** (`apps/truck_provider`): **46 screens.**

How to read the tables:

- **Server ready**: the endpoint already exists; only the app screen is missing.
- **Needs server**: the screen also needs new or changed backend work.

---

## 1. Taxi driver app: missing screens

The taxi app has only: phone entry, code entry, home/dashboard, active trip,
earnings, and a read-only profile. A brand-new taxi driver **cannot get from
sign-up to their first ride using the app alone**, and has no history, wallet,
notifications or support.

### 1.1 Sign-up and onboarding (blocks every new driver)

| Screen | What it does | Status |
|---|---|---|
| Account type / service choice | Choose ride, parcel or both | Server ready (`PUT /provider/profile`, `PUT /provider/parcel-preference`) |
| Personal details | Name, email, state, city, language | Server ready (`PUT /provider/profile`) |
| Driver licence | Licence number and expiry | Server ready (`PUT /provider/profile`) |
| Documents upload | Government ID, licence, vehicle registration photos | Needs server: uploads must go through a public route (see 3.3) |
| Profile photo | Driver photo | Needs server (same upload route) |
| Add vehicle | Class, plate, seats, year, make, model, colour | Server ready (`POST /provider/vehicles`, `GET /provider/vehicle-classes`) |
| Guarantor and emergency contact | Safety contacts | Server ready (`PUT /provider/profile`) |
| Verification pending / status | Shows identity, vehicle and document review progress and why the driver can't go online yet | Needs server: no endpoint exposes the 3 review statuses with reasons, and **no admin tool exists to approve drivers** |
| Service area check | Tells the driver up front whether their city is covered, instead of only when they tap Go online ("We do not operate in this area yet") | Needs server: today the reason only comes back from `PUT /provider/availability` |

### 1.2 Trips

| Screen | What it does | Status |
|---|---|---|
| Trip history (Past / Cancelled) | List of completed and cancelled rides | Server ready (`GET /provider/bookings`) |
| Trip detail | Route, fare breakdown, passenger rating, receipt | Server ready (`GET /provider/bookings/:id`, `GET /provider/bookings/:id/review`) |
| Scheduled / upcoming rides | Rides booked for later | Needs checking: server supports scheduled bookings; driver-side list not confirmed |
| Ratings and reviews received | Driver's rating and passenger comments | Server ready (`GET /provider/bookings/:id/review`); summary endpoint needs server |
| Cancel trip with reason | Reason picker when cancelling | Server ready (`PUT /provider/bookings/:id/cancel`) |
| Parcel (goods) delivery trip | Receiver code, proof of delivery | Server ready (`verify-delivery`, `delivery-code`) |

### 1.3 Earnings and wallet

The taxi app shows earnings but has **no way to get money out**. The truck app
already has all of these screens, and they work against the same
payment-wallet service.

| Screen | Status |
|---|---|
| Wallet balance (available / pending) | Server ready (payment-wallet `GET /provider/earnings`) |
| Transaction list and detail | Server ready |
| Withdraw money (amount, confirm, PIN, processing, receipt) | Server ready (payment-wallet `/provider/withdrawals`, `/provider/pin`) |
| Bank accounts (add, resolve name, set default, delete) | Server ready (payment-wallet `/provider/bank-accounts`) |
| Transaction PIN setup / change | Server ready (payment-wallet `/provider/pin`) |

### 1.4 Notifications

| Screen | Status |
|---|---|
| Notification feed (list of past alerts, unread state) | Server ready (`GET /provider/notifications`) |
| Live updates while the app is open (websocket) | Server ready (`POST /provider/notifications/realtime-token`). The taxi app only polls. |

Push to the phone works: the app registers its device, and the server now
delivers (fixed on 18 October, see 3.1).

### 1.5 Support and safety

| Screen | Status |
|---|---|
| Help / FAQ | Server ready (support-dispute `GET /support/faqs`) |
| Report a problem / disputes (list, detail, chat) | Server ready (support-dispute `/taxi-provider/complaints`); the truck app has these screens to copy |
| Live chat with support | Server ready (support-dispute support-chat) |
| SOS / emergency button during a trip | Server ready (support-dispute `POST /taxi-provider/sos`) |
| Safety and emergency contacts | Server ready (`PUT /provider/profile`) |

### 1.6 Profile and account

| Screen | Status |
|---|---|
| Edit profile (name, email, photo, language) | Server ready (`PUT /provider/profile`); photo needs the upload route |
| Change phone number | Server ready (`/provider/phone/change/start`, `/verify`) |
| Vehicle management (list, edit, switch active vehicle) | Server ready (`/provider/vehicles`) |
| Settings (preferences) | Server ready (`GET/PUT /provider/settings`) |
| Security (PIN, sessions) and Privacy | Needs design; PIN is server ready |
| Delete account | Needs server |

### 1.7 Fleet / organisation drivers

| Screen | Status |
|---|---|
| Organisation offers (accept / decline joining a fleet) | Server ready (`/provider/organization-offers`) |
| Company assignments (accept / decline fleet jobs) | Server ready (`/provider/company-assignments`) |

### 1.8 Sign-in

| Screen | Status |
|---|---|
| Email sign-in | **Done 19 Oct** |
| Google / Apple sign-in | Needs server |

---

## 2. Truck driver app: missing or unfinished screens

The truck app is much more complete. What remains:

### 2.1 Placeholders ("Coming soon" in the app)

| Where | Item | Status |
|---|---|---|
| Sign-in screen | Continue with Google | Needs server |
| Sign-in screen | Continue with Apple | Needs server |
| Profile | Security | Needs design (PIN change is server ready) |
| Profile | Privacy | Needs design |
| Profile info | Remove photo | Server ready (`PUT /provider/profile`) |
| Earnings | View all transactions | Server ready |
| Earnings, Earning summary | "Go to Trips" | App only: link to the existing Trips tab |
| Earning summary | Year filter | Needs server: earnings endpoint has no year parameter |
| Transaction detail | Get support | App only: link to the existing disputes screens |
| Withdrawal receipt | Download receipt | App only (generate a PDF or share image) |

### 2.2 Missing screens

| Screen | What it does | Status |
|---|---|---|
| Delivery complete confirmation | After "Confirm" the app jumps straight to "Available requests" with no summary of the finished job and earnings | App only |
| Ratings and reviews received | Customer ratings and comments | Server ready (`GET /provider/bookings/:id/review`) |
| Help / FAQ | | Server ready (support-dispute `GET /support/faqs`) |
| SOS / emergency during a trip | | Server ready (support-dispute `POST /provider/sos`) |
| Fleet company assignments | Accept / decline fleet jobs | Server ready (hauling `/:assignmentId/accept`, `/decline`) |
| Trip recovery / handover to a replacement truck | When a truck breaks down mid-trip | Server ready (hauling `/recovery/bookings/:id/...`) |
| Truck capabilities | Declare what each truck can carry (cargo types, dimensions) | Server ready (`/provider/trucks/:id/capabilities`, `/eligibility`) |
| Delete account | | Needs server |

### 2.3 Visual-only features (screen exists, nothing behind it)

- **Face verification**: screen only, no face-match service.
- **Live chat** in the profile's Support screen: now real (support-dispute), but agents reply from the admin API, as there's no admin console yet.

---

## 3. Bugs and gaps found while testing on the live server

### 3.1 Fixed during the test (server)

- **No notifications were ever delivered for rides, truck bookings or deliveries.**
  Taxi and hauling put notifications on a queue in their own Redis, but
  notification-service reads its own. Fixed: notification-service's Redis now
  has a password and is in its compose file, and taxi and hauling point at it.
- **Notification templates were missing** for every ride, booking, delivery,
  payment and withdrawal event (only 19 vendor templates existed). Fixed: the
  platform's 42 templates are loaded.
- Result: all 21 events of a full truck job were delivered, and a **push
  notification appeared in the phone's tray** with the app in the background
  ("New haulage request").

### 3.2 App bugs

All of these were fixed on 19 October (commits `55c659a5`, `0ab2339b`, `84f32010`), except the gesture-area button, which only exists in the redesigned build that is not in this repository.

| App | Bug | Impact |
|---|---|---|
| Taxi | When the app is backgrounded for ~90 s, the server marks the driver offline, but the app still shows "You're online" and never recovers | Driver silently stops receiving rides |
| Taxi | Trip screens show only the words "Pick-up" and "Destination", never the actual addresses | Driver doesn't know where to go |
| Taxi | Map shows "Map unavailable" (the same Google key works in the truck app) | Probably the key is restricted to other app IDs in Google Cloud |
| Truck | After a customer cancels an active trip, the app stops showing new requests until restarted (2 offers timed out unseen) | Driver silently misses jobs |
| Truck | Pickup code screen: the six digit circles under the QR are always empty | Customer can't type the code if they can't scan |
| Truck | After the customer verifies the driver, the screen doesn't update | Driver doesn't know verification succeeded |
| Truck | Trip card shows the **receiver's** name in place of the customer | Confusing; driver may call the wrong person |
| Truck | Code screen shows the number as "+234 0803…" (keeps the leading 0) | Cosmetic |
| Truck | "Confirm cargo pickup" button sits in the phone's bottom gesture area on smaller screens | Easy to mis-tap |
| Truck | Delivery screen says "Trip Completed:" before the trip is completed | Cosmetic |

### 3.3 Server and platform gaps

| Gap | Impact | Fix |
|---|---|---|
| **No SMS provider** | Login codes for phone numbers are never delivered; drivers can't sign in without debug mode | Add an SMS provider to notification-service |
| ~~Driver email login codes are never sent~~ | **Fixed 19 Oct:** hauling and taxi now email codes through notification-service; the taxi app has "Use email address instead" | |
| ~~Document uploads go straight to media-file-service~~ | **Fixed 19 Oct (truck):** the app uploads to hauling `POST /provider/uploads` as the signed-in driver; hauling checks the file and stores it under its own credentials. The taxi app has no upload screens yet (stage 3) | |
| **No admin tool to approve drivers** | Taxi drivers can never go online without a database edit | Admin review screens (admin-backoffice) |
| **Receiver must be reachable by email or a KarryGo account** | Truck bookings that give only a receiver phone number get stuck at pickup ("Receiver requires a supported verified channel") | Add SMS, or let the customer app require a receiver email/account |
| Only the Lagos taxi zone exists on staging | Taxi drivers elsewhere can't go online | Add zones per city |
| `HAULING_ROUTES_API_KEY` / `TAXI_ROUTES_API_KEY` not set | Routes are straight lines, not roads | Add a Google Routes key |

---

## 4. Suggested order

1. **Unblock sign-up and login**: SMS provider, driver email codes, safe document uploads, admin approval tool.
2. **Fix the silent-failure bugs**: taxi offline drift, truck stuck after cancellation.
3. **Taxi app parity with the truck app**: onboarding, trip history, wallet and withdrawals, notification feed, support and SOS (all server ready).
4. **Polish**: truck "coming soon" items, delivery-complete screen, display bugs.
