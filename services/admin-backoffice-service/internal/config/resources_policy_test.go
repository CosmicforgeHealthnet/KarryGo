package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
)

func policyFiles(t *testing.T, f *testfixture.Fixture) (string, string) {
	t.Helper()
	dir := t.TempDir()
	document, keys := filepath.Join(dir, "policy.json"), filepath.Join(dir, "keys.json")
	if err := os.WriteFile(document, f.Envelope, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keys, f.Keys, 0600); err != nil {
		t.Fatal(err)
	}
	return document, keys
}

func TestOwnerResourcePolicyConfiguration(t *testing.T) {
	for _, profile := range adminowner.Profiles() {
		t.Setenv("ADMIN_"+profile.Prefix+"_RESOURCE_READ_ENABLED", "false")
	}
	prefix := "ADMIN_CUSTOMER"
	t.Setenv(prefix+"_ACCESS_POLICY_FILE", "missing")
	if entries, err := loadOwnerResources(); err != nil || len(entries) != 0 {
		t.Fatal("disabled owner loaded policy", err)
	}
	t.Setenv(prefix+"_RESOURCE_READ_ENABLED", "true")
	t.Setenv(prefix+"_ORIGIN", "http://customer:8101")
	t.Setenv(prefix+"_RESOURCE_SECRET_FILE", "read.key")
	if _, err := loadOwnerResources(); err == nil {
		t.Fatal("partial explicit policy accepted")
	}
	t.Setenv(prefix+"_ACCESS_POLICY_KEYS_FILE", "governance.json")
	entries, err := loadOwnerResources()
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	if entries[0].AccessPolicyFile != "missing" || entries[0].AccessPolicyKeysFile != "governance.json" {
		t.Fatal("owner policy selection lost")
	}
	t.Setenv(prefix+"_ACCESS_POLICY_FILE", "")
	if _, err := loadOwnerResources(); err == nil {
		t.Fatal("keys without policy accepted")
	}
}

func TestExplicitOwnerPolicyNeverFallsBack(t *testing.T) {
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	fallback := testfixture.New(t, scope, "customer-service", "customer.directory.read", "customer.admin-resources.v1")
	fallback.Document.ID = "test-only-legacy"
	fallback.Sign(t)
	defaultFile, defaultKeys := policyFiles(t, fallback)
	owner := OwnerResources{Owner: "customer-service"}
	p, err := owner.LoadPolicy(defaultFile, defaultKeys, adminpolicy.TestOnly, time.Now())
	if err != nil || p.ID() != fallback.Engine.ID() {
		t.Fatal("legacy fallback failed", err)
	}
	explicit := testfixture.New(t, scope, "customer-service", "customer.directory.read", "customer.admin-resources.v1")
	explicit.Document.ID = "test-only-customer"
	explicit.Sign(t)
	owner.AccessPolicyFile, owner.AccessPolicyKeysFile = policyFiles(t, explicit)
	p, err = owner.LoadPolicy("missing", "missing", adminpolicy.TestOnly, time.Now())
	if err != nil || p.ID() != explicit.Engine.ID() {
		t.Fatal("explicit owner policy did not stand alone", err)
	}
	original := owner
	owner.AccessPolicyFile = "missing"
	if _, err := owner.LoadPolicy(defaultFile, defaultKeys, adminpolicy.TestOnly, time.Now()); err == nil {
		t.Fatal("unreadable explicit policy fell back")
	}
	owner = original
	owner.AccessPolicyKeysFile = defaultKeys
	if _, err := owner.LoadPolicy(defaultFile, defaultKeys, adminpolicy.TestOnly, time.Now()); err == nil {
		t.Fatal("wrong explicit key fell back")
	}
	owner = original
	owner.Owner = "driver-hauling-service"
	if _, err := owner.LoadPolicy(defaultFile, defaultKeys, adminpolicy.TestOnly, time.Now()); err == nil {
		t.Fatal("wrong owner accepted another owner's policy")
	}
	owner = original
	tampered := strings.Replace(string(explicit.Envelope), "TEST_ONLY_OWNER_APPROVAL", "TAMPERED_OWNER_APPROVAL", 1)
	if err := os.WriteFile(owner.AccessPolicyFile, []byte(tampered), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.LoadPolicy(defaultFile, defaultKeys, adminpolicy.TestOnly, time.Now()); err == nil {
		t.Fatal("tampered policy fell back")
	}
	owner.AccessPolicyFile = ""
	owner.AccessPolicyKeysFile = ""
	if _, err := owner.LoadPolicy("", "", adminpolicy.TestOnly, time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("missing policies did not block", err)
	}
}

func TestFullOwnerReadInventoryUsesIndependentlySignedBoundedPolicies(t *testing.T) {
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	count := 0
	for _, profile := range adminowner.Profiles() {
		t.Run(profile.Domain, func(t *testing.T) {
			var first adminowner.Definition
			for _, definition := range profile.Definitions {
				first = definition
				break
			}
			f := testfixture.New(t, scope, profile.Owner, first.Capability, first.SourceContract)
			base := f.Document.Rules[0]
			f.Document.ID = "test-only-" + profile.Domain
			f.Document.Rules = nil
			seen := map[string]bool{}
			for _, definition := range profile.Definitions {
				if seen[definition.Capability] {
					continue
				}
				seen[definition.Capability] = true
				rule := base
				rule.Capability = definition.Capability
				rule.Permission = definition.Capability
				rule.SourceContract = definition.SourceContract
				f.Document.Rules = append(f.Document.Rules, rule)
			}
			count += len(f.Document.Rules)
			f.Sign(t)
			owner := OwnerResources{Owner: profile.Owner}
			owner.AccessPolicyFile, owner.AccessPolicyKeysFile = policyFiles(t, f)
			p, err := owner.LoadPolicy("", "", adminpolicy.TestOnly, time.Now())
			if err != nil || p.Checksum() != f.Engine.Checksum() {
				t.Fatal("source policy failed", err)
			}
			for _, definition := range profile.Definitions {
				if _, err := p.Rule(definition.Capability, time.Now()); err != nil {
					t.Fatal(definition.Capability, err)
				}
			}
		})
	}
	if count <= 128 {
		t.Fatal("test did not cover an inventory larger than a single policy", count)
	}
}
