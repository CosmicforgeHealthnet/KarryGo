// Package compliance owns organization compliance requirements, deterministic
// evaluations and propagation of operational restrictions. Verification
// authorities remain external; this package stores their versioned evidence.
//
// Layout: models (domain and pure rules), repositories (SQL), usecases
// (workflows and workers) and http (route manifest and handlers).
package compliance
