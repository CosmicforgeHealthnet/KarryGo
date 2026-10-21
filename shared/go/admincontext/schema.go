package admincontext

import _ "embed"

// Schema is embedded for owner migration verification and disposable tests.
// Production owners install the versioned copy through their normal runner.
//
//go:embed schema.sql
var Schema string
