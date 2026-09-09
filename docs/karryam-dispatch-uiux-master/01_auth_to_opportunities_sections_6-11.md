# 6. Launch, Authentication and Session

Classification: KEEP & REDESIGN

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| AUTH-01 | Splash / launch | Brand launch and initialization | session check, loading, offline launch |
| AUTH-02 | Session restoration | Restore saved session and profile | valid session, refresh needed, refresh failed |
| AUTH-03 | Welcome / onboarding intro | Explain provider value and flow | first run, returning user |
| AUTH-04 | Sign in | Passwordless identifier entry | phone/email, validation, account not found |
| AUTH-05 | Create account | Phone + email signup entry | validation, duplicate account, network failure |
| AUTH-06 | OTP verification | Verify login/signup challenge | sending, sent, incorrect, attempts remaining, expired |
| AUTH-07 | OTP resend | Resend challenge safely | cooldown, resent, limit reached |
| AUTH-08 | Account suspended / restricted | Block auth completion where applicable | support CTA, reason-safe messaging |
| AUTH-09 | Logout confirmation | End session intentionally | confirm, processing, failure |

# 7. Onboarding and Provider Setup

Classification: KEEP & EXPAND

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| SET-01 | Account type | Choose provider/account context | selected/unselected, info state |
| SET-02 | Service type | Choose dispatch service where still applicable | verify whether redundant in dispatch-only build |
| SET-03 | Operation mode | Individual vs fleet/organization | individual, fleet-linked, explanation |
| SET-04 | Profile details | Name, location, contact and onboarding fields | validation, saved draft, submit failure |
| SET-05 | Government ID setup | Collect ID type/number/document | camera/file choice, validation |
| SET-06 | Profile/selfie photo | Capture provider image | camera permission, preview, retake |
| SET-07 | Bike/vehicle type | Choose operational vehicle class | motorcycle, dispatch bike, scooter, tricycle, bicycle, electric bike |
| SET-08 | Driver information | Licence + guarantor/emergency/vehicle registration handoff | multi-step validation |
| SET-09 | Final submission progress | Submit staged onboarding data | identity, face, licence, vehicle upload progress |
| SET-10 | Verification pending | Show current review progress | pending, partially approved, action required |

# 8. Identity, Face, Licence and Readiness Verification

Classification: KEEP & MAJOR EXPANSION

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| VER-01 | Verification hub | Canonical readiness overview | not started, in progress, pending review, verified, rejected, suspended |
| VER-02 | Readiness progress | Show completed vs required steps | step counts, blocker CTA |
| VER-03 | Identity intro | Explain government-ID requirement | not started, rejected re-entry |
| VER-04 | ID type selector | Choose supported ID type | selection, unavailable type |
| VER-05 | ID details | Enter document number/data | validation, duplicate/invalid response |
| VER-06 | ID capture/upload | Capture or upload ID | permission, capture, preview, upload, failure |
| VER-07 | Identity submitted | Confirm review submission | pending, approved, rejected |
| VER-08 | Face-check intro | Explain selfie/face match | privacy copy, camera permission |
| VER-09 | Face camera | Capture face | not detected, too dark, multiple faces, capture |
| VER-10 | Face processing | Match/process selfie | processing, success, failed match, manual review |
| VER-11 | Licence details | Licence number + expiry | validation, expired date |
| VER-12 | Licence document | Capture/upload licence | preview, upload, failure |
| VER-13 | Licence status | Review outcome | pending, approved, rejected, expiring soon, expired |
| VER-14 | Guarantor | Create/edit guarantor | not started, saved, rejected/action required |
| VER-15 | Emergency contact | Create/edit emergency contact | saved, edit, remove confirmation |

# 9. Vehicle and Vehicle Documents

Classification: KEEP & EXPAND

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| VEH-01 | Vehicle hub | Current vehicle + verification state | none, pending, verified, rejected, suspended |
| VEH-02 | Vehicle type | Select allowed vehicle class | all supported types |
| VEH-03 | Vehicle details | Brand/model/year/color/plate | validation, optional engine/chassis |
| VEH-04 | Vehicle review | Review before create/update | submit, processing, backend validation |
| VEH-05 | Documents hub | Registration/insurance status | missing, submitted, pending, verified, rejected, expired |
| VEH-06 | Registration upload | Upload vehicle registration | camera/file, preview, progress, failure |
| VEH-07 | Insurance upload | Upload insurance + expiry where required | preview, date, progress, rejection |
| VEH-08 | Vehicle rejected | Explain rejection + remediation | reason, replace doc/details |
| VEH-09 | Vehicle suspended | Operational restriction state | cannot go online, support/resolve CTA |

# 10. Home, Availability and Location

Classification: KEEP & MAJOR REDESIGN

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| HOME-01 | Home offline | Default operations dashboard | offline, readiness summary, daily stats |
| HOME-02 | Go-online readiness check | Check verification/vehicle/licence/fleet gates | each blocker type |
| HOME-03 | Location permission education | Explain precise location need | before system prompt |
| HOME-04 | Background location | Explain continuous availability tracking | allow, denied, settings recovery |
| HOME-05 | Online & available | Searching for opportunities | session duration, trips today, zone/location |
| HOME-06 | Busy / active mission | Home while mission active | persistent active mission CTA |
| HOME-07 | Location degraded | GPS accuracy/staleness issue | stale, unavailable, recovered |
| HOME-08 | Session metrics | Hours online / trips today | loading, empty, refresh |

# 11. Opportunities and Assignment Sources

Classification: KEEP + NEW

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| OPP-01 | Opportunity inbox | Unified opportunity list | loading, empty, multiple sources |
| OPP-02 | Direct request detail | Legacy/direct dispatch opportunity | fare, pickup/dropoff, package, receiver privacy-safe details |
| OPP-03 | Assignment offer | Ranked/targeted assignment offer | rank, expiry, purpose, expected version |
| OPP-04 | Organization offer | Organization marketplace offer | source identity; no fake fare when absent |
| OPP-05 | Company assignment | Company-created assignment | organization/company context, accept/decline |
| OPP-06 | Accepting offer | Process accept idempotently | processing, success, network retry |
| OPP-07 | Decline reasons | Collect allowed decline reason | too far, busy, other where applicable |
| OPP-08 | Offer no longer available | Expiry/conflict/accepted elsewhere | refresh canonical state |
