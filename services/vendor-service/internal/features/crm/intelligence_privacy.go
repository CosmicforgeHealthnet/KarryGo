package crm

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Section 8H — intelligence under privacy and identity merge.
//
// Registered through the Section 8A PrivacyHandler port that Sections 8B, 8C
// and 8G already use, so ApplyPrivacy is untouched. The fourth section to
// extend privacy through the port rather than by editing it.
//
// The governing rule is that a score is derived, so it is never the thing that
// survives a privacy action. Where a tag records a decision the vendor made
// about a person, a score records what this service inferred from facts it may
// no longer use — which is why redaction suppresses it outright and a merge
// recomputes rather than copies.
type intelligencePrivacyHandler struct{ service *Service }

func (h intelligencePrivacyHandler) Domain() string { return "intelligence" }

// Restrict does not change intelligence.
//
// Section 8A's restriction semantics permit lawful non-marketing operational
// CRM reads, and a score is exactly that: an operator asking "which of my
// relationships have gone quiet" is not marketing to anybody. Targetability is
// prevented where it is actually decided — Sections 8E and 8F check privacy
// and consent at send time and are not consulted, weakened or bypassed here.
// Inventing a Section 8H restriction policy would put a second, divergent
// answer next to the accepted one.
func (h intelligencePrivacyHandler) Restrict(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return nil
}

// Redact makes every score for the relationship non-actionable, immediately
// and in the same transaction as the redaction.
//
// The rows are demoted rather than deleted: is_current is cleared, the numbers
// are nulled and the reason becomes PRIVACY_SUPPRESSED. That takes the
// relationship out of every path at once — the segment subqueries require
// is_current AND state='CURRENT', Customer 360 refuses a redacted profile, the
// distributions count only current rows, and the export datasets exclude
// redacted profiles — while leaving an auditable trace that a score once
// existed. Deleting outright would be indistinguishable from never having
// scored the person, which is a worse answer for an auditor and no better for
// the customer, because the personal data was never in these tables to begin
// with.
func (h intelligencePrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return h.service.SuppressIntelligenceForProfileTx(ctx, tx, businessID, profileID, ReasonPrivacySuppressed)
}

// MarketingSuppress does not destroy intelligence.
//
// Suppression is about who may be contacted, and it is enforced by Sections 8E
// and 8F at the point of contact. Deleting the score would blind the operator
// running the business without adding any protection the campaign rules do not
// already provide — and would quietly remove the relationship from analytics
// that legitimately describe the book.
func (h intelligencePrivacyHandler) MarketingSuppress(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	return nil
}

// IdentityMerge makes the source non-actionable and forces the target to be
// recomputed.
//
// It deliberately does not copy the source's score onto the target. A score is
// a function of a relationship's whole observed history, and after a merge the
// target's history is the union of both — so the correct score for the merged
// relationship is generally neither of the two that existed before it. Copying
// would assert a number no model ever computed; recomputing produces one that
// a reader can trace back to the model and the facts.
//
// Until that rebuild runs, the target's scope is marked STALE rather than
// blanked, so the operator keeps seeing the last good figures with an honest
// as-of instant.
func (h intelligencePrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx,
	businessID, sourceProfileID, targetProfileID string) error {
	if err := h.service.SuppressIntelligenceForProfileTx(ctx, tx, businessID,
		sourceProfileID, ReasonMergedSource); err != nil {
		return err
	}
	// The target's own current scores are now computed from a history that no
	// longer describes it. They are demoted for the same reason: a stale number
	// presented as current is worse than an honest gap.
	if err := h.service.SuppressIntelligenceForProfileTx(ctx, tx, businessID,
		targetProfileID, ReasonSourceStale); err != nil {
		return err
	}
	return h.service.MarkIntelligenceStaleTx(ctx, tx, businessID)
}
