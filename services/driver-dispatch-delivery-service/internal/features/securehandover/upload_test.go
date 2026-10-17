package securehandover

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestCaptureContentRejectsUnsafeOrIncompleteFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		size int64
	}{
		{"empty", nil, 0},
		{"too large", []byte("x"), maxCaptureBytes + 1},
		{"truncated", []byte("x"), 10},
		{"PDF pretending to be a photo", []byte("%PDF-1.7 unsafe"), 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := captureContent(bytes.NewReader(tc.data), tc.size); err == nil {
				t.Fatal("invalid evidence file was accepted")
			}
		})
	}
}

func TestCaptureContentAcceptsPNGByBytes(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l1cAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	got, mime, err := captureContent(bytes.NewReader(data), int64(len(data)))
	if err != nil || mime != "image/png" || !bytes.Equal(got, data) {
		t.Fatalf("PNG evidence rejected: mime=%q err=%v", mime, err)
	}
}
