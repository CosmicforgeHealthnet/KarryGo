# 12. Activity, Trips and History

Classification: KEEP & REDESIGN

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| ACT-01 | Activity hub | Segment provider work | active, upcoming where supported, completed, cancelled |
| ACT-02 | Trip list | Historical trips | loading, empty, filters |
| ACT-03 | Active trip card | Resume canonical active work | status, next action |
| ACT-04 | Trip detail | Timeline + pickup/dropoff/package | fare only when provided |
| ACT-05 | Completed trip detail | Outcome, proof/recipient confirmation | rating CTA where supported |
| ACT-06 | Cancelled trip detail | Reason and final state | provider/customer/system cancellation |
| ACT-07 | Rate customer | Post-completion rating | stars, tags/comment where contract allows |

# 13. Mission Core - Pickup

Classification: NEW DEPTH ON EXISTING FLOW

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| MPU-01 | Mission workspace | Canonical mission shell | state/version, allowed actions, route, active stop |
| MPU-02 | Navigate to pickup | Route to pickup | ETA/distance, reroute alert |
| MPU-03 | Arrived at pickup | Confirm arrival | processing, success, failure |
| MPU-04 | Pickup instructions | Sender/package/stop instructions | privacy-safe contact actions |
| MPU-05 | Pickup credential | Credential retrieval/display where legacy/allowed | loading, active, expired, resend |
| MPU-06 | Pickup parcel checklist | Manifest/parcel readiness | unscanned, scanned, attention |
| MPU-07 | Pickup evidence gate | Capture required pickup evidence | session, capture, submit |
| MPU-08 | Start mission | Transition into in-progress state | processing, source-version conflict |

# 14. Mission Core - Delivery

Classification: NEW DEPTH ON EXISTING FLOW

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| MDL-01 | Mission in progress | Delivery navigation state | map + minimal driver controls |
| MDL-02 | Approaching destination | Prepare delivery handover | next required action |
| MDL-03 | Arrived at delivery | Confirm delivery arrival | processing, location issue |
| MDL-04 | Recipient details | Delivery contact/instructions | privacy masking rules |
| MDL-05 | Delivery verification entry | Choose/perform required secure handover | OTP/QR according to canonical state |
| MDL-06 | Recipient confirmation | Receiver name/phone/code where required | validation, confirmed |
| MDL-07 | Delivery evidence gate | Capture photo/signature/evidence | required items and completion state |
| MDL-08 | Complete mission | Authoritative completion request | processing, conflict, failure |
| MDL-09 | Mission completed | Success + next navigation | trip summary, rate customer |

# 15. Multi-stop Missions, Manifest and Parcels

Classification: NEW REQUIRED UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| MS-01 | Mission stop list | Ordered stops + progress | active/completed/upcoming |
| MS-02 | Mission map view | Route across stops | list alternative required |
| MS-03 | Stop detail | Stop-level allowed actions | arrival, evidence, complete |
| MS-04 | Manifest summary | Manifest ref/version/status | version mismatch state |
| MS-05 | Parcel list | All parcels for active stop | pending, scanned, verified, attention |
| MS-06 | Parcel scan | Scan parcel code | camera permission, scanning, validating |
| MS-07 | Parcel scan outcome | Show scan result | success, duplicate, invalid, wrong manifest, offline |

# 16. Secure Handover - OTP and QR

Classification: NEW REQUIRED UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| HND-01 | Handover entry | Explain protected custody step | pickup, delivery, provider transfer, return |
| HND-02 | OTP credential | Enter/verify OTP | active, validating, incorrect, attempts remaining |
| HND-03 | OTP resend | Resend protected credential | cooldown, resend count, limit reached |
| HND-04 | OTP locked | Attempts exhausted | locked, support/recovery path |
| HND-05 | OTP expired | Expired challenge | refresh/reissue where allowed |
| HND-06 | QR scanner | Scan protected QR | camera permission, scanning, torch |
| HND-07 | QR validation | Validate challenge + manifest | valid, invalid, expired, wrong manifest |
| HND-08 | Handover confirmed | Custody transition succeeded | authoritative confirmation, next step |

# 17. Evidence and Proof

Classification: KEEP & MAJOR EXPANSION

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| EVD-01 | Evidence requirement summary | List required proof items | photo, signature, receiver info, custom evidence type |
| EVD-02 | Evidence session | Create/refresh evidence session | active, expired, failure |
| EVD-03 | Photo capture | Capture field evidence | camera permission, preview, retake |
| EVD-04 | Signature capture | Collect signature where required | clear, confirm, validation |
| EVD-05 | Evidence upload | Upload media/evidence metadata | uploading, accepted, failed, retry |
| EVD-06 | Evidence bundle review | Review all proof before final action | complete, missing required item, superseded evidence |
