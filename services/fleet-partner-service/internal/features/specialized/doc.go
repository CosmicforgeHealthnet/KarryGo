// Package specialized owns organization-scoped extension data for Section 20.
// It deliberately references shared orders, shipments, assets, trips and
// financial authorities instead of duplicating them.
//
// Layout: models (module catalogue, typed schemas and validation), repositories
// (SQL), usecases (workflows and the maintenance sweep) and http (route
// manifest and handlers).
package specialized
