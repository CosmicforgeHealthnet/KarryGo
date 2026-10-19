package provideruploadsusecases

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/mediaclient"
)

type fakeMedia struct {
	got  mediaclient.UploadRequest
	body []byte
	err  error
}

func (f *fakeMedia) Upload(_ context.Context, r mediaclient.UploadRequest) (mediaclient.MediaAsset, error) {
	f.got = r
	f.body, _ = io.ReadAll(r.Body)
	if f.err != nil {
		return mediaclient.MediaAsset{}, f.err
	}
	return mediaclient.MediaAsset{ID: "asset-1", PublicURL: "https://files/asset-1"}, nil
}

var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, bytes.Repeat([]byte{1}, 64)...)

func status(err error) int {
	if e := apperrors.From(err); e != nil {
		return e.Status
	}
	return 0
}

func TestUpload_ForwardsUnderTheDriversOwnID(t *testing.T) {
	media := &fakeMedia{}
	res, err := NewUploadService(media).Upload(context.Background(), UploadInput{
		ProviderID: "driver-1", Purpose: "document_file", Filename: "licence.exe", Body: bytes.NewReader(jpeg),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "asset-1" || res.URL != "https://files/asset-1" {
		t.Fatalf("unexpected result %+v", res)
	}
	if media.got.OwnerID != "driver-1" || media.got.Purpose != "document_file" {
		t.Fatalf("owner/purpose not forwarded: %+v", media.got)
	}
	// The type comes from the bytes, not the client's filename.
	if media.got.ContentType != "image/jpeg" || !bytes.Equal(media.body, jpeg) {
		t.Fatalf("content not forwarded as sniffed jpeg: %q", media.got.ContentType)
	}
}

func TestUpload_RefusesBadInput(t *testing.T) {
	svc := NewUploadService(&fakeMedia{})
	cases := map[string]UploadInput{
		"unknown purpose": {ProviderID: "d", Purpose: "anything", Body: bytes.NewReader(jpeg)},
		"empty file":      {ProviderID: "d", Purpose: "document_file", Body: bytes.NewReader(nil)},
		"not an image":    {ProviderID: "d", Purpose: "document_file", Body: bytes.NewReader([]byte("#!/bin/sh\nrm -rf /\n"))},
		"too large":       {ProviderID: "d", Purpose: "document_file", Body: io.MultiReader(bytes.NewReader(jpeg), bytes.NewReader(make([]byte, MaxUploadBytes)))},
	}
	for name, in := range cases {
		if _, err := svc.Upload(context.Background(), in); status(err) != 422 && status(err) != 400 {
			t.Errorf("%s: want a validation error, got %v", name, err)
		}
	}
}

func TestUpload_UnavailableWithoutMediaOrWhenMediaFails(t *testing.T) {
	in := UploadInput{ProviderID: "d", Purpose: "profile_photo", Body: bytes.NewReader(jpeg)}
	var unconfigured *UploadService
	if _, err := unconfigured.Upload(context.Background(), in); status(err) != 503 {
		t.Fatalf("want 503 without media, got %v", err)
	}
	in.Body = bytes.NewReader(jpeg)
	if _, err := NewUploadService(&fakeMedia{err: errors.New("down")}).Upload(context.Background(), in); status(err) != 503 {
		t.Fatalf("want 503 when media fails, got %v", err)
	}
}
