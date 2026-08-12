// Package admin is the reserved integration boundary for
// admin-backoffice-service.
//
// Current state: admin-backoffice-service is a pure scaffold today (no
// routes, no migrations — see docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md).
// There is nothing to integrate with yet. Planned contract once it exists:
// vendor moderation actions (suspend a vendor business, force-close a
// listing), operator dashboards over vendor/order read models, and admin
// override of vendor verification decisions — all exposed on
// admin-backoffice-service's own routes, calling back into vendor-service's
// internal API rather than vendor-service depending on admin-backoffice-service
// for anything at write time.
package admin
