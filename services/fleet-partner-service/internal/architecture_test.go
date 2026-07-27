// Package internal_test enforces the architectural boundaries of
// fleet-partner-service by scanning source imports and content.
package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const serviceRoot = ".."

// collectGoFiles walks the service tree returning non-test Go files under dir.
func collectGoFiles(t *testing.T, dir string, includeTests bool) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func importsOf(t *testing.T, file string) []string {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		imports = append(imports, strings.Trim(imported.Path.Value, `"`))
	}
	return imports
}

// TestNoOtherServiceInternalsImported: fleet-partner-service must never
// import another service's packages (databases, features, anything).
func TestNoOtherServiceInternalsImported(t *testing.T) {
	for _, file := range collectGoFiles(t, serviceRoot, true) {
		for _, imported := range importsOf(t, file) {
			if strings.HasPrefix(imported, "cosmicforge/logistics/services/") &&
				!strings.HasPrefix(imported, "cosmicforge/logistics/services/fleet-partner-service") {
				t.Errorf("%s imports another service's package %s", file, imported)
			}
		}
	}
}

// TestDomainModelsAreInfrastructureFree: feature domain models must not
// import SQL drivers, HTTP routers or provider SDKs.
func TestDomainModelsAreInfrastructureFree(t *testing.T) {
	modelDirs, _ := filepath.Glob(filepath.Join(serviceRoot, "internal", "features", "*", "models"))
	forbidden := []string{"github.com/jackc/pgx", "github.com/gin-gonic/gin", "database/sql", "net/http", "github.com/redis"}
	for _, dir := range modelDirs {
		for _, file := range collectGoFiles(t, dir, false) {
			for _, imported := range importsOf(t, file) {
				for _, banned := range forbidden {
					if strings.HasPrefix(imported, banned) {
						t.Errorf("domain model %s imports infrastructure %s", file, imported)
					}
				}
			}
		}
	}
}

// TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories: cross-feature
// collaboration must use interfaces, not another feature's http/repository
// packages.
func TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories(t *testing.T) {
	featureDirs, _ := filepath.Glob(filepath.Join(serviceRoot, "internal", "features", "*"))
	for _, dir := range featureDirs {
		feature := filepath.Base(dir)
		for _, file := range collectGoFiles(t, dir, false) {
			for _, imported := range importsOf(t, file) {
				const featurePrefix = "cosmicforge/logistics/services/fleet-partner-service/internal/features/"
				if !strings.HasPrefix(imported, featurePrefix) {
					continue
				}
				rest := strings.TrimPrefix(imported, featurePrefix)
				parts := strings.Split(rest, "/")
				if parts[0] != feature && len(parts) > 1 && (parts[1] == "http" || parts[1] == "repositories") {
					t.Errorf("feature %s imports %s from feature %s (%s)", feature, parts[1], parts[0], file)
				}
			}
		}
	}
}

// TestPartnerAPINotImported: the api-gateway Partner API must stay separate
// from corporate authentication and clients.
func TestPartnerAPINotImported(t *testing.T) {
	for _, file := range collectGoFiles(t, serviceRoot, true) {
		for _, imported := range importsOf(t, file) {
			if strings.Contains(imported, "api-gateway") || strings.Contains(strings.ToLower(imported), "partner") &&
				strings.HasPrefix(imported, "cosmicforge/") && !strings.Contains(imported, "fleet-partner-service") {
				t.Errorf("%s imports Partner API package %s", file, imported)
			}
		}
	}
}

// TestMoneyPackageUsesNoFloats: no float arithmetic in the Money package.
func TestMoneyPackageUsesNoFloats(t *testing.T) {
	for _, file := range collectGoFiles(t, filepath.Join(serviceRoot, "internal", "platform", "money"), false) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		if strings.Contains(text, "float32") || strings.Contains(text, "float64") {
			t.Errorf("%s uses floating point for money", file)
		}
	}
}

// TestNoPricingFormulas: the corporate service must not introduce
// commission percentages, subscription prices or marketplace fare formulas.
// Scans non-test source (docs and tests may mention the words when stating
// the prohibition).
func TestNoPricingFormulas(t *testing.T) {
	forbidden := []string{
		"commissionPercent", "CommissionPercent", "commission_percent",
		"subscriptionPrice", "SubscriptionPrice", "subscription_price",
		"fareFormula", "FareFormula", "fare_formula",
	}
	for _, file := range collectGoFiles(t, serviceRoot, false) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Errorf("%s contains pricing token %q", file, token)
			}
		}
	}
}

// TestNoForeignServiceDSNsInConfig: the service configuration must not carry
// another service's database DSN.
func TestNoForeignServiceDSNsInConfig(t *testing.T) {
	for _, file := range collectGoFiles(t, filepath.Join(serviceRoot, "internal", "config"), true) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, foreign := range []string{
			"CUSTOMER_DATABASE_URL", "DISPATCH_DATABASE_URL", "TAXI_DATABASE_URL",
			"HAULING_DATABASE_URL", "PAYMENT_DATABASE_URL", "NOTIFICATION_DATABASE_URL",
			"MEDIA_DATABASE_URL", "VERIFICATION_DATABASE_URL", "SUPPORT_DATABASE_URL",
			"ANALYTICS_DATABASE_URL", "ADMIN_DATABASE_URL",
		} {
			if strings.Contains(text, foreign) {
				t.Errorf("%s references foreign database DSN %s", file, foreign)
			}
		}
	}
}

// TestSQLAlwaysTenantScoped: every SQL statement touching organization-owned
// tables must filter or set organization_id.
func TestSQLAlwaysTenantScoped(t *testing.T) {
	tenantTables := []string{"fleet_partner_branches", "fleet_partner_departments"}
	repoDir := filepath.Join(serviceRoot, "internal", "features", "organizations", "repositories")
	for _, file := range collectGoFiles(t, repoDir, false) {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, table := range tenantTables {
			if strings.Contains(text, table) && !strings.Contains(text, "organization_id") {
				t.Errorf("%s touches %s without organization_id scoping", file, table)
			}
		}
	}
}
