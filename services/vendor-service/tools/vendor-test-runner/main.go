package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

type testPackage struct {
	importPath string
	shortName  string
}

func main() {
	ctx := context.Background()

	root, err := serviceRoot()
	if err != nil {
		fail("%v", err)
	}

	rawDSN := strings.TrimSpace(os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL"))
	if rawDSN == "" {
		fail("VENDOR_SERVICE_TEST_DATABASE_URL is required")
	}

	baseURL, err := url.Parse(rawDSN)
	if err != nil {
		fail("parse VENDOR_SERVICE_TEST_DATABASE_URL: %v", err)
	}

	if baseURL.Scheme != "postgres" && baseURL.Scheme != "postgresql" {
		fail("test database URL must use postgres/postgresql")
	}

	if err := assertTestDatabaseSafety(baseURL); err != nil {
		fail("test database safety check failed: %v", err)
	}

	adminURL := cloneURL(baseURL)
	adminURL.Path = "/postgres"

	adminPool, err := pgxpool.New(ctx, adminURL.String())
	if err != nil {
		fail("connect PostgreSQL admin database: %v", err)
	}
	defer adminPool.Close()

	prefix := fmt.Sprintf(
		"vendor_iso_%d_%d",
		os.Getpid(),
		time.Now().UnixNano()%1_000_000_000,
	)

	templateDB := trimIdentifier(prefix + "_template")

	fmt.Println("=== VENDOR ISOLATED TEST RUNNER ===")
	fmt.Println("PASS: test database safety guard")
	fmt.Println("Creating migrated template database...")

	if err := createDatabase(ctx, adminPool, templateDB, ""); err != nil {
		fail("create template database: %v", err)
	}

	defer func() {
		_ = dropDatabase(context.Background(), adminPool, templateDB)
	}()

	templateURL := cloneURL(baseURL)
	templateURL.Path = "/" + templateDB

	templatePool, err := pgxpool.New(ctx, templateURL.String())
	if err != nil {
		fail("connect template database: %v", err)
	}

	if err := database.ApplyMigrations(
		ctx,
		templatePool,
		filepath.Join(root, "migrations"),
	); err != nil {
		templatePool.Close()
		fail("apply migrations to template: %v", err)
	}

	var migrationCount int
	if err := templatePool.QueryRow(
		ctx,
		`SELECT count(*) FROM vendor_service_schema_migrations`,
	).Scan(&migrationCount); err != nil {
		templatePool.Close()
		fail("verify template migrations: %v", err)
	}

	var measureCount int
	if err := templatePool.QueryRow(
		ctx,
		`SELECT count(*) FROM shop_measure_definitions`,
	).Scan(&measureCount); err != nil {
		templatePool.Close()
		fail("verify Shop measurement seed: %v", err)
	}

	templatePool.Close()

	if migrationCount < 47 {
		fail("template contains only %d migrations; expected at least 47", migrationCount)
	}

	if measureCount != 43 {
		fail("template contains %d Shop measurement definitions; expected 43", measureCount)
	}

	fmt.Printf(
		"PASS: template ready (%d migrations, %d measurement definitions)\n",
		migrationCount,
		measureCount,
	)

	packages, err := discoverPackages(root)
	if err != nil {
		fail("discover test packages: %v", err)
	}

	// An optional substring filter lets one gate be re-run on its own during
	// development without weakening anything: each selected package still gets
	// its own freshly migrated database cloned from the same template. The full
	// regression is what runs with no argument, and certification always runs
	// the full regression.
	if filter := strings.TrimSpace(strings.Join(os.Args[1:], " ")); filter != "" {
		var selected []testPackage
		for _, pkg := range packages {
			if strings.Contains(pkg.shortName, filter) {
				selected = append(selected, pkg)
			}
		}
		if len(selected) == 0 {
			fail("no test package matches %q", filter)
		}
		packages = selected
		fmt.Printf("Package filter: %s\n", filter)
	}

	fmt.Printf("Test-bearing packages: %d\n\n", len(packages))

	passed := 0
	failedCount := 0

	for i, pkg := range packages {
		dbName := trimIdentifier(
			fmt.Sprintf("%s_%03d", prefix, i+1),
		)

		if err := createDatabase(
			ctx,
			adminPool,
			dbName,
			templateDB,
		); err != nil {
			fmt.Printf(
				"FAIL: %s — create isolated database: %v\n",
				pkg.shortName,
				err,
			)
			failedCount++
			continue
		}

		pkgURL := cloneURL(baseURL)
		pkgURL.Path = "/" + dbName

		output, runErr := runPackage(
			root,
			pkg,
			pkgURL.String(),
		)

		if dropErr := dropDatabase(
			ctx,
			adminPool,
			dbName,
		); dropErr != nil {
			fmt.Printf(
				"FAIL: %s — drop isolated database: %v\n",
				pkg.shortName,
				dropErr,
			)
			failedCount++
			continue
		}

		if runErr != nil {
			fmt.Printf("FAIL: %s\n", pkg.shortName)
			fmt.Println(tail(output, 70))
			fmt.Println()
			failedCount++
			continue
		}

		passed++

		fmt.Printf(
			"PASS: %s [%d/%d] — %s\n",
			pkg.shortName,
			passed+failedCount,
			len(packages), output,
		)
	}

	fmt.Println()
	fmt.Printf(
		"Packages passed: %d/%d\n",
		passed,
		len(packages),
	)

	if failedCount != 0 {
		fmt.Printf(
			"VENDOR ISOLATED REGRESSION: FAIL (%d package failures)\n",
			failedCount,
		)
		os.Exit(1)
	}

	fmt.Println("VENDOR ISOLATED REGRESSION: GO")
}

func assertTestDatabaseSafety(u *url.URL) error {
	dbName := strings.TrimPrefix(u.Path, "/")
	host := strings.ToLower(u.Hostname())

	if dbName == "" {
		return fmt.Errorf("database name is empty")
	}

	switch strings.ToLower(dbName) {
	case "postgres", "template0", "template1":
		return fmt.Errorf(
			"refusing administrative/system database %q",
			dbName,
		)
	}

	localHost := host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		host == "postgres" ||
		host == "db" ||
		strings.HasSuffix(host, "-db")

	if !localHost &&
		os.Getenv("VENDOR_SERVICE_TEST_ALLOW_REMOTE_ADMIN") != "1" {
		return fmt.Errorf(
			"refusing remote host %q; set "+
				"VENDOR_SERVICE_TEST_ALLOW_REMOTE_ADMIN=1 "+
				"only for a dedicated CI test PostgreSQL server",
			host,
		)
	}

	return nil
}

func serviceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if exists(filepath.Join(dir, "go.mod")) &&
			exists(filepath.Join(dir, "migrations")) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(
				"could not locate vendor-service root",
			)
		}
		dir = parent
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cloneURL(source *url.URL) *url.URL {
	copy := *source
	return &copy
}

