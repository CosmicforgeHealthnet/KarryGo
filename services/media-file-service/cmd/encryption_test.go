package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"cosmicforge/logistics/services/media-file-service/internal/config"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
)

func TestEncryptPrivateStorage_FailsClosedInSharedEnvironments(t *testing.T) {
	inner, err := uploadclients.NewFilesystemPrivateStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"staging", "production", "Production", "prod"} {
		if _, err := encryptPrivateStorage(config.Config{AppEnv: env}, inner); !errors.Is(err, errMissingEncryptionKeys) {
			t.Errorf("APP_ENV=%s without keys: err = %v, want refusal", env, err)
		}
	}
	for _, env := range []string{"development", "test", ""} {
		got, err := encryptPrivateStorage(config.Config{AppEnv: env}, inner)
		if err != nil || got != uploadclients.PrivateStorage(inner) {
			t.Errorf("APP_ENV=%q without keys must keep working unencrypted: %v", env, err)
		}
	}
}

func TestEncryptPrivateStorage_MalformedKeysAlwaysRefuse(t *testing.T) {
	inner, _ := uploadclients.NewFilesystemPrivateStorage(t.TempDir())
	for _, env := range []string{"development", "staging"} {
		if _, err := encryptPrivateStorage(config.Config{AppEnv: env, PrivateEncryptionKeys: "k1:short"}, inner); err == nil {
			t.Errorf("APP_ENV=%s accepted a malformed key list", env)
		}
	}
}

func TestEncryptPrivateStorage_WrapsWhenKeysConfigured(t *testing.T) {
	inner, _ := uploadclients.NewFilesystemPrivateStorage(t.TempDir())
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	got, err := encryptPrivateStorage(config.Config{AppEnv: "production", PrivateEncryptionKeys: "k1:" + key}, inner)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, ok := got.(*uploadclients.EncryptingPrivateStorage)
	if !ok || encrypted.ActiveKeyID() != "k1" {
		t.Fatalf("storage not wrapped: %T", got)
	}
	ctx := context.Background()
	_ = got.Put(ctx, "o", "text/csv", []byte("a,b\n"))
	raw, _ := inner.Get(ctx, "o")
	if bytes.Contains(raw, []byte("a,b")) {
		t.Fatalf("backend holds plaintext")
	}
}

func TestBuildPrivateMediaService_RefusesUnencryptedStaging(t *testing.T) {
	cfg := config.Config{AppEnv: "staging", PrivateAPIEnabled: true, DownloadSecret: []byte("s"),
		PrivateStorageDir: t.TempDir()}
	if _, _, err := buildPrivateMediaService(cfg, nil, nil); !errors.Is(err, errMissingEncryptionKeys) {
		t.Fatalf("err = %v, want refusal", err)
	}
	cfg.PrivateAPIEnabled = false
	if svc, storage, err := buildPrivateMediaService(cfg, nil, nil); err != nil || svc != nil || storage != nil {
		t.Fatalf("disabled private API must not require keys: %v", err)
	}
}
