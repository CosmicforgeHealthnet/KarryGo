package clients

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Resources uses a fixed, compiled owner profile and operator-pinned origin.
// It cannot forward a request URL, arbitrary header, token or response object.
type Resources struct {
	transport *Taxi
	profile   adminowner.Profile
}

func NewResources(owner, origin string, secret []byte) (*Resources, error) {
	profile, ok := adminowner.ForOwner(owner)
	if !ok {
		return nil, ErrInvalid
	}
	transport, err := NewTaxi(origin, secret)
	if err != nil {
		return nil, err
	}
	return &Resources{transport: transport, profile: profile}, nil
}
func (c *Resources) Close()        { c.transport.Close() }
func (c *Resources) Owner() string { return c.profile.Owner }

func (c *Resources) Read(ctx context.Context, q adminread.Query, kind, ref string) (adminowner.Snapshot, error) {
	var out adminowner.Snapshot
	definition, ok := c.profile.Definitions[kind]
	if !ok || q.Validate(time.Now()) != nil || ref != "" && (!adminread.Opaque(ref) || strings.ContainsAny(ref, "/\\") || ref == "." || ref == ".." || q.Cursor != "") {
		return out, ErrInvalid
	}
	u := *c.transport.origin
	u.Path = c.profile.Path + "/resources/" + kind
	if ref != "" {
		u.Path += "/" + ref
	}
	u.RawQuery = q.Values().Encode()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return out, ErrInvalid
	}
	if serviceauth.SignRequest(r, adminread.Caller, c.transport.secret, nil, time.Now()) != nil {
		return out, ErrUnavailable
	}
	res, err := c.transport.client.Do(r)
	if err != nil {
		return out, ErrUnavailable
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotFound:
		return out, ErrNotFound
	case http.StatusForbidden:
		return out, ErrScopeDenied
	case http.StatusUnauthorized, http.StatusConflict:
		return out, ErrRefused
	case http.StatusOK:
	default:
		return out, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1048577))
	if err != nil || len(raw) > 1048576 || security.Strict(raw, &out) != nil {
		return adminowner.Snapshot{}, ErrInvalid
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	if out.ContractVersion != adminowner.ContractVersion || out.Owner != c.profile.Owner || out.Kind != kind || out.Scope != scope || out.PolicyID != q.PolicyID || out.PolicyVersion != q.PolicyVersion || out.PolicyChecksum != q.PolicyChecksum || out.Coverage != "CONTEXT_SCOPED_PARTIAL" || out.AsOf.Before(time.Now().Add(-time.Minute)) || out.AsOf.After(time.Now().Add(time.Minute)) || out.Items == nil || len(out.Items) > q.Limit || out.NextCursor != nil && (len(*out.NextCursor) == 0 || len(*out.NextCursor) > 4096) {
		return adminowner.Snapshot{}, ErrInvalid
	}
	if ref != "" && (len(out.Items) != 1 || out.Items[0].Reference != ref || out.NextCursor != nil) {
		return adminowner.Snapshot{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range out.Items {
		if adminowner.ValidateResource(item, definition, ref != "") != nil || seen[item.Reference] || item.CreatedAt.After(out.AsOf) || item.UpdatedAt.After(out.AsOf) || ref == "" && (item.CreatedAt.Before(q.Start) || !item.CreatedAt.Before(q.End)) {
			return adminowner.Snapshot{}, ErrInvalid
		}
		seen[item.Reference] = true
		if ref == "" && (len(item.Timeline) > 0 || item.TimelineTruncated) {
			return adminowner.Snapshot{}, ErrInvalid
		}
	}
	return out, nil
}
