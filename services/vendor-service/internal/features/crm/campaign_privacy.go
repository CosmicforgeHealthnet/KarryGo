package crm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jackc/pgx/v5"
)

// shortDigest derives a compact, stable suffix for a deterministic identifier.
func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

// campaignPrivacyHandler owns campaign targeting.
//
// Section 8E registers through the same PrivacyHandler port Sections 8B, 8C and
// 8D use, so a redaction covers campaigns without a single edit to
// privacy.go's ApplyPrivacy. What it does, though, is different in kind from
// the other domains: it does not delete the historical evidence, it revokes the
// customer's current actionability and leaves the frozen snapshot intact.
//
// That split is the whole point. A snapshot is a record of what the consent
// authority said at one moment, and rewriting it would destroy the only
// evidence of why a campaign targeted whom it targeted. Erasing a customer's
// contactability and erasing the audit trail of a past decision are different
// obligations, and conflating them would satisfy neither.
type campaignPrivacyHandler struct{ service *Service }

func (campaignPrivacyHandler) Domain() string { return "campaigns" }

// Restrict stops the relationship being contacted from any prepared audience.
// Restriction is not erasure, so the snapshot rows stay exactly as they were.
func (h campaignPrivacyHandler) Restrict(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return suppressCampaignTargets(ctx, tx, businessID, profileID, SuppressionRestricted)
}

// Redact makes every prepared target for the relationship non-actionable.
//
// The target row survives with its cohort and its consent evidence, because a
// campaign that has already been analysed must remain explicable. What does not
// survive is any possibility of contacting the customer from it: the row is
// flagged, and every read path that answers "may we act on this" consults that
// flag.
func (h campaignPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return suppressCampaignTargets(ctx, tx, businessID, profileID, SuppressionRedacted)
}

// MarketingSuppress is the same revocation by a narrower instruction.
func (h campaignPrivacyHandler) MarketingSuppress(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return suppressCampaignTargets(ctx, tx, businessID, profileID, SuppressionMarketingSuppressed)
}

// IdentityMerge stops the merged-away relationship being targeted independently.
//
// The source's targets are not moved onto the survivor. Moving them would
// assert that the survivor was selected by a definition that never evaluated
// them, and if both were already in the snapshot it would collide with the one
// row per relationship rule. Revoking the source instead is both honest and
// sufficient: the survivor is either already in the snapshot and still
// targetable exactly once, or it is not, and the next preparation will decide
// about it on the definition's own terms.
func (h campaignPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	if err := suppressCampaignTargets(ctx, tx, businessID, sourceProfileID, SuppressionIdentityMerged); err != nil {
		return err
	}
	// If the source was handed off immediately before this merge acquired both
	// profile locks, suppress the survivor's still-pending equivalent. This is
	// the durable campaign-run + canonical-survivor + channel fence: either the
	// merge wins and the source cannot hand off, or the source wins and this
	// transaction prevents a second contact through the survivor.
	rows, err := tx.Query(ctx, `SELECT d.id,d.status,d.run_id FROM crm_campaign_deliveries d
		WHERE d.business_id=$1 AND d.customer_profile_id=$2
		  AND d.status IN ('PENDING','DEFERRED_QUIET_HOURS','FAILED_TRANSIENT')
		  AND EXISTS (SELECT 1 FROM crm_campaign_contact_history hst
		    WHERE hst.business_id=d.business_id AND hst.run_id=d.run_id
		      AND hst.customer_profile_id=$3 AND hst.channel=d.channel)
		FOR UPDATE`, businessID, targetProfileID, sourceProfileID)
	if err != nil {
		return err
	}
	type pending struct{ id, status, runID string }
	var matches []pending
	for rows.Next() {
		var item pending
		if err = rows.Scan(&item.id, &item.status, &item.runID); err != nil {
			rows.Close()
			return err
		}
		matches = append(matches, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	now := h.service.now()
	runs := map[string]bool{}
	for _, item := range matches {
		if _, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries SET status='SUPPRESSED',suppression_reason='MERGE_DUPLICATE',updated_at=$2,completed_at=$2,version=version+1 WHERE id=$1`, item.id, now); err != nil {
			return err
		}
		from := item.status
		if err = h.service.appendDeliveryHistory(ctx, tx, businessID, item.id, &from, DeliverySuppressed, "MERGE_DUPLICATE", "VENDOR", "", 0, now); err != nil {
			return err
		}
		runs[item.runID] = true
	}
	for runID := range runs {
		if err = h.service.refreshRun(ctx, tx, runID, now); err != nil {
			return err
		}
	}
	return nil
}

// suppressCampaignTargets revokes current actionability without rewriting
// history. It is idempotent: a relationship already suppressed keeps its first
// reason, so a later, broader instruction cannot overwrite the record of why it
// was first stopped.
func suppressCampaignTargets(ctx context.Context, tx pgx.Tx, businessID, profileID, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_campaign_targets
		SET current_actionable=false, suppression_reason_code=$1, suppressed_at=now()
		WHERE business_id=$2 AND customer_profile_id=$3 AND current_actionable`,
		reason, businessID, profileID)
	return err
}

// ---------------------------------------------------------------------------
// Segment generation retention
// ---------------------------------------------------------------------------

// campaignGenerationRetainer answers Section 8B's cleanup question: may this
// superseded membership generation be released?
//
// It is the port Section 8B deliberately left open. Without it, a segment that
// rematerialized while a campaign was preparing from it would have its old
// membership deleted mid-run, and the campaign would freeze an audience that
// silently lost everyone the cleanup reached first.
type campaignGenerationRetainer struct{ service *Service }

// Retained reports whether a live snapshot still depends on this generation.
//
// BUILDING counts as well as CURRENT: a preparation in flight is exactly the
// case that needs protecting, and a snapshot that has not finished yet is the
// one with most to lose.
func (r campaignGenerationRetainer) Retained(ctx context.Context, businessID, segmentID string, generation int64) (bool, error) {
	var retained bool
	err := r.service.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND source_segment_id=$2 AND source_segment_generation=$3
		  AND status IN ('BUILDING','CURRENT'))`, businessID, segmentID, generation).Scan(&retained)
	return retained, err
}
