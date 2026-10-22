package adminmedia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Owner-attested legacy asset adoption.
//
// A legacy media_assets row has no case, subject or organization binding.
// Only the source owner that uploaded it can attest what it is evidence for;
// Media then records an immutable one-to-one mapping to a modern, bound,
// PRIVATE media object built from the same verified bytes. Supplying a legacy
// asset ID anywhere else never creates such an association.
const LegacyAdoptionContract = "media.legacy-adoption.v1"
const LegacyAdoptionPath = "/internal/media/v1/legacy-adoptions"

var ErrConflict = errors.New("legacy asset already adopted under a different attestation")

type LegacyAdoption struct {
	ContractVersion      string `json:"contract_version"`
	LegacyAssetID        string `json:"legacy_asset_id"`
	LegacyOwnerReference string `json:"legacy_owner_reference"`
	OrganizationID       string `json:"organization_id"`
	CaseKind             string `json:"case_kind"`
	CaseReference        string `json:"case_reference"`
	SubjectType          string `json:"subject_type"`
	SubjectReference     string `json:"subject_reference"`
	Purpose              string `json:"purpose"`
}

// Valid checks shape and that the attested case/subject/purpose exist in the
// owner's compiled case registry. It does not prove ownership; Media does.
func (a LegacyAdoption) Valid(owner string) bool {
	if a.ContractVersion != LegacyAdoptionContract || !validID(a.LegacyAssetID) {
		return false
	}
	for _, v := range []string{a.LegacyOwnerReference, a.OrganizationID, a.CaseKind, a.CaseReference, a.SubjectType, a.SubjectReference, a.Purpose} {
		if !adminread.Opaque(v) {
			return false
		}
	}
	spec, ok := admincase.Find(owner, a.CaseKind)
	return ok && len(spec.Subjects[a.SubjectType]) > 0 && admincase.Contains(spec.Purposes, a.Purpose)
}

// Digest binds the attesting owner and every attested fact. A second
// attestation for the same legacy asset must produce the same digest.
func (a LegacyAdoption) Digest(owner string) string {
	raw, _ := json.Marshal(struct {
		Owner string
		A     LegacyAdoption
	}{owner, a})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type LegacyAdoptionReceipt struct {
	ContractVersion   string    `json:"contract_version"`
	LegacyAssetID     string    `json:"legacy_asset_id"`
	MediaObjectID     string    `json:"media_object_id"`
	ChecksumSHA256    string    `json:"checksum_sha256"`
	AttestationDigest string    `json:"attestation_digest"`
	Replayed          bool      `json:"replayed"`
	RecordedAt        time.Time `json:"recorded_at"`
}

// LegacyAdoptionClient is the source owner's fixed-path caller. It signs as
// the owner with a dedicated adoption key and never retries automatically;
// the operation is idempotent by legacy asset ID, so a caller may resend the
// identical attestation after an ambiguous outcome.
type LegacyAdoptionClient struct {
	owner  string
	origin *url.URL
	secret []byte
	http   *http.Client
}

func NewLegacyAdoptionClient(owner, origin string, secret []byte) (*LegacyAdoptionClient, error) {
	known := false
	for _, s := range admincase.Specs() {
		known = known || s.Owner == owner
	}
	u, err := url.Parse(origin)
	if !known || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || len(secret) < 32 {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second, MaxConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &LegacyAdoptionClient{owner: owner, origin: u, secret: append([]byte{}, secret...), http: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *LegacyAdoptionClient) Close() { c.http.CloseIdleConnections() }

func (c *LegacyAdoptionClient) Adopt(ctx context.Context, a LegacyAdoption) (LegacyAdoptionReceipt, error) {
	var empty LegacyAdoptionReceipt
	if !a.Valid(c.owner) {
		return empty, ErrInvalid
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return empty, ErrInvalid
	}
	u := *c.origin
	u.Path, u.RawPath, u.RawQuery = LegacyAdoptionPath, "", ""
	request, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(raw))
	if err != nil {
		return empty, ErrInvalid
	}
	request.Header.Set("Content-Type", "application/json")
	if serviceauth.SignRequest(request, c.owner, c.secret, raw, time.Now()) != nil {
		return empty, ErrUnavailable
	}
	response, err := c.http.Do(request)
	if err != nil {
		return empty, ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
	case 401, 403:
		return empty, ErrDenied
	case 404:
		return empty, ErrNotFound
	case 409:
		return empty, ErrConflict
	default:
		return empty, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxBody+1))
	if err != nil || len(body) > MaxBody {
		return empty, ErrInvalid
	}
	var out LegacyAdoptionReceipt
	if Decode(body, &out) != nil || out.ContractVersion != LegacyAdoptionContract || out.LegacyAssetID != a.LegacyAssetID || out.AttestationDigest != a.Digest(c.owner) || !adminread.Opaque(out.MediaObjectID) || len(out.ChecksumSHA256) != 64 || out.RecordedAt.IsZero() {
		return empty, ErrInvalid
	}
	return out, nil
}
