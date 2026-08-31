# Section 36 traceability

This roadmap traceability document maps the implementation without creating a
roadmap-number production package or source file.

| Requirement | Authority / evidence |
|---|---|
| Typed semantic notification contract | `shared/go/notifications/notifications.go`; Vendor `notification_publisher.go`; Notification migration `002` |
| Vendor/customer notification matrix | controlled Vendor catalog, transactional outbox workers, Notification-owned templates |
| Scheduled cutoff | `scheduled_order_notifications.go`; unique marker in Vendor migration `000106` |
| Receiver credential | existing Dispatch handover authority and Dispatch-owned Notification Service intent; VendorMission remains dependency-gated |
| Realtime | Notification Service WebSocket/in-app channels plus authoritative REST read-back |
| Deterministic ETA range and confidence | Orders `confidence_eta.go` and focused unit matrix |
| Recompute and event integration | Orders Dispatch/Production projections; enriched Dispatch Commerce events |
| Immutable history/current pointer | Vendor migration `000106` and Orders ETA store |
| Isolation | customer id and VendorOrder/Parcel joins in scoped ETA reads |
| Migration ownership | Vendor `000106`; Notification `002`; no Dispatch/Hauling schema change |
| Frontend handoff | `FRONTEND_REQUIREMENTS.md` and `UI_UX_REQUIREMENTS.md`; no frontend code |

Canonical PostgreSQL apply/rollback/reapply and race evidence is environment
gated by the repository's existing test database variables. Missing external
test infrastructure is reported separately from implementation completeness.
