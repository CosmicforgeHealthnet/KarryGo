package adminmedia

import (
	"context"
	"encoding/hex"

	"cosmicforge/logistics/shared/go/adminread"
)

// Legacy mapping lookup (Section 19).
//
// After an owner-attested adoption, a legacy media_assets ID has exactly one
// modern private media object. The Bridge (Caller) may resolve that mapping
// over the signed private transport; the answer carries opaque identifiers
// and the attestation digest only, never bytes, URLs or case facts. A legacy
// ID that was never adopted has no modern identity and answers not-found.
const LegacyMappingContract = "media.legacy-mapping.v1"
const LegacyMappingPath = "/internal/media/v1/legacy-mappings"

type LegacyMappingRequest struct {
	ContractVersion string `json:"contract_version"`
	LegacyAssetID   string `json:"legacy_asset_id"`
}

func (r LegacyMappingRequest) Valid() bool {
	return r.ContractVersion == LegacyMappingContract && validID(r.LegacyAssetID)
}

type LegacyMapping struct {
	ContractVersion   string `json:"contract_version"`
	LegacyAssetID     string `json:"legacy_asset_id"`
	MediaObjectID     string `json:"media_object_id"`
	AttestationDigest string `json:"attestation_digest"`
}

// Valid checks the response shape against the requested legacy ID.
func (m LegacyMapping) Valid(legacyAssetID string) bool {
	digest, err := hex.DecodeString(m.AttestationDigest)
	return m.ContractVersion == LegacyMappingContract && m.LegacyAssetID == legacyAssetID &&
		adminread.Opaque(m.MediaObjectID) && err == nil && len(digest) == 32 &&
		hex.EncodeToString(digest) == m.AttestationDigest
}

// LegacyMapping resolves an adopted legacy asset to its modern object.
func (c *Client) LegacyMapping(ctx context.Context, legacyAssetID string) (LegacyMapping, error) {
	var empty LegacyMapping
	request := LegacyMappingRequest{ContractVersion: LegacyMappingContract, LegacyAssetID: legacyAssetID}
	if !request.Valid() {
		return empty, ErrInvalid
	}
	raw, err := c.call(ctx, LegacyMappingPath, request, MaxBody)
	if err != nil {
		return empty, err
	}
	var out LegacyMapping
	if Decode(raw, &out) != nil || !out.Valid(legacyAssetID) {
		return empty, ErrInvalid
	}
	return out, nil
}
