package adminresources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminowner"
)

func migration(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testdb.MigrationsDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// The owner tables are the shared templates, byte for byte: a drifted copy
// would silently weaken append-only evidence or the decision ledger.
func TestSecureAdminMigrationEmbedsSharedTemplatesVerbatim(t *testing.T) {
	up := migration(t, "000004_secure_admin_boundary.up.sql")
	for name, template := range map[string]string{"admincontext": admincontext.Schema, "adminowner": adminowner.Schema, "admincommand": admincommand.Schema, "adminevents outbox": adminevents.OutboxSchema} {
		if !strings.Contains(up, strings.ReplaceAll(template, "\r\n", "\n")) {
			t.Fatalf("migration does not embed the %s template verbatim", name)
		}
	}
	if !strings.Contains(up, "-- shared-template-sha256: "+admincommand.SchemaSHA256()+"\n") {
		t.Fatal("admincommand template checksum line missing or stale")
	}
	for _, seeded := range []string{"INSERT INTO verification_admin_staff_grants", "INSERT INTO admin_owner_policy_revisions", "INSERT INTO admin_owner_context", "INSERT INTO vendor_case_appeals"} {
		if strings.Contains(up, seeded) {
			t.Fatalf("migration seeds %s", seeded)
		}
	}
	if !strings.Contains(up, "'verification.vendor_case.changed.v1'") || !strings.Contains(up, "'vendor_verification_case'") {
		t.Fatal("admin event trigger does not emit the registered event")
	}
	down := migration(t, "000004_secure_admin_boundary.down.sql")
	if !strings.Contains(down, "RAISE EXCEPTION") {
		t.Fatal("down migration must refuse to discard decision evidence")
	}
}

func TestDictionaryCoversProfileAndPermissions(t *testing.T) {
	profile := adminowner.VerificationProfile()
	source := Source()
	if len(source.Specs) != len(profile.Definitions) {
		t.Fatal("source dictionary incomplete")
	}
	for kind, definition := range profile.Definitions {
		if ReadPermission(kind) != definition.Capability {
			t.Fatalf("%s native grant %q does not match capability %q", kind, ReadPermission(kind), definition.Capability)
		}
	}
}

func TestRuntimeDefaultsOffWithoutTouchingCredentials(t *testing.T) {
	for _, key := range []string{"_ADMIN_RESOURCE_READ_ENABLED", "_ADMIN_CONTEXT_PUBLISH_ENABLED", "_ADMIN_CASE_SOURCE_READ_ENABLED", "_ADMIN_CASE_CONTEXT_PUBLISH_ENABLED", "_ADMIN_COMMAND_WRITES_ENABLED", "_ADMIN_COMMAND_READBACK_ENABLED", "_ADMIN_EVENTS_ENABLED"} {
		t.Setenv(Prefix+key, "")
	}
	h, err := Runtime(context.Background(), nil)
	if err != nil || h.Enabled || h.PublishEnabled {
		t.Fatalf("owner runtime must default off: %+v %v", h, err)
	}
	commands, err := admincommand.Runtime(h, admincommand.VerificationSpec(), nil, Prefix)
	if err != nil || commands.Service.WriteEnabled || commands.Service.ReadEnabled {
		t.Fatalf("command runtime must default off: %v", err)
	}
	relay, err := adminevents.RelayFromEnvironment(nil, Prefix, Owner)
	if err != nil || relay != nil {
		t.Fatalf("relay must default off: %v", err)
	}
}
