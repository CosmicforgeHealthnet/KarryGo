package checkout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"github.com/jackc/pgx/v5"
)

// ReplacementLine names a catalogue candidate resolved by Orders. Prices are
// revalidated by Shop; the proposed price is only an equality precondition.
type ReplacementLine struct {
	CartItemID, ItemRef, VariantRef, ListingRef, SellUnitRef string
	UnitPriceMinor                                           int64
}

// ValidateReplacement creates immutable revalidation evidence. It neither
// reserves stock nor mutates the source cart/session. Orders retains its
// original reservation until its existing substitution swap is approved.
func (s *Service) ValidateReplacement(ctx context.Context, customerID, sourceID string, replacement ReplacementLine) (Session, error) {
	if err := s.requireDependencies(); err != nil {
		return Session{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize source revision allocation. The composition adapter serializes
	// authority calls using a nonblocking lock before entering this transaction.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,124))`, sourceID); err != nil {
		return Session{}, err
	}
	original, err := loadOwnedSession(ctx, tx, customerID, sourceID, true)
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	if original.Status != StatusValidated || original.Domain != cart.DomainShop || original.Expired(now) || replacement.CartItemID == "" {
		return Session{}, ErrSessionNotLive
	}
	var digest string
	if err = tx.QueryRow(ctx, `SELECT commerce_checkout_evidence_digest($1)`, sourceID).Scan(&digest); err != nil {
		return Session{}, err
	}
	material, _ := json.Marshal(struct {
		Source, Digest string
		Line           ReplacementLine
	}{sourceID, digest, replacement})
	sum := sha256.Sum256(material)
	key := hex.EncodeToString(sum[:])
	var existingID string
	err = tx.QueryRow(ctx, `SELECT id FROM commerce_checkout_sessions WHERE customer_id=$1 AND replacement_key=$2`, customerID, key).Scan(&existingID)
	if err == nil {
		existing, e := loadOwnedSession(ctx, tx, customerID, existingID, false)
		if e != nil {
			return Session{}, e
		}
		if existing.Status != StatusValidated || existing.Expired(now) {
			return Session{}, ErrSessionNotLive
		}
		return existing, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return Session{}, err
	}
	current := cart.Cart{ID: original.CartID, CustomerID: customerID, Domain: original.Domain, Version: original.CartVersion}
	found := false
	for _, group := range original.Groups {
		for _, line := range group.Lines {
			item := cart.CartItem{ID: line.CartItemID, Domain: line.Domain, BusinessID: line.BusinessID, BranchID: line.BranchID,
				ItemRef: line.ItemRef, VariantRef: line.VariantRef, ListingRef: line.ListingRef, SellUnitRef: line.SellUnitRef,
				Quantity: line.Quantity, UnitPriceMinor: line.UnitPriceMinor, Currency: line.Currency,
				PriceReference: line.PriceReference, CatalogueRevision: line.CatalogueRevision,
				PricingBasis: line.PricingBasis, RequestedMeasureRef: line.RequestedMeasureRef, ExpectedMeasureQuantity: line.ExpectedMeasureQuantity}
			if line.CartItemID == replacement.CartItemID {
				found = true
				item.ItemRef, item.VariantRef, item.ListingRef = replacement.ItemRef, replacement.VariantRef, replacement.ListingRef
				if replacement.SellUnitRef != "" {
					item.SellUnitRef = replacement.SellUnitRef
				}
			}
			current.Items = append(current.Items, item)
		}
	}
	if !found {
		return Session{}, ErrSessionNotLive
	}
	outcomes, err := s.revalidateLines(ctx, current)
	if err != nil {
		return Session{}, err
	}
	for _, outcome := range outcomes {
		if outcome.line.CartItemID == replacement.CartItemID && outcome.line.UnitPriceMinor != replacement.UnitPriceMinor {
			return Session{}, ErrSessionNotLive
		}
	}
	groups, failure := s.assemble(current, outcomes)
	if failure != "" {
		return Session{}, ErrSessionNotLive
	}
	session := original
	session.ID = newID("chks_")
	session.Groups = groups
	session.Version = 1
	session.CreatedAt, session.UpdatedAt = now, now
	session.ValidatedAt = &now
	session.SettledAt = nil
	session.ReservationCoordinationState = CoordinationNone
	session.QuotedItemsTotalMinor, session.MaxAuthorizedItemsTotalMinor = 0, 0
	for i := range groups {
		g := &groups[i]
		g.RequiresStockReservation = false
		g.ReservationStatus = ReservationNotRequired
		g.ReservationSourceRef, g.InventoryReservationRef = "", ""
		g.ReservationExpiresAt = nil
		session.QuotedItemsTotalMinor, err = SumTotals(session.QuotedItemsTotalMinor, g.QuotedTotalMinor)
		if err != nil {
			return Session{}, err
		}
		session.MaxAuthorizedItemsTotalMinor, err = SumTotals(session.MaxAuthorizedItemsTotalMinor, g.MaxAuthorizedTotalMinor)
		if err != nil {
			return Session{}, err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(validation_revision),0)+1 FROM commerce_checkout_sessions WHERE cart_id=$1`, original.CartID).Scan(&session.ValidationRevision); err != nil {
		return Session{}, err
	}
	// The ordinary live-cart uniqueness constraint excludes only these
	// explicitly source-bound, non-reserving replacement sessions.
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_checkout_sessions
        (id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,currency,
         quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,
         replacement_source_session_id,replacement_key,replacement_source_digest)
        VALUES($1,$2,$3,$4,$5,'VALIDATED',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		session.ID, customerID, session.CartID, session.CartVersion, session.Domain, session.ValidationRevision, session.AddressRef,
		session.AddressSnapshottedAt, session.Currency, session.QuotedItemsTotalMinor, session.MaxAuthorizedItemsTotalMinor,
		session.ReservationTTLSeconds, session.ExpiresAt, now, sourceID, key, digest); err != nil {
		return Session{}, err
	}
	for _, g := range groups {
		if err = insertGroup(ctx, tx, session.ID, customerID, g); err != nil {
			return Session{}, err
		}
		for _, line := range g.Lines {
			if err = insertLine(ctx, tx, session.ID, customerID, line); err != nil {
				return Session{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return session, nil
}
