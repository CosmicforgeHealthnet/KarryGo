// Package migrationlint computes a migration inventory directly from the
// on-disk migration directory (never a hand-maintained static list, which
// would silently drift the moment a new migration lands) and enforces
// Section 30's additive-first / destructive-migration-guard policy against
// it.
//
// Historical migrations (every version already applied at this repository's
// current tip, see CurrentTipVersion) are treated as an accepted baseline —
// several of them contain patterns (e.g. DROP CONSTRAINT ... ADD CONSTRAINT
// to widen a CHECK) that this package classifies but does not fail on. Only
// a migration numbered ABOVE the baseline tip is held to the strict new-
// migration policy (RequireCleanupMetadata, RequireSafeDownOrDeclaration).
// This mirrors the mandate's own instruction: "Existing applied migrations
// remain historical baseline... New migrations are held to Section 30
// policy... Do not rewrite historical migrations to satisfy the new lint."
package migrationlint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// CurrentTipVersion is the last migration version already applied at this
// repository's baseline (confirmed against migrations/ at the time Section
// 30 was implemented: 000073_retention_export_deletion_tracking). Anything
// numbered at or below this is historical baseline; anything above it is a
// new migration subject to the full Section 30 policy.
const CurrentTipVersion = "000073"

// Category is one of the nine Section 30 migration classifications.
type Category string

const (
	CategoryAdditive             Category = "ADDITIVE"
	CategoryBackfill             Category = "BACKFILL"
	CategoryIndex                Category = "INDEX"
	CategoryConstraintEvolution  Category = "CONSTRAINT_EVOLUTION"
	CategoryEnumOrStateEvolution Category = "ENUM_OR_STATE_EVOLUTION"
	CategoryDestructive          Category = "DESTRUCTIVE"
	CategoryDataRewrite          Category = "DATA_REWRITE"
	CategoryPermissionSeed       Category = "PERMISSION_SEED"
	CategoryOperational          Category = "OPERATIONAL"
)

// ValidCategories is used to assert every classification produced is one of
// the nine declared values -- catches a typo in the classifier itself.
var ValidCategories = map[Category]bool{
	CategoryAdditive: true, CategoryBackfill: true, CategoryIndex: true,
	CategoryConstraintEvolution: true, CategoryEnumOrStateEvolution: true,
	CategoryDestructive: true, CategoryDataRewrite: true,
	CategoryPermissionSeed: true, CategoryOperational: true,
}

// Migration is one loaded up/down pair.
type Migration struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
	UpSQL    string
	DownSQL  string
}

// FullVersion is the sortable, comparable migration identity ("000073").
func (m Migration) FullVersion() string { return m.Version }

// IsNewRelativeToBaseline reports whether m is numbered above
// CurrentTipVersion -- i.e. was added after this repository's Section 30
// baseline and is therefore held to the full new-migration policy.
func (m Migration) IsNewRelativeToBaseline() bool {
	return m.Version > CurrentTipVersion
}

var filenamePattern = regexp.MustCompile(`^(\d{6})_(.+)\.(up|down)\.sql$`)

