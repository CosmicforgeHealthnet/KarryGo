package crm

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Section 8B registers two privacy domains with the Section 8A registry rather
// than bolting table names onto the redaction engine.
//
// That is what makes the guarantee in PrivacySummary.CoveredDomains true as the
// CRM grows: adding classification and segmentation extended what a redaction
// covers without a single edit to privacy.go's ApplyPrivacy, and 8C onwards can
// do the same.

// tagPrivacyHandler owns vendor tag membership.
type tagPrivacyHandler struct{ service *Service }

func (tagPrivacyHandler) Domain() string { return "tags" }

// Restrict is a no-op, matching Section 8A's policy that a restriction limits
// processing without erasing anything. Restricted relationships are already
// ineligible for marketing through MarketingEligibility, so their tags stay
// available for lawful operational CRM.
func (tagPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }

// Redact removes every tag membership for the relationship.
//
// A tag is a targeting label by construction, so leaving one attached to a
// redacted subject would leave them selectable by any segment that reads tags —
// the exact outcome redaction exists to prevent. Removal happens inside the
// privacy transaction, so it takes effect the moment the redaction commits and
// never waits for a scheduled rebuild.
func (tagPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM crm_customer_tag_memberships WHERE business_id=$1 AND profile_id=$2`,
		businessID, profileID)
	return err
}

// MarketingSuppress leaves tags in place: suppression removes targetability,
// which is decided by the eligibility rule, not by the presence of a label.
func (tagPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error { return nil }

// IdentityMerge moves the source's tags onto the survivor, collapsing
// duplicates.
//
// Tags are a vendor's own classification of a relationship, so unlike consent
// they do travel: the vendor decided this human is a VIP, and the merge does
// not change that. Anything the survivor already carries is dropped rather than
// duplicated.
func (tagPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	if _, err := tx.Exec(ctx, `UPDATE crm_customer_tag_memberships m SET profile_id=$1
		WHERE m.business_id=$2 AND m.profile_id=$3
		  AND NOT EXISTS (SELECT 1 FROM crm_customer_tag_memberships t
			WHERE t.business_id=m.business_id AND t.tag_id=m.tag_id AND t.profile_id=$1)`,
		targetProfileID, businessID, sourceProfileID); err != nil {
		return err
	}
	// Anything left is a tag the survivor already had.
	_, err := tx.Exec(ctx, `DELETE FROM crm_customer_tag_memberships WHERE business_id=$1 AND profile_id=$2`,
		businessID, sourceProfileID)
	return err
}

// segmentPrivacyHandler owns segment membership and manual exclusions.
type segmentPrivacyHandler struct{ service *Service }

func (segmentPrivacyHandler) Domain() string { return "segments" }

func (h segmentPrivacyHandler) Restrict(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return h.removeFromEligibilitySegments(ctx, tx, businessID, profileID)
}

// Redact removes the relationship from every audience it is in.
//
// Static membership and materialized dynamic membership both go, and the
// dynamic segments that held them are marked stale so their counts are rebuilt
// honestly. Manual exclusions go too: an exclusion is a record about a customer
// who no longer has a relationship, and retaining one would keep a
// customer-specific decision alive after erasure with nothing left to exclude
// it from.
func (h segmentPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	// Invalidation runs first: it finds the affected segments through the very
	// membership rows the next statement deletes.
	if err := h.service.invalidateSegmentsForProfile(ctx, tx, businessID, profileID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `DELETE FROM crm_segment_memberships
		WHERE business_id=$1 AND profile_id=$2 RETURNING segment_id`, businessID, profileID)
	if err != nil {
		return err
	}
	affected, err := segmentIDs(rows)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM crm_segment_exclusions WHERE business_id=$1 AND profile_id=$2`,
		businessID, profileID); err != nil {
		return err
	}
	// Counts are part of the same privacy transaction as removal. This covers
	// static membership and whichever dynamic generation is externally current.
	return h.service.refreshCurrentSegmentCountsTx(ctx, tx, businessID, affected)
}

func (h segmentPrivacyHandler) MarketingSuppress(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return h.removeFromEligibilitySegments(ctx, tx, businessID, profileID)
}

