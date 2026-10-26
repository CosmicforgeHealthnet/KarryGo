package vendors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/mediaclient"
)

// Automated pre-verification (Section 18).
//
// Automation is a reviewer's assistant, not a decider. It records bounded,
// immutable findings per case version and an advisory recommendation. It never
// writes the formal status, never bumps source_version and never produces
// VERIFIED: only a named reviewer through the state machine can.
//
// What is checked, honestly:
//   - DOCUMENT_REFS_PRESENT / DOCUMENT_REF_FORMAT: shape only.
//   - DUPLICATE_DOCUMENT_ACROSS_BUSINESSES: the same document reference cited
//     by a case for a different business. Two independent sources claiming one
//     document is the dual-source mismatch signal; it blocks auto-eligibility.
//   - MEDIA_OBJECT_STATE: when Media is configured, each reference must be an
//     existing, active, unexpired object. Without Media it is NOT_EVALUATED.
//   - EXTERNAL_AUTHORITY: no government registry or KYC provider is contracted,
//     so the only implementation is DisabledAuthority and the outcome is
//     SOURCE_UNVERIFIED. Nothing here pretends otherwise.
//   - ADDRESS_CONSISTENCY: always NOT_EVALUATED. Verification holds no address
//     data for vendor cases (Vendor sends document references only), so an
//     address-conflict check would be invented, not performed.

// Finding outcomes, severities and check codes (mirrored by DB CHECKs).
const (
	OutcomePass             = "PASS"
	OutcomeFail             = "FAIL"
	OutcomeFormatValid      = "FORMAT_VALID"
	OutcomeSourceUnverified = "SOURCE_UNVERIFIED"
	OutcomeNotEvaluated     = "NOT_EVALUATED"

	SeverityInfo     = "INFO"
	SeverityWarn     = "WARN"
	SeverityBlocking = "BLOCKING"

	CheckRefsPresent  = "DOCUMENT_REFS_PRESENT"
	CheckRefFormat    = "DOCUMENT_REF_FORMAT"
	CheckDuplicateDoc = "DUPLICATE_DOCUMENT_ACROSS_BUSINESSES"
	CheckMediaState   = "MEDIA_OBJECT_STATE"
	CheckExternal     = "EXTERNAL_AUTHORITY"
	CheckAddress      = "ADDRESS_CONSISTENCY"

	RecommendationNotEvaluated     = "NOT_EVALUATED"
	RecommendationReviewRequired   = "REVIEW_REQUIRED"
	RecommendationSourceUnverified = "SOURCE_UNVERIFIED"
	RecommendationAutoEligible     = "AUTO_ELIGIBLE"
	RecommendationFormatValid      = "FORMAT_VALID"
)

// Finding is one bounded automation result. It carries codes only.
type Finding struct {
	CheckCode string
	Outcome   string
	Severity  string
}

// MediaInspector is the narrow Media read automation needs.
type MediaInspector interface {
	GetByID(ctx context.Context, id string) (mediaclient.MediaAsset, error)
}

// External authority results.
const (
	ExternalUnavailable   = "UNAVAILABLE"
	ExternalVerified      = "VERIFIED"
	ExternalMismatch      = "MISMATCH"
	ExternalNotApplicable = "NOT_APPLICABLE"
)

// CaseFacts is what an external authority would be asked about. It holds
// identifiers only; Verification stores no identity numbers for vendor cases.
type CaseFacts struct {
	CaseID, BusinessID, Dimension, SubjectType, SubjectRef string
	DocumentRefs                                           []string
}

// ExternalAuthority is a source-of-truth check (registry, KYC provider).
type ExternalAuthority interface {
	Check(ctx context.Context, facts CaseFacts) (string, error)
}

// DisabledAuthority is the only implementation in this build: no external
// verification provider is contracted, so every check is unavailable.
type DisabledAuthority struct{}

func (DisabledAuthority) Check(context.Context, CaseFacts) (string, error) {
	return ExternalUnavailable, nil
}

var documentRefFormat = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)

// Recommend derives the advisory recommendation from findings. Any FAIL needs
// a reviewer; otherwise an unavailable external source keeps the case
// SOURCE_UNVERIFIED; AUTO_ELIGIBLE requires an external VERIFIED (PASS),
// which cannot happen while only DisabledAuthority exists. A recommendation
// is never a status.
func Recommend(findings []Finding) string {
	external := ""
	for _, f := range findings {
		if f.Outcome == OutcomeFail {
			return RecommendationReviewRequired
		}
		if f.CheckCode == CheckExternal {
			external = f.Outcome
		}
	}
	switch external {
	case OutcomeSourceUnverified:
		return RecommendationSourceUnverified
	case OutcomePass:
		return RecommendationAutoEligible
	}
	return RecommendationFormatValid
}

