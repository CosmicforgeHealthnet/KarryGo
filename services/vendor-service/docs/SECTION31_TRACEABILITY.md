# Section 31 traceability

| Requirement | Authority / evidence |
|---|---|
| Restricted Vendor Rider identity | Existing Vendor IAM role and branch permission; `vendor_rider.go` |
| Availability and one active mission | migration `000104_vendor_owned_delivery_runtime`; `vendor_rider_store.go` |
| Canonical VendorMission runtime | existing fulfilment aggregate plus capability-named Rider runtime files |
| Secure pickup/delivery/return | opaque `VendorHandoverAuthority`; production dependency-gated |
| Parcel custody and immutable history | existing custody authority, written atomically with Rider action |
| Pre-custody fallback | `hybrid_fallback.go`, `hybrid_fallback_store.go` |
| Immutable replan | accepted Section 25 `ActivatePlan` OCC authority |
| Dispatch execution | accepted Section 28 quote/create/read-back adapter |
| Funding transition | Wallet migration `000012` and internal HMAC v2 API |
| Vendor order lifecycle/settlement | existing VendorOrder projection and Section 23/24 Wallet authority |
| Concurrency evidence | env-gated Vendor and Wallet PostgreSQL integration tests |
| Frontend contract | `docs/handoffs/VENDOR_RIDER_MODE.md`; no UI implementation |

Sections 29, 30 and 32 were not implemented by this work.
