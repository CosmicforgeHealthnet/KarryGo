package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMediaAssetsMigrationContainsRequiredTableAndIndexes(t *testing.T) {
	content, err := os.ReadFile("001_media_assets.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS media_assets",
		"CREATE INDEX IF NOT EXISTS idx_media_assets_owner",
		"CREATE INDEX IF NOT EXISTS idx_media_assets_purpose",
		"CREATE INDEX IF NOT EXISTS idx_media_assets_status",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %s", expected)
		}
	}
}
