//go:build admin_policy_test

package adminpolicyenv

// This build tag is used only by the real-process integration test. It is not
// passed by production Docker builds or exposed as a runtime environment flag.
const Environment = "TEST_ONLY"
