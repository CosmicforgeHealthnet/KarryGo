// Migration policy contract tests required by Section 30 WP4-WP7, WP11-14,
// and WP42. These are pure/offline (no database needed) except where noted
// -- they assert properties of the on-disk migration set and the
// migrationlint inventory computed from it, which is why they run in every
// `go test ./...` invocation rather than being gated on
// FLEET_PARTNER_TEST_DATABASE_URL like the replay tests in
// migration_replay_test.go.
package section30contract

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/migrationlint"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pgversion"
)

// TestInventory_CompletenessMatchesDirectory is WP4/WP42's "inventory
// completeness" gate. It counts *.up.sql files directly with os.ReadDir --
// deliberately NOT reusing migrationlint.LoadMigrations for the count side
// of this comparison, so the test does not just check migrationlint
// against itself -- and asserts the inventory migrationlint actually
// builds has exactly one entry per file. A migration added to the
// directory without a matching inventory entry (or vice versa) fails this
// test; there is no hand-maintained list to silently drift.
func TestInventory_CompletenessMatchesDirectory(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	wantUpFiles := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			wantUpFiles++
		}
	}
	if wantUpFiles < 73 {
		t.Fatalf("directory scan found %d .up.sql files, want at least 73 (Section 30 baseline)", wantUpFiles)
	}

	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	inventory := migrationlint.BuildInventory(migrations)
	if len(inventory) != wantUpFiles {
		t.Fatalf("inventory has %d entries, directory scan found %d .up.sql files -- inventory is not complete", len(inventory), wantUpFiles)
	}
}

// TestInventory_NumberingAndPairingIsClean is WP7's up/down pairing and
// sequential-numbering gate.
func TestInventory_NumberingAndPairingIsClean(t *testing.T) {
	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	issues := migrationlint.CheckPairingAndNumbering(migrations)
	if len(issues) > 0 {
		t.Errorf("found %d numbering/pairing issues:", len(issues))
		for _, issue := range issues {
			t.Errorf("  %s: %s", issue.Version, issue.Problem)
		}
	}
}

// TestInventory_NoUnexplainedDestructiveMigrationsInCurrentBaseline is
// WP6's real-inventory half of the destructive-migration guard: every
// migration actually in this repository today, classified DESTRUCTIVE by
// migrationlint, must either be historical baseline (accepted, see
// migrationlint's package doc) or -- if ever numbered above the baseline
// tip -- carry SECTION30-CLEANUP-RELEASE metadata. As of this session, zero
// migrations classify DESTRUCTIVE at all (the five that appeared to before
// migrationlint's dynamic-check-discovery fix -- 000056/57/59/61/63 -- are
// all safe catalog-lookup CHECK-widenings, see migrationlint_test.go), so
// this test is a regression guard: it must keep failing loudly the day a
// genuinely unexplained destructive migration is ever added.
func TestInventory_NoUnexplainedDestructiveMigrationsInCurrentBaseline(t *testing.T) {
	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	inventory := migrationlint.BuildInventory(migrations)
	for _, entry := range inventory {
		if !entry.CleanupReleaseRequired {
			continue
		}
		if entry.IsHistoricalBaseline {
			// Accepted baseline -- see migrationlint's package doc comment.
			// (Currently unreachable: no historical migration classifies
			// DESTRUCTIVE. Kept as a documented exception, not a silent one,
			// in case a future baseline bump ever introduces one.)
			continue
		}
		// A migration numbered above the baseline tip and classified
		// DESTRUCTIVE must carry SECTION30-CLEANUP-RELEASE metadata.
		if entry.CleanupReleaseNote == "" {
			t.Errorf("%s (%s): new destructive migration is missing SECTION30-CLEANUP-RELEASE metadata", entry.Version, entry.Name)
		}
	}
}

