// Package assets owns organization assets: the asset register and lifecycle,
// provider assignments, asset documents, eligibility, meter readings,
// operating-cost history, breakdowns, spare parts and stock movements,
// defects, tyres and the asset safety-block primitives. Canonical vehicles,
// trips, Media bytes, provider identities and financial ledgers remain
// externally authoritative.
//
// Inspections, maintenance (schedules and work orders) and fuel live in
// their own features and compose the safety primitives into their own
// transactions through ports wired in internal/app.
//
// Layout: models, repositories, usecases, http.
package assets
