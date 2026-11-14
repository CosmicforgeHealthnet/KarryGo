package uploadclients

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type countingMetrics struct {
	mu     sync.Mutex
	counts map[string]float64
}

func (m *countingMetrics) IncCounter(name string, _ map[string]string, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts == nil {
		m.counts = map[string]float64{}
	}
	m.counts[name] += delta
}

func testKey(id string, fill byte) EncryptionKey {
	return EncryptionKey{ID: id, Key: bytes.Repeat([]byte{fill}, 32)}
}

func newEncrypted(t *testing.T, inner PrivateStorage, metrics CounterRecorder, keys ...EncryptionKey) *EncryptingPrivateStorage {
	t.Helper()
	storage, err := NewEncryptingPrivateStorage(inner, keys, metrics)
	if err != nil {
		t.Fatalf("new encrypting storage: %v", err)
	}
	return storage
}

func newFilesystem(t *testing.T) (*FilesystemPrivateStorage, string) {
	t.Helper()
	dir := t.TempDir()
	fs, err := NewFilesystemPrivateStorage(dir)
	if err != nil {
		t.Fatalf("filesystem storage: %v", err)
	}
	return fs, dir
}

var plaintextExport = []byte("organization_id,driver,amount_kobo\norg_123,Ada Okafor,150000\norg_123,Bola Ade,98000\n")

func TestEncryptingPrivateStorage_RoundTrip(t *testing.T) {
	ctx := context.Background()
	fs, _ := newFilesystem(t)
	storage := newEncrypted(t, fs, nil, testKey("k1", 1))
	if err := storage.Put(ctx, "exports/a.csv", "text/csv", plaintextExport); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := storage.Get(ctx, "exports/a.csv")
	if err != nil || !bytes.Equal(got, plaintextExport) {
		t.Fatalf("round trip mismatch: %v %q", err, got)
	}
	empty := []byte{}
	if err := storage.Put(ctx, "exports/empty.csv", "text/csv", empty); err != nil {
		t.Fatalf("put empty: %v", err)
	}
	if got, err := storage.Get(ctx, "exports/empty.csv"); err != nil || len(got) != 0 {
		t.Fatalf("empty round trip: %v %q", err, got)
	}
}