// TestPolicy_NewDestructiveMigrationRequiresCleanupMetadata is WP6's
// synthetic half: proves migrationlint's own logic actually enforces the
// cleanup-metadata requirement for a migration numbered ABOVE the current
// baseline tip, using a fabricated in-memory Migration rather than waiting
// for a real one to exist in the repository. This is what makes the guard
// meaningful going forward, not just descriptive of today's clean state.
func TestPolicy_NewDestructiveMigrationRequiresCleanupMetadata(t *testing.T) {
	newDestructiveNoMetadata := migrationlint.Migration{
		Version: "000074",
		Name:    "fabricated_destructive_no_metadata",
		UpPath:  "000074_fabricated_destructive_no_metadata.up.sql",
		UpSQL:   `ALTER TABLE fleet_partner_widgets DROP COLUMN legacy_field;`,
		DownSQL: `ALTER TABLE fleet_partner_widgets ADD COLUMN legacy_field text;`,
	}
	if !newDestructiveNoMetadata.IsNewRelativeToBaseline() {
		t.Fatalf("test setup bug: 000074 should be new relative to baseline %s", migrationlint.CurrentTipVersion)
	}

	inventory := migrationlint.BuildInventory([]migrationlint.Migration{newDestructiveNoMetadata})
	entry := inventory[0]
	if entry.Category != migrationlint.CategoryDestructive {
		t.Fatalf("fabricated DROP COLUMN migration classified %s, want DESTRUCTIVE", entry.Category)
	}
	if entry.CleanupReleaseNote != "" {
		t.Fatal("fabricated migration has no SECTION30-CLEANUP-RELEASE comment, but CleanupReleaseNote is non-empty")
	}
	if !strings.Contains(entry.Notes, "missing SECTION30-CLEANUP-RELEASE metadata") {
		t.Errorf("expected Notes to flag missing cleanup metadata for a new destructive migration, got: %q", entry.Notes)
	}

	// Now prove the SAME shape, WITH the metadata comment, is accepted.
	newDestructiveWithMetadata := newDestructiveNoMetadata
	newDestructiveWithMetadata.UpSQL = `-- SECTION30-CLEANUP-RELEASE: requires 2027.01-release (legacy_field fully unread by then)
ALTER TABLE fleet_partner_widgets DROP COLUMN legacy_field;`
	inventory = migrationlint.BuildInventory([]migrationlint.Migration{newDestructiveWithMetadata})
	entry = inventory[0]
	if entry.CleanupReleaseNote == "" {
		t.Error("expected CleanupReleaseNote to be populated once SECTION30-CLEANUP-RELEASE is present")
	}
	if strings.Contains(entry.Notes, "missing SECTION30-CLEANUP-RELEASE metadata") {
		t.Error("Notes still flags missing metadata even though SECTION30-CLEANUP-RELEASE is present")
	}
}

// TestPolicy_NoSafeDownDeclarationIsHonoured is WP7's exception path:
// migrationlint must accept a migration whose down cannot safely undo the
// up (real data would be destroyed) ONLY when it carries a
// SECTION30-NO-SAFE-DOWN rationale -- never silently, and never by
// fabricating a fake reversible down.
func TestPolicy_NoSafeDownDeclarationIsHonoured(t *testing.T) {
	irreversibleWithoutDeclaration := migrationlint.Migration{
		Version: "000075", Name: "fabricated_irreversible",
		UpSQL:   `DELETE FROM fleet_partner_widgets WHERE archived = true;`,
		DownSQL: ``, // no down at all
	}
	entry := migrationlint.BuildInventory([]migrationlint.Migration{irreversibleWithoutDeclaration})[0]
	if entry.RollbackSafe {
		t.Error("a migration with data loss and no down file at all must not be RollbackSafe")
	}

	irreversibleWithDeclaration := migrationlint.Migration{
		Version: "000075", Name: "fabricated_irreversible",
		UpSQL: `DELETE FROM fleet_partner_widgets WHERE archived = true;`,
		DownSQL: `-- SECTION30-NO-SAFE-DOWN: the deleted rows cannot be reconstructed
SELECT 1;`,
	}
	entry = migrationlint.BuildInventory([]migrationlint.Migration{irreversibleWithDeclaration})[0]
	if entry.RollbackSafe {
		t.Error("a SECTION30-NO-SAFE-DOWN declaration must remain explicitly not rollback-safe; the rationale documents irreversibility, it does not make rollback safe")
	}
}

// TestPolicy_NewConstraintTighteningAndDataRewriteRequireCleanupMetadata
// proves the guard covers the other potentially destructive operations named
// by the mandate, not only literal DROP statements.
func TestPolicy_NewConstraintTighteningAndDataRewriteRequireCleanupMetadata(t *testing.T) {
	for _, sql := range []string{
		`ALTER TABLE fleet_partner_widgets ALTER COLUMN external_ref SET NOT NULL;`,
		`ALTER TABLE fleet_partner_widgets ALTER COLUMN external_ref TYPE bigint;`,
		`UPDATE fleet_partner_widgets SET external_ref = id;`,
		`DELETE FROM fleet_partner_widgets WHERE archived = true;`,
	} {
		entry := migrationlint.BuildInventory([]migrationlint.Migration{{
			Version: "000074", Name: "fabricated_cleanup", UpSQL: sql, DownSQL: `SELECT 1;`,
		}})[0]
		if !entry.CleanupReleaseRequired {
			t.Errorf("%q did not require cleanup-release metadata", sql)
		}
		if !strings.Contains(entry.Notes, "missing SECTION30-CLEANUP-RELEASE metadata") {
			t.Errorf("%q did not report missing cleanup metadata: %q", sql, entry.Notes)
		}
	}
}

