// Package taxi is the reserved integration boundary for driver-taxi-service.
//
// Current state (see docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md):
// driver-taxi-service is a pure directory scaffold today — zero Go files
// beyond cmd/main.go's capability list, zero migrations, zero goods-delivery
// concept. It owns passenger rides only, and only in intent, not in code yet.
//
// Per CLAUDE.md ("Taxi Important Boundary") and the implementation
// specification: this architecture phase does not implement Taxi goods
// delivery. It only records the future boundary:
//   - A future GOODS_DELIVERY capability on driver-taxi-service, distinct
//     from passenger ride booking/matching/trip-lifecycle, so a car-class
//     provider can carry a small commerce order without being routed through
//     dispatch (motorcycle) or hauling (van/truck).
//   - Same non-authority rule as dispatch/hauling: vendor-service requests
//     execution, never assigns a taxi provider or mutates taxi trip state.
//   - Payment/logistics boundary: fare and mission lifecycle for
//     GOODS_DELIVERY would need to reuse the same
//     shared/go/dispatchpricing-style single-source-of-truth pattern
//     dispatch and its api-gateway partner surface already share, not a
//     third independently-maintained fare formula.
//   - Pickup/delivery custody confirmation would depend on the same
//     internal/integrations/handover credential as dispatch/hauling —
//     PENDING_EXTERNAL_DEPENDENCY.
//
// Recorded in docs/PLATFORM_SERVICE_CHANGE_REGISTER.md as future work on
// driver-taxi-service, status NOT_STARTED. Not attempted here.
package taxi