// Automation is the leased, bounded pre-verification worker.
type Automation struct {
	DB       *pgxpool.Pool
	Media    MediaInspector
	External ExternalAuthority
	Owner    string
	Lease    time.Duration
	Now      func() time.Time
}

func (a *Automation) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

type claimedCase struct {
	CaseFacts
	Version int64
}

// RunOnce claims at most limit open case versions not yet evaluated, records
// their findings and returns how many were recorded.
func (a *Automation) RunOnce(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	lease := a.Lease
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	owner := a.Owner
	if owner == "" {
		owner = "verification-automation"
	}
	rows, err := a.DB.Query(ctx, `WITH due AS (
		SELECT id FROM vendor_verification_cases
		WHERE status IN ('PENDING_REVIEW','IN_PROGRESS')
		  AND automation_evaluated_version IS DISTINCT FROM source_version
		  AND (automation_lease_expires_at IS NULL OR automation_lease_expires_at < now())
		ORDER BY updated_at, id LIMIT $1 FOR UPDATE SKIP LOCKED)
		UPDATE vendor_verification_cases c SET automation_lease_owner=$2, automation_lease_expires_at=now()+$3::interval
		FROM due WHERE c.id=due.id
		RETURNING c.id::text,c.business_id,c.dimension,c.subject_type,c.subject_ref,c.document_refs,c.source_version`, limit, owner, fmt.Sprintf("%d seconds", int64(lease.Seconds())))
	if err != nil {
		return 0, err
	}
	var claimed []claimedCase
	for rows.Next() {
		var c claimedCase
		var docs []byte
		if err = rows.Scan(&c.CaseID, &c.BusinessID, &c.Dimension, &c.SubjectType, &c.SubjectRef, &docs, &c.Version); err != nil {
			rows.Close()
			return 0, err
		}
		if err = json.Unmarshal(docs, &c.DocumentRefs); err != nil {
			c.DocumentRefs = nil
		}
		claimed = append(claimed, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	recorded := 0
	for _, c := range claimed {
		findings, err := a.Evaluate(ctx, c.CaseFacts)
		if err != nil {
			a.release(ctx, c.CaseID, owner)
			return recorded, err
		}
		ok, err := a.record(ctx, c, findings, owner)
		if err != nil {
			return recorded, err
		}
		if ok {
			recorded++
		}
	}
	return recorded, nil
}

func (a *Automation) release(ctx context.Context, caseID, owner string) {
	_, _ = a.DB.Exec(context.WithoutCancel(ctx), `UPDATE vendor_verification_cases SET automation_lease_owner=NULL,automation_lease_expires_at=NULL WHERE id=$1::uuid AND automation_lease_owner=$2`, caseID, owner)
}

// Evaluate runs every check for one case. It reads, never writes.
func (a *Automation) Evaluate(ctx context.Context, facts CaseFacts) ([]Finding, error) {
	findings := []Finding{}
	refs := make([]string, 0, len(facts.DocumentRefs))
	for _, ref := range facts.DocumentRefs {
		if ref = strings.TrimSpace(ref); ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		findings = append(findings, Finding{CheckRefsPresent, OutcomeFail, SeverityBlocking})
	} else {
		findings = append(findings, Finding{CheckRefsPresent, OutcomePass, SeverityInfo})
	}
	format := Finding{CheckRefFormat, OutcomeFormatValid, SeverityInfo}
	for _, ref := range refs {
		if !documentRefFormat.MatchString(ref) {
			format = Finding{CheckRefFormat, OutcomeFail, SeverityWarn}
			break
		}
	}
	if len(refs) == 0 {
		format = Finding{CheckRefFormat, OutcomeNotEvaluated, SeverityInfo}
	}
	findings = append(findings, format)

	duplicate := Finding{CheckDuplicateDoc, OutcomePass, SeverityInfo}
	if len(refs) == 0 {
		duplicate = Finding{CheckDuplicateDoc, OutcomeNotEvaluated, SeverityInfo}
	}
	for _, ref := range refs {
		var shared bool
		if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_verification_cases WHERE business_id<>$1 AND document_refs @> jsonb_build_array($2::text))`, facts.BusinessID, ref).Scan(&shared); err != nil {
			return nil, err
		}
		if shared {
			duplicate = Finding{CheckDuplicateDoc, OutcomeFail, SeverityBlocking}
			break
		}
	}
	findings = append(findings, duplicate)
	findings = append(findings, a.mediaFinding(ctx, refs))

	external := Finding{CheckExternal, OutcomeSourceUnverified, SeverityWarn}
	authority := a.External
	if authority == nil {
		authority = DisabledAuthority{}
	}
	result, err := authority.Check(ctx, facts)
	switch {
	case err != nil || result == ExternalUnavailable:
	case result == ExternalVerified:
		external = Finding{CheckExternal, OutcomePass, SeverityInfo}
	case result == ExternalMismatch:
		external = Finding{CheckExternal, OutcomeFail, SeverityBlocking}
	case result == ExternalNotApplicable:
		external = Finding{CheckExternal, OutcomeNotEvaluated, SeverityInfo}
	}
	findings = append(findings, external)
	findings = append(findings, Finding{CheckAddress, OutcomeNotEvaluated, SeverityInfo})
	return findings, nil
}

// mediaFinding checks each reference against Media. A missing, revoked,
// quarantined, deleted or expired object fails; an unreachable Media is not a
// pass and not a fail, so it is NOT_EVALUATED.
func (a *Automation) mediaFinding(ctx context.Context, refs []string) Finding {
	if a.Media == nil || len(refs) == 0 {
		return Finding{CheckMediaState, OutcomeNotEvaluated, SeverityInfo}
	}
	now := a.now()
	unknown := false
	for _, ref := range refs {
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		asset, err := a.Media.GetByID(callCtx, ref)
		cancel()
		if err != nil {
			var mediaErr mediaclient.Error
			if errors.As(err, &mediaErr) && (mediaErr.StatusCode == http.StatusNotFound || mediaErr.StatusCode == http.StatusGone) {
				return Finding{CheckMediaState, OutcomeFail, SeverityBlocking}
			}
			unknown = true
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(asset.Status), "active") || mediaExpired(asset, now) {
			return Finding{CheckMediaState, OutcomeFail, SeverityBlocking}
		}
	}
	if unknown {
		return Finding{CheckMediaState, OutcomeNotEvaluated, SeverityWarn}
	}
	return Finding{CheckMediaState, OutcomePass, SeverityInfo}
}

func mediaExpired(asset mediaclient.MediaAsset, now time.Time) bool {
	raw, ok := asset.Metadata["expires_at"].(string)
	if !ok {
		return false
	}
	at, err := time.Parse(time.RFC3339, raw)
	return err == nil && !at.After(now)
}

// record writes findings only if the case still holds the evaluated version
// and is still open; a decided or changed case is left for its next version.
func (a *Automation) record(ctx context.Context, c claimedCase, findings []Finding, owner string) (bool, error) {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var version int64
	var status string
	var leaseOwner *string
	err = tx.QueryRow(ctx, `SELECT source_version,status,automation_lease_owner FROM vendor_verification_cases WHERE id=$1::uuid FOR UPDATE`, c.CaseID).Scan(&version, &status, &leaseOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if leaseOwner == nil || *leaseOwner != owner {
		return false, tx.Commit(ctx)
	}
	if version != c.Version || (status != StatusPendingReview && status != StatusInProgress) {
		_, err = tx.Exec(ctx, `UPDATE vendor_verification_cases SET automation_lease_owner=NULL,automation_lease_expires_at=NULL WHERE id=$1::uuid`, c.CaseID)
		if err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	evaluated := a.now()
	for _, f := range findings {
		if _, err = tx.Exec(ctx, `INSERT INTO vendor_case_automation_findings(case_id,case_version,check_code,outcome,severity,evaluated_at) VALUES($1::uuid,$2,$3,$4,$5,$6) ON CONFLICT (case_id,case_version,check_code) DO NOTHING`, c.CaseID, c.Version, f.CheckCode, f.Outcome, f.Severity, evaluated); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE vendor_verification_cases SET automation_recommendation=$2,automation_evaluated_version=$3,automation_lease_owner=NULL,automation_lease_expires_at=NULL WHERE id=$1::uuid`, c.CaseID, Recommend(findings), c.Version); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
