package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestCustomerReviewConfigurationSeparatesAudiencesAndReadback(t *testing.T) {
	dir := t.TempDir()
	file := func(name string, b byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	resource := file("resource.key", 11)
	write := file("write.key", 12)
	read := file("read.key", 13)
	c := Config{OwnerResources: []OwnerResources{{Owner: "customer-service", Origin: "http://127.0.0.1:12345", SecretFile: resource}}, Trust: Trust{ServiceKeys: map[string]string{"hq": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{14}, 32))}}}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED", "false")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED", "false")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE", "missing")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_SECRET_FILE", "missing")
	if client, _, _, err := c.CustomerReviewClient(); err != nil || client != nil {
		t.Fatal("off loaded credentials", err)
	}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED", "true")
	if _, _, _, err := c.CustomerReviewClient(); err == nil {
		t.Fatal("write without readback")
	}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED", "true")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE", write)
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_SECRET_FILE", read)
	client, w, r, err := c.CustomerReviewClient()
	if err != nil || !w || !r {
		t.Fatal(err)
	}
	client.Close()
	for _, path := range []string{resource, read, file("hq.key", 14)} {
		t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE", path)
		if _, _, _, err := c.CustomerReviewClient(); err == nil {
			t.Fatal("credential audience reused")
		}
	}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED", "false")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE", "missing")
	client, w, r, err = c.CustomerReviewClient()
	if err != nil || w || !r {
		t.Fatal("readback-only restart failed", err)
	}
	client.Close()
	c.OwnerResources = nil
	if _, _, _, err := c.CustomerReviewClient(); err == nil {
		t.Fatal("origin invented without canonical source configuration")
	}
}

func TestCustomerWorkItemFlagsDoNotEnableIntakeOrBypassReadback(t *testing.T) {
	c := Config{}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED", "false")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED", "false")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_WRITES_ENABLED", "false")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_READBACK_ENABLED", "false")
	if client, write, read, err := c.CustomerReviewClient(); client != nil || write || read || err != nil {
		t.Fatal("off read keys", err)
	}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_WRITES_ENABLED", "true")
	if _, _, _, err := c.CustomerReviewClient(); err == nil {
		t.Fatal("work item writes bypassed readback")
	}
}