// removeFromEligibilitySegments makes a restriction/suppression effective for
// already-materialized audiences before the authoritative privacy transaction
// can commit. Consent decisions are untouched; only dynamic segments whose
// dependency set reads an effective eligibility field are invalidated, queued
// and stripped of the now-ineligible profile.
func (h segmentPrivacyHandler) removeFromEligibilitySegments(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	dependencies := make([]string, 0, len(MarketingChannels))
	for _, channel := range MarketingChannels {
		dependencies = append(dependencies, "consent.marketing."+strings.ToLower(string(channel))+".eligible")
	}
	segmentIDs, err := h.service.invalidateSegmentsForDependenciesReturningIDs(ctx, tx, businessID, dependencies)
	if err != nil || len(segmentIDs) == 0 {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM crm_segment_memberships
		WHERE business_id=$1 AND profile_id=$2 AND segment_id=ANY($3)`,
		businessID, profileID, segmentIDs); err != nil {
		return err
	}
	return h.service.refreshCurrentSegmentCountsTx(ctx, tx, businessID, segmentIDs)
}

// IdentityMerge moves the survivor's explicit relationships and rebuilds the
// derived ones.
//
// Static membership and manual exclusions are deliberate operator decisions and
// transfer to the survivor. Dynamic membership is not copied at all: it is
// derived, so the honest move is to drop the source's rows and mark the
// affected segments stale, letting the rule decide about the merged
// relationship on its own terms. Copying it would assert a match the rule was
// never asked about.
func (h segmentPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	// A merge can make the survivor newly qualify for a rule it was not already
	// in, so looking only at existing membership is insufficient. Restake every
	// dynamic segment in this tenant; derived membership is never copied.
	if err := h.service.invalidateAllDynamicSegmentsTx(ctx, tx, businessID); err != nil {
		return err
	}
	// Exclusions transfer first. A vendor who excluded the source from an
	// audience must not find the survivor quietly back in it — a merge is not a
	// reason to revisit that decision.
	if _, err := tx.Exec(ctx, `UPDATE crm_segment_exclusions x SET profile_id=$1
		WHERE x.business_id=$2 AND x.profile_id=$3
		  AND NOT EXISTS (SELECT 1 FROM crm_segment_exclusions t
			WHERE t.business_id=x.business_id AND t.segment_id=x.segment_id AND t.profile_id=$1)`,
		targetProfileID, businessID, sourceProfileID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM crm_segment_exclusions WHERE business_id=$1 AND profile_id=$2`,
		businessID, sourceProfileID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE crm_segment_memberships m SET profile_id=$1
		WHERE m.business_id=$2 AND m.profile_id=$3 AND m.membership_source='STATIC'
		  AND NOT EXISTS (SELECT 1 FROM crm_segment_memberships t
			WHERE t.business_id=m.business_id AND t.segment_id=m.segment_id
			  AND t.profile_id=$1 AND t.generation=m.generation)`,
		targetProfileID, businessID, sourceProfileID); err != nil {
		return err
	}
	// Everything still pointing at the source is either a duplicate static row
	// or derived dynamic membership, and neither should survive the merge.
	rows, err := tx.Query(ctx, `DELETE FROM crm_segment_memberships
		WHERE business_id=$1 AND profile_id=$2 RETURNING segment_id`, businessID, sourceProfileID)
	if err != nil {
		return err
	}
	affected, err := segmentIDs(rows)
	if err != nil {
		return err
	}
	return h.service.refreshCurrentSegmentCountsTx(ctx, tx, businessID, affected)
}

// refreshCurrentSegmentCountsTx keeps the public count aligned with the rows
// visible through the current generation pointer. Privacy and merge removals
// call it before committing, so no reader can observe a safe member list paired
// with a stale count.
func (s *Service) refreshCurrentSegmentCountsTx(ctx context.Context, tx pgx.Tx, businessID string, segmentIDs []string) error {
	if len(segmentIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE crm_segments g SET current_member_count=(
			SELECT count(*) FROM crm_segment_memberships m
			WHERE m.business_id=g.business_id AND m.segment_id=g.id
			  AND m.generation=CASE WHEN g.segment_type='STATIC' THEN 0 ELSE COALESCE(
				(SELECT e.generation FROM crm_segment_evaluations e
				 WHERE e.business_id=g.business_id AND e.id=g.current_evaluation_id), -1) END),
			updated_at=now()
		WHERE g.business_id=$1 AND g.id=ANY($2)`, businessID, segmentIDs)
	return err
}
