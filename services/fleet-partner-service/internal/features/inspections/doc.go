// Package inspections owns Section 18 checklist templates, inspections and
// the unsafe-vehicle decline reconciler. Completing an inspection commits the
// inspection, the asset block, the repair work order, the defects, audit and
// outbox in one transaction: the asset safety primitives and the maintenance
// work-order writer are composed into that transaction through ports wired
// in internal/app.
//
// Layout: models, repositories, usecases, http.
package inspections
