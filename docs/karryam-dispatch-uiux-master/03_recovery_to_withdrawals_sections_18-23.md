# 18. Delivery Recovery

Classification: NEW REQUIRED UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| RCV-01 | Delivery attempt | Create/track attempt | grace period, receiver unavailable |
| RCV-02 | Contact attempt | Record contact attempt | call/contact outcome |
| RCV-03 | Receiver unavailable | Explain failed handover state | grace period and next actions |
| RCV-04 | Recovery selection | Choose backend-supported recovery | retry same day, reschedule, alternative receiver, return, hub hold, operations review |
| RCV-05 | Retry same day | Schedule/confirm retry | window, confirmation |
| RCV-06 | Reschedule | Choose/confirm new slot where server provides options | pending, confirmed |
| RCV-07 | Alternative receiver | Protected alternate receiver path | validation, secure handover still required |
| RCV-08 | Operations review | Escalate into operations-held state | submitted, waiting |
| RCV-09 | Attempt failed | Record terminal/structured failure | reason, server-declared retry eligibility |
| RCV-10 | Recovery resolved | Show canonical resolved state | next mission step |

# 19. Breakdown, Return, Hub Hold and Provider Transfer

Classification: NEW REQUIRED UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| RTH-01 | Breakdown report | Report mechanical/safety issue | reason, safe note, preferred strategy |
| RTH-02 | Breakdown submitted | Operations reviewing | waiting, replacement path |
| RTH-03 | Replacement assigned | Show replacement provider status | no custody transfer yet |
| RTH-04 | Secure provider transfer | Transfer parcels via protected handover | OTP/QR + manifest exactness |
| RTH-05 | Return initiated | Begin return-to-sender | reason, route, custody status |
| RTH-06 | Return arrival | Arrive at sender | secure return verification |
| RTH-07 | Hub hold | Navigate/arrive at hub | hub checkpoint stage |
| RTH-08 | Hub checkpoint | Scan parcel codes + media + authoritative confirmation | pending, confirmed, failure |

# 20. Route Integrity and Offline Replay

Classification: NEW REQUIRED UX

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| RTE-01 | Route navigation | Active route state | route version/status/reference |
| RTE-02 | Route alert | Road closure / safety / provider update | info, attention, critical |
| RTE-03 | Wrong-way / corridor warning | Warn route deviation | non-distracting driver UI |
| RTE-04 | Stationary / stale location | Location health issue | refresh/recover |
| RTE-05 | Reroute request | Submit allowed reroute reason | road closure, safety, provider update |
| RTE-06 | Offline mode | Explicit network-loss state | what remains usable |
| RTE-07 | Offline command queued | Saved on device | device command ID/idempotency state |
| RTE-08 | Offline reconciliation | Sync queued commands | syncing, confirmed, conflict/review required |

# 21. Wallet and Earnings

Classification: CONFIRMED - KEEP & REDESIGN

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| WAL-01 | Wallet home | Provider financial overview | loading, available data, error |
| WAL-02 | Available earnings | Show spendable/withdrawable earnings if returned | amount visibility, privacy |
| WAL-03 | Pending earnings | Show pending amounts if returned by contract | pending explanation |
| WAL-04 | Earnings summary | Time-period summary | today, week, month as supported by response/client logic |
| WAL-05 | Recent transactions | Financial activity list | empty, pagination/refresh |
| WAL-06 | Transaction detail | Income/withdrawal/adjustment detail | success, pending, failed |

# 22. Bank Accounts

Classification: CONFIRMED - KEEP

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| BNK-01 | Bank accounts | List provider payout destinations | empty, default, multiple |
| BNK-02 | Add bank account | Enter bank + account number | validation |
| BNK-03 | Resolve bank account | Verify account identity | resolving, success, mismatch/failure |
| BNK-04 | Register bank account | Confirm resolved destination | processing, success, failure |
| BNK-05 | Set default account | Choose payout default | confirm, failure |
| BNK-06 | Remove bank account | Delete destination safely | confirmation, blocked/default rules as backend returns |

# 23. Withdrawals and Transaction PIN

Classification: CONFIRMED - KEEP & REDESIGN

The following frames are the minimum design templates for this domain. Each template must be accompanied by the materially different states listed in the final column; minor visual variants should be represented as component states rather than unnecessary routes.

| ID | Screen / State | Purpose | Required UX states / notes |
| --- | --- | --- | --- |
| WDR-01 | Withdrawal entry | Choose amount and destination | balance/eligibility validation |
| WDR-02 | Withdrawal review | Review amount, bank and fees if provided | do not invent fee data |
| WDR-03 | PIN status gate | Determine whether transaction PIN exists | set PIN vs authenticate |
| WDR-04 | Create PIN | Set transaction PIN | validation, confirm PIN |
| WDR-05 | Change PIN | Current + new PIN flow | incorrect current PIN, success |
| WDR-06 | Authenticate transaction | Enter PIN for withdrawal | incorrect, attempts/lock if backend signals |
| WDR-07 | Withdrawal processing | Authoritative request in progress | idempotent retry |
| WDR-08 | Withdrawal result | Success / pending / failed | receipt/reference where returned |
| WDR-09 | Withdrawal receipt | Transaction summary | reference, date, destination, status |