// LoadMigrations scans dir for *.up.sql/*.down.sql pairs and returns them
// sorted by version. An up file with no matching down file (or vice versa)
// is still returned with the missing side's path/SQL left empty -- callers
// that need strict pairing use PairingIssues.
func LoadMigrations(dir string) ([]Migration, error) {
	entries := map[string]*Migration{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		match := filenamePattern.FindStringSubmatch(name)
		if match == nil {
			return nil
		}
		version, rest, side := match[1], match[2], match[3]
		key := version + "_" + rest
		m, ok := entries[key]
		if !ok {
			m = &Migration{Version: version, Name: rest}
			entries[key] = m
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		switch side {
		case "up":
			m.UpPath = path
			m.UpSQL = string(content)
		case "down":
			m.DownPath = path
			m.DownSQL = string(content)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]Migration, 0, len(entries))
	for _, m := range entries {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Version == out[j].Version {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// --- classification -------------------------------------------------------

var (
	reDropTable       = regexp.MustCompile(`(?i)\bDROP\s+TABLE\b`)
	reDropColumn      = regexp.MustCompile(`(?i)\bDROP\s+COLUMN\b`)
	reRenameColumn    = regexp.MustCompile(`(?i)\bRENAME\s+COLUMN\b`)
	reRenameTable     = regexp.MustCompile(`(?i)\bRENAME\s+TO\b`)
	reAlterColumnType = regexp.MustCompile(`(?i)\bALTER\s+COLUMN\s+\S+\s+TYPE\b`)
	reSetNotNull      = regexp.MustCompile(`(?i)\bSET\s+NOT\s+NULL\b`)
	reDropConstraint  = regexp.MustCompile(`(?i)\bDROP\s+CONSTRAINT\b`)
	reAddConstraint   = regexp.MustCompile(`(?i)\bADD\s+CONSTRAINT\b`)
	// reCheckConstraint matches a DROP CONSTRAINT targeting a CHECK
	// constraint under either naming convention actually used across this
	// repository's migrations: a "_check" suffix (Postgres's own default
	// name for an inline CHECK) or a "chk_" prefix (this repository's
	// explicit-naming convention for constraints added via ALTER TABLE, e.g.
	// chk_fleet_partner_operating_mode).
	reCheckConstraint = regexp.MustCompile(`(?i)DROP\s+CONSTRAINT\s+(?:IF\s+EXISTS\s+)?(?:\S*_check\b|chk_\S+)`)
	// reDynamicCheckDiscovery matches this repository's other safe
	// check-widening idiom: several later migrations (e.g. 000056, 000057,
	// 000059, 000061, 000063) widen an ORIGINALLY-UNNAMED inline CHECK
	// constraint (so Postgres assigned it an autogenerated name that cannot
	// be guessed statically) by looking it up in the pg_constraint catalog
	// at migration time and dropping it via dynamic EXECUTE + quote_ident.
	// This is the exact "safe catalog lookup" pattern WP14 calls for, not a
	// destructive drop -- it is always paired with a same-file ADD
	// CONSTRAINT that reinstates a widened CHECK.
	reDynamicCheckDiscovery = regexp.MustCompile(`(?is)pg_constraint.*?DROP\s+CONSTRAINT.*?quote_ident\(`)
	reCreateIndex           = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\b`)
	reConcurrently          = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+CONCURRENTLY\b`)
	reCreateTable           = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\b`)
	reAddColumn             = regexp.MustCompile(`(?i)\bADD\s+COLUMN\b`)
	reUpdateStatement       = regexp.MustCompile(`(?im)^\s*UPDATE\s+\S+\s+SET\b`)
	reDeleteStatement       = regexp.MustCompile(`(?im)^\s*DELETE\s+FROM\s+\S+\b`)
	// The capture group stops at "(", matching TablesAffected's fix, so an
	// INSERT with no space before its column list (this repository's
	// compact migrations use both styles, e.g.
	// "INSERT INTO fleet_partner_permissions(code,...)VALUES(...)") never
	// pulls the column list into the captured table name.
	reInsertStatement = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+([A-Za-z0-9_."]+)`)
)

// permissionSeedTableHint matches ONLY the platform's actual RBAC catalog
// tables (fleet_partner_permissions, fleet_partner_role_permissions,
// fleet_partner_roles, fleet_partner_role_bindings) by identifier suffix --
// never a loose substring match. A business-domain table that merely
// contains one of these words, e.g. fleet_partner_recurring_order_templates
// or fleet_partner_organization_roles_of_a_different_kind, must not be
// misclassified as an IAM permission/role seed.
var permissionSeedTableHint = regexp.MustCompile(`(?i)_(permissions|roles|role_bindings)$`)

// Flags captures the individual structural properties a migration's SQL
// exhibits, independent of the single primary Category chosen for it.
type Flags struct {
	HasDropTable            bool
	HasDropColumn           bool
	HasRenameColumn         bool
	HasRenameTable          bool
	HasAlterColumnType      bool
	HasSetNotNull           bool
	HasDropConstraint       bool
	HasCheckWidening        bool // a check-shaped constraint dropped (by name or by catalog lookup) and paired with a same-file ADD CONSTRAINT
	HasDynamicCheckDiscover bool // DROP CONSTRAINT via pg_constraint catalog lookup + quote_ident (safe, unnamed-constraint idiom)
	HasConcurrentIndex      bool
	HasPlainIndex           bool
	HasCreateTable          bool
	HasAddColumn            bool
	HasDataUpdate           bool
	HasDataDelete           bool
	HasPermissionInsert     bool
}

// ExtractFlags inspects sql and reports which structural patterns it
// contains. Used both for classification and for the destructive guard.
func ExtractFlags(sql string) Flags {
	return Flags{
		HasDropTable:       reDropTable.MatchString(sql),
		HasDropColumn:      reDropColumn.MatchString(sql),
		HasRenameColumn:    reRenameColumn.MatchString(sql),
		HasRenameTable:     reRenameTable.MatchString(sql),
		HasAlterColumnType: reAlterColumnType.MatchString(sql),
		HasSetNotNull:      reSetNotNull.MatchString(sql),
		HasDropConstraint:  reDropConstraint.MatchString(sql),
		HasCheckWidening: (reCheckConstraint.MatchString(sql) || reDynamicCheckDiscovery.MatchString(sql)) &&
			reAddConstraint.MatchString(sql),
		HasDynamicCheckDiscover: reDynamicCheckDiscovery.MatchString(sql),
		HasConcurrentIndex:      reConcurrently.MatchString(sql),
		HasPlainIndex:           reCreateIndex.MatchString(sql) && !reConcurrently.MatchString(sql),
		HasCreateTable:          reCreateTable.MatchString(sql),
		HasAddColumn:            reAddColumn.MatchString(sql),
		HasDataUpdate:           reUpdateStatement.MatchString(sql),
		HasDataDelete:           reDeleteStatement.MatchString(sql),
		HasPermissionInsert: func() bool {
			for _, m := range reInsertStatement.FindAllStringSubmatch(sql, -1) {
				if permissionSeedTableHint.MatchString(m[1]) {
					return true
				}
			}
			return false
		}(),
	}
}

// Classify picks ONE primary Category for a migration's up-SQL, in
// descending risk order: an unexplained destructive operation always wins
// classification even if the same file also creates additive structures,
// because the inventory's job is to surface the highest-risk property of a
// migration, not average it away.
func Classify(sql string) Category {
	f := ExtractFlags(sql)

	// Genuinely destructive operations not otherwise explained by a same-
	// file compatible replacement.
	if f.HasDropTable || f.HasDropColumn || f.HasRenameColumn || f.HasRenameTable || f.HasDataDelete {
		return CategoryDestructive
	}
	if f.HasDropConstraint && !f.HasCheckWidening {
		// A constraint drop with no same-file re-add and no CHECK-widening
		// shape is treated as destructive pending review -- it may be a
		// legitimate FK/UNIQUE cleanup, but that requires a human to
		// classify explicitly rather than a heuristic assuming safety.
		return CategoryDestructive
	}
	if f.HasAlterColumnType || f.HasSetNotNull {
		return CategoryConstraintEvolution
	}
	// Only a genuine same-file DROP+ADD CONSTRAINT pair (widening an
	// EXISTING check/enum-like constraint) counts as enum/state evolution.
	// A brand-new table or column that simply declares an inline
	// CHECK(col IN (...)) is not "evolving" anything -- it is ordinary
	// additive schema (handled by the CreateTable/AddColumn branch below).
	if f.HasCheckWidening {
		return CategoryEnumOrStateEvolution
	}
	if f.HasDataUpdate {
		return CategoryDataRewrite
	}
	if f.HasPermissionInsert {
		return CategoryPermissionSeed
	}
	if f.HasConcurrentIndex || (f.HasPlainIndex && !f.HasCreateTable) {
		return CategoryIndex
	}
	if f.HasCreateTable || f.HasAddColumn {
		return CategoryAdditive
	}
	return CategoryOperational
}

// TablesAffected extracts table names referenced by CREATE TABLE, ALTER
// TABLE, and DROP TABLE statements -- a best-effort scan for the
// inventory's "tables affected" field, not a full SQL parser.
// The capture group deliberately excludes "(" (and other punctuation that
// can never be part of a bare or quoted identifier) so a "CREATE TABLE
// name(" with no space before the opening paren does not pull the paren --
// or the first column name after it -- into the captured table name.
var reTableRef = regexp.MustCompile(`(?i)\b(?:CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?|ALTER\s+TABLE|DROP\s+TABLE(?:\s+IF\s+EXISTS)?)\s+([A-Za-z0-9_."]+)`)

func TablesAffected(sql string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range reTableRef.FindAllStringSubmatch(sql, -1) {
		name := strings.Trim(m[1], `"`)
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

var reIndexRef = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z0-9_."]+)`)

// IndexesAffected extracts index names created by the migration.
func IndexesAffected(sql string) []string {
	var out []string
	for _, m := range reIndexRef.FindAllStringSubmatch(sql, -1) {
		out = append(out, m[1])
	}
	return out
}

var reConstraintRef = regexp.MustCompile(`(?i)\b(?:ADD\s+CONSTRAINT|DROP\s+CONSTRAINT(?:\s+IF\s+EXISTS)?)\s+([A-Za-z0-9_."]+)`)

// ConstraintsAffected extracts constraint names added or dropped.
func ConstraintsAffected(sql string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range reConstraintRef.FindAllStringSubmatch(sql, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// --- inventory --------------------------------------------------------

// CleanupMetadataMarker is the SQL comment marker a new destructive
// migration must include, naming the prerequisite release/version this
// cleanup depends on (see RequireCleanupMetadata). Example:
//
//	-- SECTION30-CLEANUP-RELEASE: requires 2026.09-release (backfill complete)
var reCleanupMarker = regexp.MustCompile(`(?i)--\s*SECTION30-CLEANUP-RELEASE:\s*(.+)`)

// CleanupReleaseMetadata returns the declared prerequisite release string,
// if any.
func CleanupReleaseMetadata(sql string) (string, bool) {
	m := reCleanupMarker.FindStringSubmatch(sql)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// NoSafeDownMarker is the SQL comment a migration's down file uses to
// declare it deliberately cannot safely undo the up migration (e.g. it
// would destroy legitimate data), with a required rationale. Example:
//
//	-- SECTION30-NO-SAFE-DOWN: dropping this table would discard audit rows
var reNoSafeDownMarker = regexp.MustCompile(`(?i)--\s*SECTION30-NO-SAFE-DOWN:\s*(.+)`)

// NoSafeDownRationale returns the declared rationale, if any.
func NoSafeDownRationale(sql string) (string, bool) {
	m := reNoSafeDownMarker.FindStringSubmatch(sql)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// InventoryEntry is one migration's full Section 30 classification.
type InventoryEntry struct {
	Version                string   `json:"migrationNumber"`
	Name                   string   `json:"name"`
	UpFile                 string   `json:"upFile"`
	DownFile               string   `json:"downFile"`
	Category               Category `json:"category"`
	Transactional          bool     `json:"transactional"` // this migration runner always wraps each migration in its own transaction
	TablesAffected         []string `json:"tablesAffected"`
	IndexesAffected        []string `json:"indexesAffected"`
	ConstraintsAffected    []string `json:"constraintsAffected"`
	RollbackSafe           bool     `json:"rollbackSafe"`
	ReplayVerified         bool     `json:"replayVerified"`
	DataLossPossible       bool     `json:"dataLossPossible"`
	CleanupReleaseRequired bool     `json:"cleanupReleaseRequired"`
	CleanupReleaseNote     string   `json:"cleanupReleaseNote,omitempty"`
	IsHistoricalBaseline   bool     `json:"historicalBaseline"`
	Notes                  string   `json:"notes,omitempty"`
}

// BuildInventory computes the full, drift-proof inventory directly from the
// loaded migrations -- never a hand-maintained list.
func BuildInventory(migrations []Migration) []InventoryEntry {
	out := make([]InventoryEntry, 0, len(migrations))
	for _, m := range migrations {
		flags := ExtractFlags(m.UpSQL)
		category := Classify(m.UpSQL)
		cleanupNote, hasCleanup := CleanupReleaseMetadata(m.UpSQL)
		_, hasNoSafeDown := NoSafeDownRationale(m.DownSQL)

		dataLoss := flags.HasDropTable || flags.HasDropColumn || flags.HasDataUpdate || flags.HasDataDelete ||
			(flags.HasDropConstraint && !flags.HasCheckWidening)
		rollbackSafe := m.DownSQL != "" && !dataLoss && !hasNoSafeDown
		cleanupRequired := category == CategoryDestructive || flags.HasAlterColumnType ||
			flags.HasSetNotNull || flags.HasDataUpdate || flags.HasDataDelete

		notes := ""
		switch {
		case cleanupRequired && !hasCleanup && m.IsNewRelativeToBaseline():
			notes = "NEW cleanup/data-rewrite migration missing SECTION30-CLEANUP-RELEASE metadata"
		case category == CategoryDestructive:
			notes = "destructive-shaped operation (historical baseline or metadata present)"
		case flags.HasSetNotNull && flags.HasDataUpdate:
			notes = "historical same-transaction backfill plus SET NOT NULL; a new equivalent requires a staged cleanup release"
		case flags.HasSetNotNull:
			notes = "SET NOT NULL constraint evolution; a new equivalent requires prior backfill and cleanup-release metadata"
		case flags.HasCheckWidening:
			notes = "CHECK-constraint widened (drop old check, add new check) -- the standard safe enum/state-evolution pattern"
		}

		out = append(out, InventoryEntry{
			Version: m.Version, Name: m.Name, UpFile: m.UpPath, DownFile: m.DownPath,
			Category: category, Transactional: true,
			TablesAffected:         TablesAffected(m.UpSQL),
			IndexesAffected:        IndexesAffected(m.UpSQL),
			ConstraintsAffected:    ConstraintsAffected(m.UpSQL),
			RollbackSafe:           rollbackSafe,
			ReplayVerified:         !m.IsNewRelativeToBaseline(), // every baseline migration is exercised by Section 30's empty-database and staged replay tests
			DataLossPossible:       dataLoss,
			CleanupReleaseRequired: cleanupRequired,
			CleanupReleaseNote:     cleanupNote,
			IsHistoricalBaseline:   !m.IsNewRelativeToBaseline(),
			Notes:                  notes,
		})
	}
	return out
}

// --- pairing / numbering checks ----------------------------------------

// PairingIssue describes one numbering or up/down pairing defect.
type PairingIssue struct {
	Version string
	Problem string
}

// CheckPairingAndNumbering validates sequential numbering, absence of
// duplicate numbers, and that every version has both an up and a down file
// (a version with no down file is only acceptable when its up file's SQL
// declares NO_SAFE_DOWN is not applicable here -- an entirely MISSING down
// file is always an issue; NO_SAFE_DOWN is declared inside a present-but-
// intentionally-destructive down file, see NoSafeDownRationale).
func CheckPairingAndNumbering(migrations []Migration) []PairingIssue {
	var issues []PairingIssue
	seen := map[string]int{}
	for _, m := range migrations {
		seen[m.Version]++
		if m.UpPath == "" {
			issues = append(issues, PairingIssue{Version: m.Version, Problem: "orphan down migration: no matching up file"})
		}
		if m.DownPath == "" {
			issues = append(issues, PairingIssue{Version: m.Version, Problem: "orphan up migration: no matching down file"})
		}
	}
	for version, count := range seen {
		if count > 1 {
			issues = append(issues, PairingIssue{Version: version, Problem: fmt.Sprintf("duplicate migration number (%d entries)", count)})
		}
	}
	// Sequential: every version must be a positive integer and versions
	// must be gapless and strictly increasing by exactly 1 in numeric
	// terms (matching this repository's own 000001, 000002, ... convention).
	versions := make([]string, 0, len(seen))
	for v := range seen {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	for i, v := range versions {
		n, err := strconv.Atoi(v)
		if err != nil {
			issues = append(issues, PairingIssue{Version: v, Problem: "version is not numeric"})
			continue
		}
		want := i + 1
		if n != want {
			issues = append(issues, PairingIssue{Version: v, Problem: fmt.Sprintf("non-sequential: expected position %d (%06d), found %s", want, want, v)})
		}
	}
	return issues
}
