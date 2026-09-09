# 24. Notifications

Classification: KEEP & EXPAND

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| NOT-01 | Notification list | Provider notification feed | loading, empty, unread/read |
| NOT-02 | Notification detail / deep link | Resolve notification into destination | opportunity, mission, wallet, support |
| NOT-03 | Push permission | Explain and request notifications | denied, settings recovery |
| NOT-04 | Realtime unavailable | Websocket fast path unavailable | silent polling fallback, non-alarming UI |
| NOT-05 | Stale notification | Target no longer current | refresh canonical state |

# 25. Account, Profile and Preferences

Classification: KEEP & REDESIGN

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| ACC-01 | Account home | Profile + readiness + settings entry | verified/restricted states |
| ACC-02 | Personal information | View/edit profile data | save, validation, failure |
| ACC-03 | Avatar/photo | View/update provider image | permission, preview, upload |
| ACC-04 | Verification shortcut | Open readiness/verification hub | action required badge |
| ACC-05 | Vehicle shortcut | Open vehicle hub | status badge |
| ACC-06 | Notification preferences | Push/SMS preferences | save, failure |
| ACC-07 | Language / appearance | Language + dark mode boolean | do not invent unsupported system-mode persistence |
| ACC-08 | Delete account | Destructive account deletion | warning, final confirmation, processing, logout |

# 26. Phone, Privacy and Security

Classification: KEEP + VERIFY LEGACY

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| SEC-01 | Change phone start | Enter new phone number | validation |
| SEC-02 | Change phone OTP | Verify new phone | incorrect, expired, resend |
| SEC-03 | Phone changed | Confirm successful update | return to account |
| SEC-04 | Privacy information | Explain data/location/custody use | static content backed by policy copy |
| SEC-05 | Security settings | Transaction PIN + account security entry | only supported controls |
| SEC-06 | Legacy password/email-verification decision | Do not redesign as final until backend owner confirmed | current auth is OTP/passwordless; mark legacy/review |

# 27. Support Cases and Evidence

Classification: CONFIRMED - KEEP & EXPAND

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| SUP-01 | Help & support home | FAQs, categories, cases, contact options | loading/error |
| SUP-02 | Support categories | Backend-provided categories | loading, empty/error |
| SUP-03 | FAQs | Backend-provided FAQs | search/filter optional only if locally implemented |
| SUP-04 | My cases | Complaint/case list | empty, open, waiting, resolved |
| SUP-05 | Create support case | Subject, description, booking reference | validation, submit |
| SUP-06 | Case detail | Case status and messages/evidence | open, waiting, resolved |
| SUP-07 | Add support evidence | Attach evidence to case | upload, accepted, failed |
| SUP-08 | Case resolved | Show outcome and history | reopen only if backend supports; otherwise no invented action |

# 28. Support Chat

Classification: CONFIRMED - KEEP

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| CHAT-01 | Start support chat | Begin support conversation | starting, failure |
| CHAT-02 | Conversation | Message thread | loading, unread agent messages |
| CHAT-03 | Send message | Compose/send provider message | sending, failed, retry |
| CHAT-04 | Read state | Mark agent messages read | sync silently, preserve canonical thread |

# 29. Safety and SOS

Classification: CONFIRMED BACKEND - NEW PROPER UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| SOS-01 | Safety hub | Emergency contact, safety guidance, SOS access | always accessible from account/support |
| SOS-02 | SOS entry | Explain emergency action | large target, accidental-tap protection |
| SOS-03 | SOS context | Collect minimum necessary context | reason/location where backend accepts |
| SOS-04 | SOS confirmation | Final emergency confirmation | do not add friction that blocks genuine emergency use |
| SOS-05 | SOS sending | Transmit emergency signal | offline/failure treatment |
| SOS-06 | SOS submitted | Authoritative confirmation | next safety instructions/support contact |

# 30. Global Sheets, Dialogs and Microflows

Classification: REQUIRED SHARED UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| GLB-01 | Decline opportunity sheet | Reason selection | source-specific allowed reasons |
| GLB-02 | Cancel trip sheet | Backend cancel reasons | rider unavailable, package not ready, wrong address, customer unreachable, safety concern, other |
| GLB-03 | Navigation choice sheet | Open supported navigation provider | no background surprise |
| GLB-04 | Permission blocked sheet | Explain settings recovery | location, camera, notifications |
| GLB-05 | Document source sheet | Camera vs file/photos | permission-aware |
| GLB-06 | Credential resend sheet | Confirm/resend protected credential | cooldown visible |
| GLB-07 | Parcel issue sheet | Scan/problem resolution | wrong parcel, damaged/attention path only if backend action exists |
| GLB-08 | Evidence options sheet | Capture/replace evidence | respect evidence session and supersedes rules |
| GLB-09 | Recovery option sheet | Choose structured recovery action | backend options only |
| GLB-10 | Reroute reason sheet | Select reroute reason | road closure, safety, provider update |
| GLB-11 | Offline queue sheet | Explain queued commands | saved locally vs confirmed |
| GLB-12 | Conflict refresh sheet | Explain server state changed | refetch and continue |
| GLB-13 | GPS degraded sheet | Location stale/unavailable | restore permission/GPS |
| GLB-14 | Logout confirmation | Confirm logout | preserve unsynced warning if applicable |
| GLB-15 | Delete account confirmation | Destructive confirmation | irreversible copy |
| GLB-16 | Financial confirmation | Bank/withdrawal critical confirmation | no invented fees/rates |
| GLB-17 | Support evidence attachment | Attach/remove evidence | upload states |
| GLB-18 | Generic error sheet | Recoverable backend/network error | retry, refresh, cancel as context allows |
