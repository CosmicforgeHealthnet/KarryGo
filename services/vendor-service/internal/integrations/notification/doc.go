// Package notification is the reserved integration boundary for
// notification-service.
//
// Planned contract (not implemented in this architecture phase): send
// requests via shared/go/notifications.Client (idempotency key, source
// service "vendor-service", event type, recipient type/id, template key or
// inline title/body), matching every other service's convention. Vendor
// commerce events that plausibly need this once implemented: order placed,
// order accepted/rejected by vendor, substitution proposed, fulfilment
// dispatched, delivery completed, vendor staff invited, vendor verification
// decision.
//
// notification-service already accepts a caller-supplied source_service
// string per HTTP request (no service allowlist migration needed there); the
// only onboarding step is adding vendor-service to
// NOTIFICATION_SERVICE_SECRETS on notification-service, tracked in
// docs/PLATFORM_SERVICE_CHANGE_REGISTER.md when the first real notification
// send is implemented.
package notification
