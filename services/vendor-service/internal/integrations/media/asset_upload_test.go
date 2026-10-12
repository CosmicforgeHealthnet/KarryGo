package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAssetGrantAndScopedValidation(t *testing.T) {
	object := Object{ID: "media-a", OwnerReference: "business-a", OwnerService: "vendor-service", UploadPurpose: "PRODUCT_IMAGE", UploadStatus: "COMPLETED", SafetyStatus: "SAFE", DeletionState: "ACTIVE", Visibility: "PRIVATE", BindingType: "ASSET", BindingID: "FOOD_ITEM:item-a", SourceVersion: 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Organization-ID") != "business-a" || r.Header.Get("X-Service-Nonce") == "" {
			t.Error("scoped authenticated headers missing")
		}
		if r.URL.Path == privateBase+"/upload-authorizations" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["uploadPurpose"] != "PRODUCT_IMAGE" || body["visibility"] != "PRIVATE" || body["bindingType"] != "ASSET" || body["bindingId"] != "FOOD_ITEM:item-a" {
				t.Error("unbound upload request")
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"uploadAuthorizationId": "grant-a", "mediaObjectId": "media-a", "uploadToken": "token", "expiresAt": time.Now().Add(time.Minute), "maxSizeBytes": 100, "allowedMimeTypes": []string{"image/jpeg"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": object})
	}))
	defer server.Close()
	client := NewClient(server.URL, "secret", time.Second).WithPublicURL("https://media.example")
	grant, err := client.AuthorizeAssetUpload(context.Background(), "business-a", "FOOD_ITEM", "item-a", "PRODUCT_IMAGE", "meal.jpg", "image/jpeg", 100, "key")
	if err != nil || grant.MediaRef != "media-a" || grant.URL != "https://media.example/api/v1/media-files/vendor-uploads/media-a/content" {
		t.Fatalf("invalid grant: %+v %v", grant, err)
	}
	if err = client.ValidateAsset(context.Background(), "business-a", "FOOD_ITEM", "item-a", "media-a"); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Object){"foreign target": func(o *Object) { o.BindingID = "FOOD_ITEM:item-b" }, "foreign business": func(o *Object) { o.OwnerReference = "business-b" }, "unsafe": func(o *Object) { o.SafetyStatus = "UNSAFE" }, "pending": func(o *Object) { o.SafetyStatus = "PENDING" }, "wrong purpose": func(o *Object) { o.UploadPurpose = "COMPLIANCE_DOCUMENT" }, "public": func(o *Object) { o.Visibility = "PUBLIC" }} {
		t.Run(name, func(t *testing.T) {
			original := object
			change(&object)
			if client.ValidateAsset(context.Background(), "business-a", "FOOD_ITEM", "item-a", "media-a") == nil {
				t.Fatal("invalid attachment accepted")
			}
			object = original
		})
	}
}
