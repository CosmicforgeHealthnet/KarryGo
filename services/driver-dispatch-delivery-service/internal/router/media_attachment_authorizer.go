package router

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"cosmicforge/logistics/shared/go/mediaclient"
)

const dispatchMediaOwnerService = "dispatch-delivery-service"

type mediaAttachmentAuthorizer struct{ client *mediaclient.Client }

func newMediaAttachmentAuthorizer(client *mediaclient.Client) *mediaAttachmentAuthorizer {
	if client == nil {
		return nil
	}
	return &mediaAttachmentAuthorizer{client: client}
}

func (a *mediaAttachmentAuthorizer) AuthorizeEvidence(ctx context.Context, missionRef string, parcelRefs, mediaRefs []string) error {
	if a == nil || a.client == nil {
		return errors.New("media attachment authority is unavailable")
	}
	missionRef = strings.TrimSpace(missionRef)
	if missionRef == "" || len(mediaRefs) == 0 {
		return errors.New("mission and media references are required")
	}
	wantParcels := canonicalStrings(parcelRefs)
	for _, mediaRef := range canonicalStrings(mediaRefs) {
		asset, err := a.client.GetByID(ctx, mediaRef)
		if err != nil {
			return fmt.Errorf("read media %s: %w", mediaRef, err)
		}
		if asset.OwnerService != dispatchMediaOwnerService || strings.TrimSpace(asset.OwnerID) != missionRef || !strings.EqualFold(asset.Status, "active") {
			return fmt.Errorf("media %s is not owned by mission %s", mediaRef, missionRef)
		}
		if asset.Purpose != "proof_image" && asset.Purpose != "signature" {
			return fmt.Errorf("media %s has unsupported evidence purpose", mediaRef)
		}
		if metadataMission := metadataString(asset.Metadata, "mission_ref"); metadataMission != "" && metadataMission != missionRef {
			return fmt.Errorf("media %s mission metadata mismatch", mediaRef)
		}
		boundParcels := metadataStrings(asset.Metadata, "parcel_refs")
		if one := metadataString(asset.Metadata, "parcel_ref"); one != "" {
			boundParcels = append(boundParcels, one)
		}
		boundParcels = canonicalStrings(boundParcels)
		if len(boundParcels) > 0 && !equalStrings(boundParcels, wantParcels) {
			return fmt.Errorf("media %s parcel metadata mismatch", mediaRef)
		}
	}
	return nil
}

func canonicalStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func metadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func metadataStrings(metadata map[string]interface{}, key string) []string {
	value, ok := metadata[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func equalStrings(left, right []string) bool {
	left, right = canonicalStrings(left), canonicalStrings(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