func discoverPackages(root string) ([]testPackage, error) {
	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{.ImportPath}}|{{len .TestGoFiles}}|{{len .XTestGoFiles}}",
		"./...",
	)

	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	moduleBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}

	modulePath := ""
	for _, line := range strings.Split(string(moduleBytes), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(
				strings.TrimPrefix(line, "module "),
			)
			break
		}
	}

	var result []testPackage

	for _, line := range strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	) {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}

		internalCount, err1 := strconv.Atoi(parts[1])
		externalCount, err2 := strconv.Atoi(parts[2])

		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf(
				"parse go list output %q",
				line,
			)
		}

		if internalCount+externalCount == 0 {
			continue
		}

		short := parts[0]
		if modulePath != "" {
			short = strings.TrimPrefix(
				parts[0],
				modulePath+"/",
			)
		}

		result = append(
			result,
			testPackage{
				importPath: parts[0],
				shortName:  short,
			},
		)
	}

	return result, nil
}

func createDatabase(
	ctx context.Context,
	admin *pgxpool.Pool,
	name string,
	template string,
) error {
	identifier := pgx.Identifier{name}.Sanitize()

	sql := "CREATE DATABASE " + identifier

	if template != "" {
		sql += " TEMPLATE " +
			pgx.Identifier{template}.Sanitize()
	}

	_, err := admin.Exec(ctx, sql)
	return err
}

func dropDatabase(
	ctx context.Context,
	admin *pgxpool.Pool,
	name string,
) error {
	_, _ = admin.Exec(
		ctx,
		`SELECT pg_terminate_backend(pid)
         FROM pg_stat_activity
         WHERE datname = $1
           AND pid <> pg_backend_pid()`,
		name,
	)

	_, err := admin.Exec(
		ctx,
		"DROP DATABASE IF EXISTS "+
			pgx.Identifier{name}.Sanitize(),
	)

	return err
}

