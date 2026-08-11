// Package support is the reserved integration boundary for
// support-dispute-service, which already has a real multi-app routing model
// (customer + 3 provider-app route groups, each its own bearer secret) and an
// admin triage surface.
//
// Planned contract (not implemented in this architecture phase): vendor-service
// is a new caller class, analogous to the existing taxi-provider/dispatch-provider
// groups — a POST /vendor route group (vendor-business or vendor-staff bearer
// auth) for filing complaints/escalations about orders, plus using the
// existing cross-service identity resolution
// (support-dispute-service/internal/features/support/clients/identity.go)
// so a support agent sees the vendor's name/contact without support-dispute-service
// owning vendor identity. Requires a new complainant_type value
// ("vendor" or similar) and a support-dispute-service migration + route group —
// tracked in docs/PLATFORM_SERVICE_CHANGE_REGISTER.md, not attempted here.
package support
