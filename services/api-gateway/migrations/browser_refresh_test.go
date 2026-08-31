package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestRefreshCookieMigrationShape(t *testing.T) {
	data, err := os.ReadFile("010_partner_refresh_cookie.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, s := range []string{"ALTER TABLE partner_sessions", "refresh_token_hash", "UNIQUE INDEX", "WHERE refresh_token_hash IS NOT NULL", "CHECK"} {
		if !strings.Contains(sql, s) {
			t.Fatalf("missing %s", s)
		}
	}
	down, err := os.ReadFile("010_partner_refresh_cookie_down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(down), "DROP TABLE") || !strings.Contains(string(down), "DROP COLUMN IF EXISTS refresh_token_hash") {
		t.Fatal("unsafe refresh rollback")
	}
}