func TestInventory_DuplicateNumberAndMismatchedPairAreDetected(t *testing.T) {
	temp := t.TempDir()
	files := map[string]string{
		"000001_first.up.sql":    "CREATE TABLE first_table(id text);",
		"000001_second.down.sql": "DROP TABLE second_table;",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(temp, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	migrations, err := migrationlint.LoadMigrations(temp)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	issues := migrationlint.CheckPairingAndNumbering(migrations)
	joined := ""
	for _, issue := range issues {
		joined += issue.Problem + "\n"
	}
	for _, want := range []string{"duplicate migration number", "orphan up migration", "orphan down migration"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues %q do not contain %q", joined, want)
		}
	}
}

// TestPolicy_NoBackfillClassifiedMigrationsInCurrentBaseline is WP9/WP10's
// documentation-backed gate: this repository's schema migrations never
// perform an unbounded data backfill inside the migration transaction --
// large data population is delegated to the Section 27 durable job
// framework (internal/jobs), which supports bounded batches, SKIP LOCKED,
// resumability and dead-lettering; a migration-transaction backfill cannot
// offer any of that. Zero migrations classify BACKFILL today; this is a
// regression guard, not a tautology -- see
// docs/section30-database-migrations-backward-compatibility.md's "Backfill
// policy" section for the full design reasoning.
func TestPolicy_NoBackfillClassifiedMigrationsInCurrentBaseline(t *testing.T) {
	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	inventory := migrationlint.BuildInventory(migrations)
	for _, entry := range inventory {
		if entry.Category == migrationlint.CategoryBackfill {
			t.Errorf("%s (%s) classifies BACKFILL -- large data population belongs in a Section 27 durable job, not a migration transaction; see WP9/WP10", entry.Version, entry.Name)
		}
	}
}

// concurrentIndexPattern matches CREATE INDEX CONCURRENTLY in any casing.
var concurrentIndexPattern = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+CONCURRENTLY\b`)

// TestPolicy_NoConcurrentIndexInsideAnyMigration is WP12/WP13's guard: this
// service's migration runner (internal/database/migrate.go) wraps every
// migration file in its own pool.Begin()/Commit() transaction, and
// PostgreSQL categorically refuses CREATE INDEX CONCURRENTLY inside a
// transaction block ("CREATE INDEX CONCURRENTLY cannot run inside a
// transaction block"). A migration file containing it would fail at apply
// time on every environment, always -- this test catches that at commit
// time instead. For a genuinely large live table, the documented
// production-safe procedure is an operator-run, ordinary (non-transactional)
// psql/CLI step OUTSIDE the migration runner, never inside a migration
// file; see docs/section30-database-migrations-backward-compatibility.md's
// "Index creation strategy" section.
func TestPolicy_NoConcurrentIndexInsideAnyMigration(t *testing.T) {
	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	for _, m := range migrations {
		if concurrentIndexPattern.MatchString(m.UpSQL) {
			t.Errorf("%s: up migration contains CREATE INDEX CONCURRENTLY, which cannot run inside this runner's per-migration transaction", m.Version)
		}
		if concurrentIndexPattern.MatchString(m.DownSQL) {
			t.Errorf("%s: down migration contains CREATE INDEX CONCURRENTLY, which cannot run inside this runner's per-migration transaction", m.Version)
		}
	}
}

// TestPolicy_EnumEvolutionEntriesAreWideningNotNarrowing is WP14's
// evolution-safety gate: every migration classified ENUM_OR_STATE_EVOLUTION
// must be a same-file DROP+ADD CONSTRAINT pair (a widen), which
// migrationlint's HasCheckWidening flag already requires structurally --
// this test asserts that invariant holds for the real inventory, so a
// future classifier change cannot silently let a narrowing-only migration
// through this category.
func TestPolicy_EnumEvolutionEntriesAreWideningNotNarrowing(t *testing.T) {
	migrations, err := migrationlint.LoadMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	for _, m := range migrations {
		if migrationlint.Classify(m.UpSQL) != migrationlint.CategoryEnumOrStateEvolution {
			continue
		}
		flags := migrationlint.ExtractFlags(m.UpSQL)
		if !flags.HasCheckWidening {
			t.Errorf("%s: classified ENUM_OR_STATE_EVOLUTION but HasCheckWidening is false -- not a same-file widen", m.Version)
		}
	}
}

// TestPolicy_PostgreSQLMinimumVersionEnforced is WP11's live half,
// consolidated into section30contract as the single GO-gate package: the
// disposable test database (when configured) must meet
// pgversion.MinimumSupportedMajor. This duplicates
// pgversion's own live test deliberately -- section30contract is meant to
// be the one package a reviewer runs to get every Section 30 gate in one
// place.
func TestPolicy_PostgreSQLMinimumVersionEnforced(t *testing.T) {
	rawURL := envOrSkip(t)
	pool, err := pgxpool.New(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()
	if err := pgversion.RequireSupported(context.Background(), pool); err != nil {
		t.Error(err)
	}
}

func envOrSkip(t *testing.T) string {
	t.Helper()
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping live PostgreSQL version gate")
	}
	requireTestDatabaseName(t, rawURL)
	return rawURL
}
