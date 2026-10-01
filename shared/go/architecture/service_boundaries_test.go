// Package architecture holds repository-wide boundary checks that no single
// service can make about the others.
package architecture

// Section 26 (Definition of Done): no new cross-database writes, and no
// service reaches into another service's code. Every service owns its schema
// and its module; other services reach it through its API, its events or a
// shared/go contract. This test derives each service's module path and
// database setting from the service itself, so a new service is covered
// without editing this file.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type service struct {
	dir      string
	module   string
	database []string
}

var databaseSetting = regexp.MustCompile(`"([A-Z][A-Z0-9_]*_DATABASE_URL)"`)

func servicesRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "services"))
	if _, err := os.Stat(root); err != nil {
		t.Skipf("services tree not available from %s", root)
	}
	return root
}

func loadServices(t *testing.T) []service {
	t.Helper()
	root := servicesRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []service
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		module := modulePath(filepath.Join(dir, "go.mod"))
		if module == "" {
			continue // not a Go service
		}
		svc := service{dir: dir, module: module}
		seen := map[string]bool{}
		for _, configDir := range []string{"internal/config", "cmd"} {
			walkGo(t, filepath.Join(dir, configDir), func(_ string, content string) {
				for _, match := range databaseSetting.FindAllStringSubmatch(content, -1) {
					name := match[1]
					if !strings.Contains(name, "_TEST_") && !seen[name] {
						seen[name] = true
						svc.database = append(svc.database, name)
					}
				}
			})
		}
		out = append(out, svc)
	}
	if len(out) < 10 {
		t.Fatalf("found only %d Go services; the boundary check would prove nothing", len(out))
	}
	return out
}

func modulePath(goMod string) string {
	file, err := os.Open(goMod)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// walkGo visits every production Go file under dir.
func walkGo(t *testing.T, dir string, visit func(path, content string)) {
	t.Helper()
	if _, err := os.Stat(dir); err != nil {
		return
	}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, string(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoServiceImportsAnotherServicesCode(t *testing.T) {
	services := loadServices(t)
	for _, owner := range services {
		walkGo(t, owner.dir, func(path, content string) {
			for _, other := range services {
				if other.module == owner.module {
					continue
				}
				if strings.Contains(content, `"`+other.module+`/`) || strings.Contains(content, `"`+other.module+`"`) {
					t.Errorf("%s imports %s: services talk through APIs, events and shared/go contracts", path, other.module)
				}
			}
		})
	}
}

func TestNoServiceIsConfiguredWithAnotherServicesDatabase(t *testing.T) {
	services := loadServices(t)
	owners := map[string]string{}
	for _, svc := range services {
		for _, name := range svc.database {
			owners[name] = svc.module
		}
	}
	if len(owners) < 10 {
		t.Fatalf("found only %d database settings; the boundary check would prove nothing", len(owners))
	}
	for _, svc := range services {
		walkGo(t, svc.dir, func(path, content string) {
			for _, match := range databaseSetting.FindAllStringSubmatch(content, -1) {
				if owner, known := owners[match[1]]; known && owner != svc.module {
					t.Errorf("%s reads %s, which belongs to %s: no service reads or writes another service's database",
						path, match[1], owner)
				}
			}
		})
	}
}
