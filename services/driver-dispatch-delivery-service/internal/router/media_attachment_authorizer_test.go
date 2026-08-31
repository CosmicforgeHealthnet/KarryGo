package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/mediaclient"
)

func TestMediaAttachmentAuthorizerMissionAndParcelBinding(t *testing.T) {
	assets := map[string]mediaclient.MediaAsset{
		"valid": {
			ID: "valid", OwnerService: dispatchMediaOwnerService, OwnerID: "mission-a",
			Purpose: "proof_image", Status: "active",
			Metadata: map[string]interface{}{"mission_ref": "mission-a", "parcel_refs": []interface{}{"parcel-a"}},
		},
		"mission-b": {
			ID: "mission-b", OwnerService: dispatchMediaOwnerService, OwnerID: "mission-b",
			Purpose: "proof_image", Status: "active", Metadata: map[string]interface{}{"mission_ref": "mission-b"},
		},
		"parcel-b": {
			ID: "parcel-b", OwnerService: dispatchMediaOwnerService, OwnerID: "mission-a",
			Purpose: "proof_image", Status: "active",
			Metadata: map[string]interface{}{"mission_ref": "mission-a", "parcel_ref": "parcel-b"},
		},
		"inactive": {
			ID: "inactive", OwnerService: dispatchMediaOwnerService, OwnerID: "mission-a",
			Purpose: "proof_image", Status: "deleted",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/files/")
		asset, found := assets[id]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"code": "not_found", "message": "not found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": asset})
	}))
	defer server.Close()

	authorizer := newMediaAttachmentAuthorizer(mediaclient.New(mediaclient.Config{
		BaseURL: server.URL, ServiceName: dispatchMediaOwnerService, Token: "test", HTTPClient: server.Client(),
	}))
	tests := []struct {
		name      string
		mission   string
		parcels   []string
		mediaRefs []string
		wantError bool
	}{
		{"valid exact binding", "mission-a", []string{"parcel-a"}, []string{"valid"}, false},
		{"cross mission", "mission-a", []string{"parcel-a"}, []string{"mission-b"}, true},
		{"cross parcel", "mission-a", []string{"parcel-a"}, []string{"parcel-b"}, true},
		{"inactive", "mission-a", []string{"parcel-a"}, []string{"inactive"}, true},
		{"unknown", "mission-a", []string{"parcel-a"}, []string{"missing"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := authorizer.AuthorizeEvidence(context.Background(), test.mission, test.parcels, test.mediaRefs)
			if (err != nil) != test.wantError {
				t.Fatalf("err=%v wantError=%v", err, test.wantError)
			}
		})
	}
}
