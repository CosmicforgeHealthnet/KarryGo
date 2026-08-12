package section31contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMandatoryCIProvisionsDependenciesAndEnforcesEveryGate(t *testing.T) {
	root := repositoryRoot(t)
	workflow, err := os.ReadFile(filepath.Join(root, "..", "..", ".github", "workflows", "section31-mandatory.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"postgres:16", "redis:7", "section31-testgate", "section31contract", "section30contract",
		"TestSQLAlwaysTenantScoped", "TestSection26RealRedisTransactionalDedupDeadLetterAndReplay",
		"TestCapacity_", "flutter test", "go vet ./...", "go build ./...", "git diff --check",
		"private-key", "untracked-env", "upload-artifact", "-parallel 4", "FLEET_PARTNER_TEST_DATABASE_URL",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("mandatory workflow lacks %q", required)
		}
	}
	if strings.Contains(text, "continue-on-error: true") {
		t.Error("mandatory workflow contains continue-on-error: true")
	}
}
