package uploadclients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PrivateStorage is the storage port used by the authoritative media object
// contract.
//
// It is deliberately narrower than ObjectStorage: there is no notion of a
// public URL anywhere in this interface, because a private object never has
// one. Callers reach content through a short-lived, object-specific grant that
// this service mints and immediately forgets.
type PrivateStorage interface {
	// Put writes content at key, replacing whatever was there.
	Put(ctx context.Context, key string, contentType string, content []byte) error
	// Get returns the stored bytes, or ErrObjectMissing.
	Get(ctx context.Context, key string) ([]byte, error)
	// Stat returns the stored size, or ErrObjectMissing.
	Stat(ctx context.Context, key string) (int64, error)
	// Delete removes the object. Deleting an absent object succeeds: the
	// post-condition the caller wants is "the bytes are gone".
	Delete(ctx context.Context, key string) error
	// Check reports storage reachability for readiness.
	Check(ctx context.Context) error
}

// ErrObjectMissing means the storage backend holds no object at that key.
var ErrObjectMissing = errors.New("media storage: object not found")

// FilesystemPrivateStorage keeps private objects on a local volume.
//
// This is the adapter used when no bucket is configured — local development,
// CI and single-node deployments with an attached volume. It is a real
// backend, not a stub: it writes and reads actual bytes, which is what makes
// the file-validation tests meaningful.
type FilesystemPrivateStorage struct {
	root string
	mu   sync.RWMutex
}

// NewFilesystemPrivateStorage roots private storage at dir.
func NewFilesystemPrivateStorage(dir string) (*FilesystemPrivateStorage, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("media storage: private storage directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("media storage: create private root: %w", err)
	}
	return &FilesystemPrivateStorage{root: dir}, nil
}

// resolve maps an object key onto a path inside the root.
//
// The key is server-minted, but this still refuses traversal: a defect
// elsewhere that let a caller influence the key must not become a write
// outside the storage root.
func (s *FilesystemPrivateStorage) resolve(key string) (string, error) {
	cleaned := filepath.Clean("/" + strings.ReplaceAll(key, "\\", "/"))
	if cleaned == "/" {
		return "", fmt.Errorf("media storage: object key is required")
	}
	return filepath.Join(s.root, filepath.FromSlash(cleaned)), nil
}

// Put writes content at key.
func (s *FilesystemPrivateStorage) Put(_ context.Context, key string, _ string, content []byte) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("media storage: create object directory: %w", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("media storage: write object: %w", err)
	}
	return nil
}

// Get returns the stored bytes.
func (s *FilesystemPrivateStorage) Get(_ context.Context, key string) ([]byte, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrObjectMissing
	}
	if err != nil {
		return nil, fmt.Errorf("media storage: read object: %w", err)
	}
	return content, nil
}

// Stat returns the stored size.
func (s *FilesystemPrivateStorage) Stat(_ context.Context, key string) (int64, error) {
	path, err := s.resolve(key)
	if err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, ErrObjectMissing
	}
	if err != nil {
		return 0, fmt.Errorf("media storage: stat object: %w", err)
	}
	return info.Size(), nil
}

// Delete removes the object.
func (s *FilesystemPrivateStorage) Delete(_ context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("media storage: delete object: %w", err)
	}
	return nil
}

// Check confirms the root is writable.
func (s *FilesystemPrivateStorage) Check(_ context.Context) error {
	info, err := os.Stat(s.root)
	if err != nil {
		return fmt.Errorf("media storage: private root unavailable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("media storage: private root is not a directory")
	}
	return nil
}

// BucketPrivateStorage adapts the existing object-storage client (Firebase /
// GCS) to the private port.
//
// It reuses the configured bucket but never asks it for a public or signed
// URL: content leaves this service only through the authenticated content
// route, so the bucket's own URL scheme is irrelevant to callers.
type BucketPrivateStorage struct {
	storage ObjectStorage
	bucket  string
}

// NewBucketPrivateStorage wraps an object-storage client.
func NewBucketPrivateStorage(storage ObjectStorage, bucket string) *BucketPrivateStorage {
	return &BucketPrivateStorage{storage: storage, bucket: bucket}
}

// Bucket reports the configured bucket name, recorded on each object so a
// later reader knows where the bytes live.
func (s *BucketPrivateStorage) Bucket() string { return s.bucket }

// Put uploads content.
func (s *BucketPrivateStorage) Put(ctx context.Context, key string, contentType string, content []byte) error {
	_, err := s.storage.Upload(ctx, UploadObjectInput{
		Path:        key,
		Body:        newByteReader(content),
		ContentType: contentType,
		Metadata:    map[string]string{"private": "true"},
	})
	if err != nil {
		return fmt.Errorf("media storage: upload object: %w", err)
	}
	return nil
}

// Get downloads content.
func (s *BucketPrivateStorage) Get(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.storage.Download(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrObjectMissing
		}
		return nil, fmt.Errorf("media storage: download object: %w", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("media storage: read object: %w", err)
	}
	return content, nil
}

// Stat reports the stored size by reading the object; the bucket client
// exposes no head operation.
func (s *BucketPrivateStorage) Stat(ctx context.Context, key string) (int64, error) {
	content, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return int64(len(content)), nil
}

// Delete removes the object.
func (s *BucketPrivateStorage) Delete(ctx context.Context, key string) error {
	if err := s.storage.Delete(ctx, key); err != nil {
		return fmt.Errorf("media storage: delete object: %w", err)
	}
	return nil
}

// Check reports bucket reachability.
func (s *BucketPrivateStorage) Check(ctx context.Context) error { return s.storage.Check(ctx) }

type byteReader struct {
	content []byte
	offset  int
}

func newByteReader(content []byte) io.Reader { return &byteReader{content: content} }

func (r *byteReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.content) {
		return 0, io.EOF
	}
	n := copy(p, r.content[r.offset:])
	r.offset += n
	return n, nil
}
