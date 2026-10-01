// Package features is the root of pricing-service's feature packages.
//
// Each feature owns one bounded responsibility and follows the platform layout:
//
//	internal/features/<feature>/
//	    http/          request parsing and response shaping only
//	    usecases/      the workflow and its authorisation rules
//	    models/        domain types and pure validation
//	    repositories/  persistence
//
// A feature may not import another feature's http or repositories package; the
// architecture test in internal/ enforces it. Cross-feature needs go through a
// port interface wired in cmd/.
//
// The pricing calculation itself deliberately does NOT live here. It sits in
// internal/engine as pure, I/O-free code, because quotes, reconciliation,
// simulation and shadow comparison must all evaluate a policy the same way, and
// an evaluator that could reach a database could never be replayed.
//
// Planned features: geography, policies, approvals, rollouts, quotes,
// reconciliation, shadow, indices, costprofiles, promotions.
package features
