package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func captureVendorAdminWire(t *testing.T, name string, value any) {
	t.Helper()
	directory := os.Getenv("VENDOR_SERVICE_TEST_WIRE_DIR")
	if directory == "" {
		return
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, name), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
