# Dispatch delivery foundation and customer serviceability

This additive capability introduces the Sections 4-6 delivery-domain and
pre-booking serviceability foundation without replacing the existing
`dispatch_bookings -> delivery_requests -> trips` execution chain.

## Authority and compatibility

- `dispatch_bookings`, request broadcasting/matching and `trips` remain the
  canonical legacy execution path.
- Delivery groups and delivery missions are richer correlation/state
  foundations. A mission may reference one canonical Trip; it is not another
  rider execution engine.
- Wallet remains the only financial authority. A serviceability response is a
  non-financial snapshot and never holds funds, creates a booking, starts
  matching or reserves a rider.
- Existing Commerce mission tables and routes remain unchanged. The customer
  foundation does not replace the accepted Vendor Commerce integration.
- Existing proof, recipient confirmation, failure and location paths are not
  changed.

## Customer serviceability API

`POST /api/v1/customer/dispatch/serviceability` is registered only when both
`DISPATCH_ROUTE_SERVICEABILITY_ENABLED` and
`DISPATCH_PACKAGE_POLICY_ENABLED` are true. It requires a Customer bearer
token and an `Idempotency-Key` header. Customer scope comes exclusively from
the authenticated token.

The endpoint evaluates the configured operating zone, restricted zones,
motorcycle support, the existing road-route authority and versioned package
policy. It persists the decision before returning an opaque
`serviceability_id`. Existing fare math in `shared/go/dispatchpricing` remains
the pricing authority.

An identical idempotent retry returns the original snapshot. Reusing the key
with changed semantics returns a conflict.

## Deployment configuration

All new behavior defaults off:

- `DISPATCH_ROUTE_SERVICEABILITY_ENABLED=false`
- `DISPATCH_PACKAGE_POLICY_ENABLED=false`
- `DISPATCH_SERVICEABILITY_DEGRADED_MODE_ENABLED=false`
- `DISPATCH_SERVICEABILITY_TTL_SECONDS=900`
- `DISPATCH_SERVICEABILITY_MAX_ROUTE_KM=60`
- `DISPATCH_SERVICE_ZONE_CENTER_LAT=0`
- `DISPATCH_SERVICE_ZONE_CENTER_LNG=0`
- `DISPATCH_SERVICE_ZONE_RADIUS_KM=0`
- `DISPATCH_SERVICE_RESTRICTED_ZONES=`

The service-zone center/radius must be configured before enabling the route.
Restricted zones use `lat,lng,radius_km` entries separated by semicolons.
Missing or malformed zone policy fails closed with a controlled review result.

Authoritative road routing uses `DISPATCH_ROUTE_SERVICE_URL`. A routing fault
fails closed by default. Explicit degraded mode permits a conservative,
visibly low-confidence geographic estimate and records `DEGRADED_ROUTING`; it
is never presented as authoritative road evidence.

## Package policy

The server-owned `dispatch_package_admission_v1` evaluator returns controlled
admission results and reason codes. Explicit category and hazard
classifications drive prohibited/restricted decisions; free-text description
is never classification authority. Declared and measured attributes support a
future mismatch/reprice decision, but neither a rider nor this endpoint can
set a fare.

## Deferred deliberately

Sections 7-12 remain deferred: sequential offers/queues, multi-vendor planning,
new OTP/QR challenge issuance, evidence/custody workflows, returns/provider
transfer workflows and route-alert workers. Existing capabilities in those
areas remain untouched.
