package adminfeed

import (
	"context"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strconv"
	"strings"
	"time"
)

// SourceReader binds an administrative allocation to the immutable, actual
// campaign revision. Content, recipients and approver identities never leave it.
type SourceReader struct{}

func (SourceReader) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (admincontext.RecordBinding, error) {
	var empty admincontext.RecordBinding
	id, rev, ok := strings.Cut(ref, ":")
	version, err := strconv.ParseInt(rev, 10, 64)
	if kind != "campaign-revisions" || !ok || err != nil || version < 1 || strconv.FormatInt(version, 10) != rev {
		return empty, admincontext.ErrUnmapped
	}
	var raw []byte
	var digest string
	var created time.Time
	if err := tx.QueryRow(ctx, `SELECT intent,intent_digest,created_at FROM campaigns WHERE campaign_id::text=$1 AND revision=$2`, id, version).Scan(&raw, &digest, &created); err != nil {
		return empty, err
	}
	var intent admincampaign.Intent
	if json.Unmarshal(raw, &intent) != nil || intent.Digest() != digest || len(intent.Markets) != 1 || len(intent.Zones) > 1 {
		return empty, admincontext.ErrUnmapped
	}
	canonical := adminpolicy.Scope{Product: "karryam", Market: intent.Markets[0]}
	if len(intent.Zones) == 1 {
		canonical.Zone = intent.Zones[0]
	}
	return admincontext.RecordBinding{SourceContract: admincampaign.Contract, Canonical: canonical, Fingerprint: admincontext.Fingerprint(struct {
		Owner, Reference, Digest string
		Created                  time.Time
	}{Source, ref, digest, created})}, nil
}

// No new staff resource API is introduced: reads keep using the existing
// campaign readback contract. This adapter supports owner context publication.
func (SourceReader) Read(context.Context, pgx.Tx, string, string, bool) (adminowner.Resource, error) {
	return adminowner.Resource{}, errors.New("use existing campaign readback")
}