func runPackage(
	root string,
	pkg testPackage,
	dsn string,
) (string, error) {
	cmd := exec.Command(
		"go",
		"test",
		"-json",
		pkg.importPath,
		"-count=1",
		"-timeout",
		"20m",
	)

	// The existing Customer certification also separates this five-process
	// running-stack test from isolated owner certification (cert.py VENDOR_ENV).
	// Exclude that exact optional stack test visibly; no database test is exempt.
	if pkg.shortName == "internal/features/fulfilmentgroup" {
		cmd.Args = append(cmd.Args, "-skip", "^TestStackOwnDeliveryShare$")
		fmt.Println("EXTERNAL STACK CERTIFICATION NOT RUN: TestStackOwnDeliveryShare requires five running local services; isolated owner cases still mandatory")
	}
	cmd.Dir = root

	env := os.Environ()
	env = replaceEnv(
		env,
		"VENDOR_SERVICE_TEST_DATABASE_URL",
		dsn,
	)

	env = replaceEnv(env, "VENDOR_TEST_DATABASE_URL", dsn)
	env = replaceEnv(env, "CLOSURE_MIGRATION_DATABASE_URL", dsn)
	env = replaceEnv(env, "CUSTOMER14_MIGRATION_ROUNDTRIP_DATABASE_URL", dsn)
	if addr := os.Getenv("VENDOR_REDIS_ADDR"); addr != "" {
		env = replaceEnv(env, "VENDOR_SERVICE_TEST_REDIS_ADDR", addr)
		env = replaceEnv(env, "CUSTOMER14_TEST_REDIS_ADDR", addr)
		env = replaceEnv(env, "JOURNEY_TEST_REDIS_ADDR", addr)
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if directory := strings.TrimSpace(os.Getenv("VENDOR_SERVICE_TEST_EVIDENCE_DIR")); directory != "" {
		if mkdirErr := os.MkdirAll(directory, 0700); mkdirErr != nil {
			return string(output), mkdirErr
		}
		filename := strings.ReplaceAll(pkg.shortName, "/", "_") + ".jsonl"
		if writeErr := os.WriteFile(filepath.Join(directory, filename), output, 0600); writeErr != nil {
			return string(output), writeErr
		}
	}
	if err != nil {
		return string(output), err
	}
	count, skipped, evidenceErr := testEvidence(output)
	if evidenceErr != nil {
		return string(output) + "\n" + evidenceErr.Error(), evidenceErr
	}
	return fmt.Sprintf("%d tests passed; %d skipped", count, len(skipped)), nil
}

// A successful package exit alone is insufficient: missing fixtures used to
// silently skip database cases while the canonical runner still printed GO.
func testEvidence(output []byte) (int, []string, error) {
	passed := 0
	skipped := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var event struct{ Action, Test, Package string }
		if err := json.Unmarshal(line, &event); err != nil {
			return passed, skipped, err
		}
		if event.Action == "fail" {
			return passed, skipped, fmt.Errorf("failed test/package %s %s", event.Package, event.Test)
		}
		if event.Test != "" && event.Action == "pass" {
			passed++
		}
		if event.Test != "" && event.Action == "skip" {
			skipped = append(skipped, event.Test)
		}
	}
	if err := scanner.Err(); err != nil {
		return passed, skipped, err
	}
	if len(skipped) > 0 {
		return passed, skipped, fmt.Errorf("skipped required Vendor tests: %s", strings.Join(skipped, ", "))
	}
	if passed == 0 {
		return 0, skipped, fmt.Errorf("no passing Vendor test evidence")
	}
	return passed, skipped, nil
}

func replaceEnv(
	env []string,
	key string,
	value string,
) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)

	found := false

	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			result = append(result, prefix+value)
			found = true
			continue
		}
		result = append(result, entry)
	}

	if !found {
		result = append(result, prefix+value)
	}

	return result
}

func trimIdentifier(value string) string {
	if len(value) <= 60 {
		return value
	}
	return value[:60]
}

func tail(text string, lines int) string {
	all := strings.Split(strings.TrimSpace(text), "\n")
	if len(all) <= lines {
		return strings.Join(all, "\n")
	}

	return strings.Join(
		all[len(all)-lines:],
		"\n",
	)
}

func fail(format string, args ...any) {
	fmt.Fprintf(
		os.Stderr,
		"FAIL: "+format+"\n",
		args...,
	)
	os.Exit(1)
}
