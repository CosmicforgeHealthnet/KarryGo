// Package pgversion records this service's supported PostgreSQL version
// range and gives migration/integration tests a single place to assert
// the connected server meets it, per Section 30 WP11.
package pgversion

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MinimumSupportedMajor is the lowest PostgreSQL major version this
// service's migrations are safe to run against. It is derived from the
// actual SQL features the migration set uses, not chosen arbitrarily:
//
//   - gen_random_uuid() is called directly (e.g. migrations 000045,
//     000053) with no `CREATE EXTENSION pgcrypto` anywhere in the
//     migration set. Before PostgreSQL 13, gen_random_uuid() only exists
//     via the pgcrypto extension; PostgreSQL 13 made it a built-in
//     function. This is the binding constraint.
//   - Generated columns (`GENERATED ALWAYS AS (...) STORED`, migration
//     000053's search_vector) require PostgreSQL 12+, a lower bar than
//     the gen_random_uuid() constraint above.
//
// If a future migration relies on a feature requiring a higher minimum
// (e.g. MERGE, introduced in PostgreSQL 15), raise this constant in the
// same change and update the doc comment's evidence list.
const MinimumSupportedMajor = 13

// TestedMajor is the PostgreSQL major version this service's migration and
// integration suites are actually exercised against in this repository's
// development/CI setup (see infra/docker-compose.yml and
// scripts/bootstrap_services' fleet-partner bootstrap, both pinned to the
// postgres:16-alpine image; confirmed live via `SHOW server_version`
// against the disposable test container as 16.14 at the time Section 30
// was implemented).
const TestedMajor = 16

var majorVersionPattern = regexp.MustCompile(`^(\d+)`)

// ServerMajorVersion queries the connected server's reported major version
// number (the leading integer of `SHOW server_version`, e.g. "16" from
// "16.14 (Debian ...)").
func ServerMajorVersion(ctx context.Context, pool *pgxpool.Pool) (int, string, error) {
	var full string
	if err := pool.QueryRow(ctx, `SHOW server_version`).Scan(&full); err != nil {
		return 0, "", fmt.Errorf("query server_version: %w", err)
	}
	match := majorVersionPattern.FindStringSubmatch(full)
	if match == nil {
		return 0, full, fmt.Errorf("could not parse a major version out of server_version %q", full)
	}
	var major int
	if _, err := fmt.Sscanf(match[1], "%d", &major); err != nil {
		return 0, full, fmt.Errorf("parse major version %q: %w", match[1], err)
	}
	return major, full, nil
}

// CheckMajorSupported is the pure comparison RequireSupported applies to a
// parsed major version and its full server_version string. Exported
// separately from RequireSupported so the below-minimum failure path is
// directly unit-testable without needing to actually connect to a
// below-minimum PostgreSQL server.
func CheckMajorSupported(major int, full string) error {
	if major < MinimumSupportedMajor {
		return fmt.Errorf(
			"connected PostgreSQL server_version %q (major %d) is below this service's minimum supported major version %d "+
				"(gen_random_uuid() is used directly with no pgcrypto extension, which requires PostgreSQL 13+)",
			full, major, MinimumSupportedMajor,
		)
	}
	return nil
}

// RequireSupported queries the connected server's version and returns an
// error if it is below MinimumSupportedMajor. Migration and integration
// suites call this once, alongside their existing FLEET_PARTNER_TEST_DATABASE_URL
// gate, so a below-minimum test database fails with an explicit,
// actionable message instead of an inscrutable mid-migration SQL error.
func RequireSupported(ctx context.Context, pool *pgxpool.Pool) error {
	major, full, err := ServerMajorVersion(ctx, pool)
	if err != nil {
		return err
	}
	return CheckMajorSupported(major, full)
}
