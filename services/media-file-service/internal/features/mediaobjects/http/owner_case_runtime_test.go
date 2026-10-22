package http

import (
	"bytes"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerCaseEnrollmentRequiresFiniteOwnersAndDistinctAudienceKeys(t *testing.T) {
	dir := t.TempDir()
	caseKey := bytes.Repeat([]byte("c"), 32)
	sourceKey := bytes.Repeat([]byte("s"), 32)
	keyPath := filepath.Join(dir, "source.key")
	configPath := filepath.Join(dir, "sources.json")
	t.Setenv("MEDIA_ADMIN_CASE_SOURCES_FILE", "")
	if authority, err := ownerCaseAuthority(nil, nil, caseKey, nil, nil); err != nil || authority != nil {
		t.Fatal("missing enrollment manufactured an authority", err)
	}
	t.Setenv("MEDIA_ADMIN_CASE_SOURCES_FILE", configPath)
	for _, name := range []string{"valid", "fleet", "all-owners", "too-many", "case-key", "download-key", "legacy-key", "unknown-owner", "duplicate-owner", "path-origin", "unknown-field", "duplicate-json"} {
		t.Run(name, func(t *testing.T) {
			key := sourceKey
			if name == "case-key" {
				key = caseKey
			}
			if err := os.WriteFile(keyPath, []byte(base64.RawURLEncoding.EncodeToString(key)), 0600); err != nil {
				t.Fatal(err)
			}
			owner, origin := "customer-service", "http://127.0.0.1:12345"
			if name == "unknown-owner" {
				owner = "admin-backoffice-service"
			}
			if name == "fleet" {
				owner = "fleet-partner-service"
			}
			if name == "path-origin" {
				origin += "/arbitrary"
			}
			row := map[string]string{"owner": owner, "origin": origin, "secret_file": keyPath}
			rows := []map[string]string{row}
			if name == "duplicate-owner" {
				rows = append(rows, row)
			}
			if name == "all-owners" || name == "too-many" {
				// Every owner with a compiled case kind, each with its own key.
				rows = nil
				seen := map[string]bool{}
				for _, spec := range admincase.Specs() {
					if seen[spec.Owner] {
						continue
					}
					seen[spec.Owner] = true
					keyFile := filepath.Join(dir, spec.Owner+".key")
					if err := os.WriteFile(keyFile, []byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{byte('A' + len(rows))}, 32))), 0600); err != nil {
						t.Fatal(err)
					}
					rows = append(rows, map[string]string{"owner": spec.Owner, "origin": origin, "secret_file": keyFile})
				}
				if name == "too-many" {
					extra := map[string]string{"owner": "admin-backoffice-service", "origin": origin, "secret_file": keyPath}
					rows = append(rows, extra)
				}
			}
			raw, _ := json.Marshal(map[string]any{"sources": rows})
			if name == "unknown-field" {
				raw = append(raw[:len(raw)-1], []byte(`,"authority":"invented"}`)...)
			}
			if name == "duplicate-json" {
				raw = append(raw[:len(raw)-1], []byte(`,"sources":[]}`)...)
			}
			if err := os.WriteFile(configPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			var download []byte
			var legacy serviceauth.Secrets
			if name == "download-key" {
				download = sourceKey
			}
			if name == "legacy-key" {
				legacy = serviceauth.Secrets{"customer-service": sourceKey}
			}
			authority, err := ownerCaseAuthority(nil, nil, caseKey, download, legacy)
			if name == "valid" || name == "fleet" || name == "all-owners" {
				want := 1
				if name == "all-owners" {
					want = 5
				}
				if err != nil || len(authority.Sources) != want {
					t.Fatal(err)
				}
				if name == "fleet" && authority.Sources["fleet-partner-service"] == nil {
					t.Fatal("fleet case source not enrolled")
				}
				for _, client := range authority.Sources {
					client.(*admincase.Client).Close()
				}
			} else if err == nil {
				t.Fatal("unsafe source enrollment accepted")
			}
		})
	}
}