func TestEncryptingPrivateStorage_StoredBytesAreCiphertext(t *testing.T) {
	ctx := context.Background()
	fs, dir := newFilesystem(t)
	storage := newEncrypted(t, fs, nil, testKey("k1", 1))
	if err := storage.Put(ctx, "exports/a.csv", "text/csv", plaintextExport); err != nil {
		t.Fatalf("put: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "exports", "a.csv"))
	if err != nil {
		t.Fatalf("read disk: %v", err)
	}
	if !bytes.HasPrefix(onDisk, encryptionMagic) {
		t.Fatalf("stored object lacks encryption header")
	}
	for _, fragment := range []string{"Ada Okafor", "org_123", "organization_id", "150000"} {
		if bytes.Contains(onDisk, []byte(fragment)) {
			t.Fatalf("plaintext fragment %q found on disk", fragment)
		}
	}
	// Two writes of the same content use fresh nonces.
	if err := storage.Put(ctx, "exports/b.csv", "text/csv", plaintextExport); err != nil {
		t.Fatalf("put b: %v", err)
	}
	other, _ := os.ReadFile(filepath.Join(dir, "exports", "b.csv"))
	if bytes.Equal(onDisk[len(onDisk)-len(plaintextExport)-16:], other[len(other)-len(plaintextExport)-16:]) {
		t.Fatalf("identical ciphertext for two writes: nonce reuse")
	}
}

func TestEncryptingPrivateStorage_TamperFails(t *testing.T) {
	ctx := context.Background()
	key := testKey("k1", 1)
	headerLen := len(encryptionHeader(key.ID))
	cases := map[string]func(b []byte){
		"version":    func(b []byte) { b[4] = 0x02 },
		"key id":     func(b []byte) { b[6] = 'x' },
		"id length":  func(b []byte) { b[5] = 3 },
		"nonce":      func(b []byte) { b[headerLen] ^= 0x01 },
		"ciphertext": func(b []byte) { b[headerLen+12+3] ^= 0x01 },
		"tag":        func(b []byte) { b[len(b)-1] ^= 0x01 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fs, dir := newFilesystem(t)
			storage := newEncrypted(t, fs, nil, key)
			if err := storage.Put(ctx, "o", "text/csv", plaintextExport); err != nil {
				t.Fatalf("put: %v", err)
			}
			path := filepath.Join(dir, "o")
			raw, _ := os.ReadFile(path)
			mutate(raw)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := storage.Get(ctx, "o")
			if err == nil {
				t.Fatalf("tampered object decrypted: %q", got)
			}
			if !errors.Is(err, ErrEncryptedObjectUnreadable) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	// A truncated object is refused too.
	fs, dir := newFilesystem(t)
	storage := newEncrypted(t, fs, nil, key)
	_ = storage.Put(ctx, "o", "text/csv", plaintextExport)
	raw, _ := os.ReadFile(filepath.Join(dir, "o"))
	_ = os.WriteFile(filepath.Join(dir, "o"), raw[:len(raw)-5], 0o600)
	if _, err := storage.Get(ctx, "o"); !errors.Is(err, ErrEncryptedObjectUnreadable) {
		t.Fatalf("truncated object: %v", err)
	}
	if _, err := storage.Stat(ctx, "o"); err == nil {
		t.Fatalf("stat of truncated object succeeded")
	}
}

// Destroying the magic makes the object look like legacy plaintext. It is then
// served exactly as stored, which is ciphertext: the plaintext never leaks.
func TestEncryptingPrivateStorage_DestroyedMagicNeverYieldsPlaintext(t *testing.T) {
	ctx := context.Background()
	fs, dir := newFilesystem(t)
	storage := newEncrypted(t, fs, nil, testKey("k1", 1))
	_ = storage.Put(ctx, "o", "text/csv", plaintextExport)
	raw, _ := os.ReadFile(filepath.Join(dir, "o"))
	raw[1] ^= 0xFF
	_ = os.WriteFile(filepath.Join(dir, "o"), raw, 0o600)
	got, _ := storage.Get(ctx, "o")
	if bytes.Contains(got, []byte("Ada Okafor")) {
		t.Fatalf("plaintext leaked")
	}
}

func TestEncryptingPrivateStorage_WrongObjectKeyFails(t *testing.T) {
	ctx := context.Background()
	fs, dir := newFilesystem(t)
	storage := newEncrypted(t, fs, nil, testKey("k1", 1))
	if err := storage.Put(ctx, "orgs/a/export.csv", "text/csv", plaintextExport); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "orgs", "a", "export.csv"))
	if err := fs.Put(ctx, "orgs/b/export.csv", "text/csv", raw); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Get(ctx, "orgs/b/export.csv"); !errors.Is(err, ErrEncryptedObjectUnreadable) {
		t.Fatalf("ciphertext copied to another key decrypted: %v", err)
	}
}

func TestEncryptingPrivateStorage_UnknownKeyIDFails(t *testing.T) {
	ctx := context.Background()
	fs, _ := newFilesystem(t)
	writer := newEncrypted(t, fs, nil, testKey("k1", 1))
	if err := writer.Put(ctx, "o", "text/csv", plaintextExport); err != nil {
		t.Fatal(err)
	}
	reader := newEncrypted(t, fs, nil, testKey("k2", 2))
	got, err := reader.Get(ctx, "o")
	if !errors.Is(err, ErrEncryptedObjectUnreadable) || got != nil {
		t.Fatalf("unknown key id: %v %q", err, got)
	}
	// Same id, different key material: authentication fails.
	impostor := newEncrypted(t, fs, nil, testKey("k1", 9))
	if _, err := impostor.Get(ctx, "o"); !errors.Is(err, ErrEncryptedObjectUnreadable) {
		t.Fatalf("wrong key material: %v", err)
	}
}

func TestEncryptingPrivateStorage_Rotation(t *testing.T) {
	ctx := context.Background()
	fs, dir := newFilesystem(t)
	k1, k2 := testKey("k1", 1), testKey("k2", 2)
	before := newEncrypted(t, fs, nil, k1)
	if err := before.Put(ctx, "old", "text/csv", plaintextExport); err != nil {
		t.Fatal(err)
	}
	after := newEncrypted(t, fs, nil, k2, k1)
	if after.ActiveKeyID() != "k2" {
		t.Fatalf("active key = %s", after.ActiveKeyID())
	}
	got, err := after.Get(ctx, "old")
	if err != nil || !bytes.Equal(got, plaintextExport) {
		t.Fatalf("old object after rotation: %v", err)
	}
	if err := after.Put(ctx, "new", "text/csv", plaintextExport); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "new"))
	if !bytes.HasPrefix(raw, encryptionHeader("k2")) {
		t.Fatalf("new write not sealed with k2")
	}
	// Once k1 is retired, objects it sealed are refused, not served raw.
	retired := newEncrypted(t, fs, nil, k2)
	if _, err := retired.Get(ctx, "old"); !errors.Is(err, ErrEncryptedObjectUnreadable) {
		t.Fatalf("retired key: %v", err)
	}
}

