package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type bulkMediaAuthorityStub struct {
	state   platformclients.MediaObjectState
	content []byte
	reads   int
}

func (s *bulkMediaAuthorityStub) AuthorizeUpload(context.Context, platformclients.CallMeta, platformclients.MediaUploadRequest) (platformclients.MediaUploadGrant, error) {
	return platformclients.MediaUploadGrant{MediaObjectID: "report", UploadToken: "opaque"}, nil
}
func (s *bulkMediaAuthorityStub) CompleteUpload(context.Context, platformclients.CallMeta, platformclients.MediaUploadGrant, string, string, []byte) (platformclients.MediaObjectState, error) {
	return platformclients.MediaObjectState{MediaObjectID: "report"}, nil
}
func (s *bulkMediaAuthorityStub) ReadObject(context.Context, platformclients.CallMeta, string) (platformclients.MediaObjectState, error) {
	return s.state, nil
}
func (s *bulkMediaAuthorityStub) AuthorizeDownload(context.Context, platformclients.CallMeta, string) (platformclients.MediaDownloadGrant, error) {
	s.reads++
	return platformclients.MediaDownloadGrant{MediaObjectID: s.state.MediaObjectID, Token: "opaque"}, nil
}
func (s *bulkMediaAuthorityStub) ReadContent(context.Context, platformclients.CallMeta, platformclients.MediaDownloadGrant, int64) ([]byte, error) {
	s.reads++
	return s.content, nil
}
func (s *bulkMediaAuthorityStub) RequestDeletion(context.Context, platformclients.CallMeta, string, string) (platformclients.MediaDeletionOutcome, error) {
	return platformclients.MediaDeletionOutcome{}, errors.New("unused")
}
func (s *bulkMediaAuthorityStub) ReadDeletionState(context.Context, platformclients.CallMeta, string) (platformclients.MediaDeletionState, error) {
	return platformclients.MediaDeletionState{}, errors.New("unused")
}
func (s *bulkMediaAuthorityStub) SetRetention(context.Context, platformclients.CallMeta, string, string, *time.Time, *bool) (platformclients.MediaObjectState, error) {
	return platformclients.MediaObjectState{}, errors.New("unused")
}
func (*bulkMediaAuthorityStub) AuthorityService() string { return "media-file-service" }
func (*bulkMediaAuthorityStub) Configured() bool         { return true }

func TestBulkMediaAdapterEnforcesOwnershipPurposeSafetyAndReadiness(t *testing.T) {
	tenantCtx := tenant.Context{OrganizationID: "org_bulk"}
	valid := platformclients.MediaObjectState{MediaObjectID: "med_bulk", OrganizationID: tenantCtx.OrganizationID,
		UploadPurpose: "BULK_ORDER_IMPORT", UploadStatus: "COMPLETED", SafetyStatus: "SAFE",
		DetectedMIME: "text/csv", SizeBytes: 20}
	tests := []struct {
		name   string
		mutate func(*platformclients.MediaObjectState)
		ok     bool
	}{
		{name: "valid", ok: true},
		{name: "foreign owner", mutate: func(s *platformclients.MediaObjectState) { s.OrganizationID = "org_other" }},
		{name: "wrong purpose", mutate: func(s *platformclients.MediaObjectState) { s.UploadPurpose = "PROFILE_PHOTO" }},
		{name: "unsafe", mutate: func(s *platformclients.MediaObjectState) { s.SafetyStatus = "UNSAFE" }},
		{name: "not ready", mutate: func(s *platformclients.MediaObjectState) { s.UploadStatus = "PROCESSING" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := valid
			if tc.mutate != nil {
				tc.mutate(&state)
			}
			stub := &bulkMediaAuthorityStub{state: state, content: []byte("rowReference\nrow-1\n")}
			file, err := (bulkMediaAdapter{media: stub}).LoadImport(context.Background(), tenantCtx, state.MediaObjectID, 1024)
			if tc.ok {
				if err != nil || file.Format != "CSV" || stub.reads != 2 {
					t.Fatalf("file=%+v reads=%d err=%v", file, stub.reads, err)
				}
				return
			}
			if !errors.Is(err, platformclients.ErrNotConfigured) || stub.reads != 0 {
				t.Fatalf("unauthorized media err=%v reads=%d", err, stub.reads)
			}
		})
	}
}
