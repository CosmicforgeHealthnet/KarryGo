//go:build !admin_policy_test

// Package adminpolicyenv fixes approval-policy environment at build time.
package adminpolicyenv

const Environment = "PRODUCTION"
