// Package maintenance owns Section 18 preventive maintenance schedules, work
// orders (including those raised by inspections, defects, breakdowns and
// safety declines) and authorized return to service. Safety-block and
// return-to-service transaction boundaries are preserved by composing the
// asset safety primitives into the same transaction through ports wired in
// internal/app.
//
// Layout: models, repositories, usecases, http.
package maintenance