func TestEncryptingPrivateStorage_LegacyPlaintextReadable(t *testing.T) {
	ctx := context.Background()
	fs, _ := newFilesystem(t)
	if err := fs.Put(ctx, "legacy.pdf", "application/pdf", []byte("%PDF-1.7 legacy")); err != nil {
		t.Fatal(err)
	}
	metrics := &countingMetrics{}
	storage := newEncrypted(t, fs, metrics, testKey("k1", 1))
	got, err := storage.Get(ctx, "legacy.pdf")
	if err != nil || string(got) != "%PDF-1.7 legacy" {
		t.Fatalf("legacy read: %v %q", err, got)
	}
	size, err := storage.Stat(ctx, "legacy.pdf")
	if err != nil || size != int64(len("%PDF-1.7 legacy")) {
		t.Fatalf("legacy stat: %v %d", err, size)
	}
	if metrics.counts[MetricLegacyPlaintextReads] != 2 {
		t.Fatalf("legacy reads counted = %v", metrics.counts[MetricLegacyPlaintextReads])
	}
	_ = storage.Put(ctx, "new", "text/csv", plaintextExport)
	_, _ = storage.Get(ctx, "new")
	if metrics.counts[MetricLegacyPlaintextReads] != 2 {
		t.Fatalf("encrypted read counted as legacy")
	}
}

func TestEncryptingPrivateStorage_StatReturnsPlaintextSize(t *testing.T) {
	ctx := context.Background()
	fs, dir := newFilesystem(t)
	storage := newEncrypted(t, fs, nil, testKey("rotation-2026.11", 1))
	if err := storage.Put(ctx, "o", "text/csv", plaintextExport); err != nil {
		t.Fatal(err)
	}
	size, err := storage.Stat(ctx, "o")
	if err != nil || size != int64(len(plaintextExport)) {
		t.Fatalf("stat = %d, %v; want %d", size, err, len(plaintextExport))
	}
	raw, _ := fs.Stat(ctx, "o")
	if raw == size {
		t.Fatalf("backend size equals plaintext size; object not encrypted")
	}
	_ = dir
	if _, err := storage.Stat(ctx, "missing"); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("missing stat: %v", err)
	}
	if _, err := storage.Get(ctx, "missing"); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("missing get: %v", err)
	}
	if err := storage.Delete(ctx, "o"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(ctx, "o"); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("delete did not pass through")
	}
	if err := storage.Check(ctx); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestParseEncryptionKeys(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	other := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))
	keys, err := ParseEncryptionKeys(" k2:" + good + " , k1:" + other)
	if err != nil || len(keys) != 2 || keys[0].ID != "k2" || keys[1].ID != "k1" {
		t.Fatalf("parse: %v %+v", err, keys)
	}
	if keys, err := ParseEncryptionKeys("  "); err != nil || keys != nil {
		t.Fatalf("empty: %v %v", keys, err)
	}
	short := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 16))
	bad := map[string]string{
		"wrong length":   "k1:" + short,
		"bad base64":     "k1:not*base64",
		"duplicate ids":  "k1:" + good + ",k1:" + other,
		"missing colon":  "k1" + good,
		"empty id":       ":" + good,
		"bad id chars":   "k 1:" + good,
		"empty entry":    "k1:" + good + ",",
		"overlong id":    strings.Repeat("a", 65) + ":" + good,
		"raw hex secret": "k1:" + strings.Repeat("ab", 32),
	}
	for name, raw := range bad {
		if _, err := ParseEncryptionKeys(raw); err == nil {
			t.Errorf("%s: accepted %q", name, raw)
		} else if strings.Contains(err.Error(), good) || strings.Contains(err.Error(), other) {
			t.Errorf("%s: error leaks key material", name)
		}
	}
}

func TestNewEncryptingPrivateStorageRejectsMissingKeys(t *testing.T) {
	fs, _ := newFilesystem(t)
	if _, err := NewEncryptingPrivateStorage(fs, nil, nil); err == nil {
		t.Fatal("no keys accepted")
	}
	if _, err := NewEncryptingPrivateStorage(nil, []EncryptionKey{testKey("k1", 1)}, nil); err == nil {
		t.Fatal("nil inner accepted")
	}
	if _, err := NewEncryptingPrivateStorage(fs, []EncryptionKey{{ID: "k1", Key: []byte("short")}}, nil); err == nil {
		t.Fatal("short key accepted")
	}
}
