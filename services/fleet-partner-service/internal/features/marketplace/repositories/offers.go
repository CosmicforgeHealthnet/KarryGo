package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// OfferRepository stores organization offers, provider recipients and the
// immutable organization acceptance.
type OfferRepository struct {
	db database.Querier
}

// NewOfferRepository wires the repository.
func NewOfferRepository(db database.Querier) *OfferRepository {
	return &OfferRepository{db: db}
}

const offerColumns = `id, organization_id, request_projection_id, operating_mode, status,
	offered_at, expires_at, reserved_at, accepted_at, declined_at, cancelled_at,
	resolved_actor_type, resolved_actor_id, source_reservation_id, reason_code, reason_text,
	version, created_at, updated_at`

// Create inserts an organization offer. The partial unique index makes a
// second live offer for the same request impossible.
func (r *OfferRepository) Create(ctx context.Context, q database.Querier, organizationID string, offer *models.Offer) error {
	if offer.ID == "" {
		offer.ID = ids.New("ofr")
	}
	offer.OrganizationID = organizationID
	offer.Version = 1
	if offer.Status == "" {
		offer.Status = models.OfferOffered
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_request_offers
			(id, organization_id, request_projection_id, operating_mode, status,
			 offered_at, expires_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 1, now(), now())`,
		offer.ID, organizationID, offer.RequestProjectionID, offer.OperatingMode,
		offer.Status, offer.OfferedAt, offer.ExpiresAt)
	if err != nil {
		return fmt.Errorf("marketplace: create offer: %w", err)
	}
	return nil
}

// GetByID loads an offer inside the organization scope.
func (r *OfferRepository) GetByID(ctx context.Context, organizationID string, offerID string) (*models.Offer, error) {
	row := r.db.QueryRow(ctx, `SELECT `+offerColumns+`
		FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1 AND id = $2`, organizationID, offerID)
	return scanOffer(row)
}

// LockByID loads an offer FOR UPDATE inside the caller's transaction, so
// concurrent acceptance attempts serialize on this row.
func (r *OfferRepository) LockByID(ctx context.Context, q database.Querier, organizationID string, offerID string) (*models.Offer, error) {
	row := q.QueryRow(ctx, `SELECT `+offerColumns+`
		FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1 AND id = $2 FOR UPDATE`, organizationID, offerID)
	return scanOffer(row)
}

// GetLiveByProjection loads the current offer for a request projection.
func (r *OfferRepository) GetLiveByProjection(ctx context.Context, q database.Querier, organizationID string, projectionID string) (*models.Offer, error) {
	if q == nil {
		q = r.db
	}
	row := q.QueryRow(ctx, `SELECT `+offerColumns+`
		FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1 AND request_projection_id = $2
		  AND status IN ('OFFERED', 'RESERVED', 'ACCEPTED')`, organizationID, projectionID)
	return scanOffer(row)
}

// SetStatus performs a compare-and-swap offer status change.
func (r *OfferRepository) SetStatus(ctx context.Context, q database.Querier, organizationID string, offerID string, status string, expectedVersion int64, actorType string, actorID string, reasonCode string, reasonText string) error {
	var actorTypePtr, actorIDPtr, reasonCodePtr, reasonTextPtr *string
	if actorType != "" {
		actorTypePtr = &actorType
	}
	if actorID != "" {
		actorIDPtr = &actorID
	}
	if reasonCode != "" {
		reasonCodePtr = &reasonCode
	}
	if reasonText != "" {
		reasonTextPtr = &reasonText
	}

	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_request_offers SET
			status = $3,
			reserved_at = CASE WHEN $3 = 'RESERVED' THEN now() ELSE reserved_at END,
			accepted_at = CASE WHEN $3 = 'ACCEPTED' THEN now() ELSE accepted_at END,
			declined_at = CASE WHEN $3 = 'DECLINED' THEN now() ELSE declined_at END,
			cancelled_at = CASE WHEN $3 IN ('CANCELLED', 'EXPIRED') THEN now() ELSE cancelled_at END,
			resolved_actor_type = COALESCE($5, resolved_actor_type),
			resolved_actor_id = COALESCE($6, resolved_actor_id),
			reason_code = COALESCE($7, reason_code),
			reason_text = COALESCE($8, reason_text),
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $4`,
		organizationID, offerID, status, expectedVersion,
		actorTypePtr, actorIDPtr, reasonCodePtr, reasonTextPtr)
	if err != nil {
		return fmt.Errorf("marketplace: set offer status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// SetSourceReservation records the confirmed source reservation reference.
func (r *OfferRepository) SetSourceReservation(ctx context.Context, q database.Querier, offerID string, reservationID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_request_offers SET
			source_reservation_id = $2, updated_at = now()
		WHERE id = $1`, offerID, reservationID)
	if err != nil {
		return fmt.Errorf("marketplace: set source reservation: %w", err)
	}
	return nil
}

// ExpireDue expires overdue open offers and their pending recipients.
func (r *OfferRepository) ExpireDue(ctx context.Context, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		WITH due AS (
			SELECT id FROM fleet_partner_organization_request_offers
			WHERE status = 'OFFERED' AND expires_at < now()
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		), expired AS (
			UPDATE fleet_partner_organization_request_offers o SET
				status = 'EXPIRED', cancelled_at = now(), version = o.version + 1, updated_at = now()
			FROM due WHERE o.id = due.id
			RETURNING o.id
		)
		UPDATE fleet_partner_provider_offer_recipients r SET
			status = 'EXPIRED', version = r.version + 1, updated_at = now()
		FROM expired WHERE r.offer_id = expired.id AND r.status = 'PENDING'`, limit)
	if err != nil {
		return 0, fmt.Errorf("marketplace: expire offers: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---- Recipients ----

const recipientColumns = `id, organization_id, offer_id, membership_id, status, delivery_status,
	delivered_at, viewed_at, responded_at, client_response_id, reason_code, reason_text,
	expires_at, version, created_at, updated_at`

// recipientColumnsRec is the same list qualified for the "rec" alias, used by
// the joined provider-ownership queries below.
const recipientColumnsRec = `rec.id, rec.organization_id, rec.offer_id, rec.membership_id,
	rec.status, rec.delivery_status, rec.delivered_at, rec.viewed_at, rec.responded_at,
	rec.client_response_id, rec.reason_code, rec.reason_text, rec.expires_at,
	rec.version, rec.created_at, rec.updated_at`

// CreateRecipient adds one managed provider to an offer.
func (r *OfferRepository) CreateRecipient(ctx context.Context, q database.Querier, organizationID string, recipient *models.OfferRecipient) error {
	if recipient.ID == "" {
		recipient.ID = ids.New("orc")
	}
	recipient.OrganizationID = organizationID
	recipient.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_offer_recipients
			(id, organization_id, offer_id, membership_id, status, delivery_status,
			 expires_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'PENDING', 'PENDING', $5, 1, now(), now())
		ON CONFLICT (offer_id, membership_id) DO NOTHING`,
		recipient.ID, organizationID, recipient.OfferID, recipient.MembershipID, recipient.ExpiresAt)
	if err != nil {
		return fmt.Errorf("marketplace: create recipient: %w", err)
	}
	return nil
}

// GetRecipient loads one recipient inside the organization scope.
func (r *OfferRepository) GetRecipient(ctx context.Context, organizationID string, recipientID string) (*models.OfferRecipient, error) {
	row := r.db.QueryRow(ctx, `SELECT `+recipientColumns+`
		FROM fleet_partner_provider_offer_recipients
		WHERE organization_id = $1 AND id = $2`, organizationID, recipientID)
	return scanRecipient(row)
}

// LockRecipientForMembership loads a recipient FOR UPDATE and confirms it
// belongs to the responding provider. A provider can never act on another
// provider's recipient row.
func (r *OfferRepository) LockRecipientForMembership(ctx context.Context, q database.Querier, recipientID string, membershipID string) (*models.OfferRecipient, error) {
	row := q.QueryRow(ctx, `SELECT `+recipientColumns+`
		FROM fleet_partner_provider_offer_recipients
		WHERE id = $1 AND membership_id = $2 FOR UPDATE`, recipientID, membershipID)
	return scanRecipient(row)
}

// GetByIDUnscoped loads an offer without an organization predicate.
//
// The caller is the owning execution service, which holds the offer id from
// its own relay and does not know the organization. The organization is
// RETURNED so the caller's own records can be bound against it; it is never
// accepted as input. Not for organization-facing paths.
func (r *OfferRepository) GetByIDUnscoped(ctx context.Context, offerID string) (*models.Offer, error) {
	row := r.db.QueryRow(ctx, `SELECT `+offerColumns+`
		FROM fleet_partner_organization_request_offers WHERE id = $1`, offerID)
	return scanOffer(row)
}

// LockRecipientForProvider locks a provider's own recipient row FOR UPDATE.
//
// Ownership is enforced in SQL by joining the membership to the executing
// provider: a caller holding a valid service signature still cannot answer an
// offer that was made to somebody else.
func (r *OfferRepository) LockRecipientForProvider(ctx context.Context, q database.Querier,
	offerID string, executingProviderID string, sourceService string) (*models.OfferRecipient, error) {
	row := q.QueryRow(ctx, `SELECT `+recipientColumnsRec+`
		FROM fleet_partner_provider_offer_recipients rec
		JOIN fleet_partner_organization_provider_memberships mem
		  ON mem.id = rec.membership_id AND mem.organization_id=rec.organization_id
		JOIN fleet_partner_organization_request_offers o ON o.id=rec.offer_id AND o.organization_id=rec.organization_id
		JOIN fleet_partner_marketplace_request_projections p ON p.id=o.request_projection_id AND p.organization_id=o.organization_id
		WHERE rec.offer_id = $1 AND mem.provider_id = $2 AND mem.service_category=$3 AND p.source_service=$3
		FOR UPDATE OF rec`, offerID, executingProviderID, sourceService)
	return scanRecipient(row)
}

// ListRecipientsForProvider lists offer recipients belonging to one provider.
//
// The join to memberships is the ownership boundary: a provider id can only
// ever reach recipient rows whose membership belongs to that provider.
func (r *OfferRepository) ListRecipientsForProvider(ctx context.Context,
	executingProviderID string, sourceService string, limit int, offset int) ([]models.OfferRecipient, error) {
	rows, err := r.db.Query(ctx, `SELECT `+recipientColumnsRec+`
		FROM fleet_partner_provider_offer_recipients rec
		JOIN fleet_partner_organization_provider_memberships mem ON mem.id = rec.membership_id
		WHERE mem.provider_id = $1 AND mem.service_category = $2
		ORDER BY rec.created_at DESC
		LIMIT $3 OFFSET $4`, executingProviderID, sourceService, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list recipients for provider: %w", err)
	}
	defer rows.Close()

	var recipients []models.OfferRecipient
	for rows.Next() {
		recipient, scanErr := scanRecipientRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		recipients = append(recipients, *recipient)
	}
	return recipients, rows.Err()
}

// GetRecipientForProvider loads one recipient row owned by this provider.
func (r *OfferRepository) GetRecipientForProvider(ctx context.Context, offerID string,
	executingProviderID string, sourceService string) (*models.OfferRecipient, error) {
	row := r.db.QueryRow(ctx, `SELECT `+recipientColumnsRec+`
		FROM fleet_partner_provider_offer_recipients rec
		JOIN fleet_partner_organization_provider_memberships mem ON mem.id = rec.membership_id AND mem.organization_id=rec.organization_id
		JOIN fleet_partner_organization_request_offers o ON o.id=rec.offer_id AND o.organization_id=rec.organization_id
		JOIN fleet_partner_marketplace_request_projections p ON p.id=o.request_projection_id AND p.organization_id=o.organization_id
		WHERE rec.offer_id = $1 AND mem.provider_id = $2 AND mem.service_category = $3 AND p.source_service=$3`,
		offerID, executingProviderID, sourceService)
	return scanRecipient(row)
}

// FindRecipientByClientResponse replays a redelivered provider decision.
func (r *OfferRepository) FindRecipientByClientResponse(ctx context.Context,
	offerID string, clientResponseID string) (*models.OfferRecipient, error) {
	row := r.db.QueryRow(ctx, `SELECT `+recipientColumnsRec+`
		FROM fleet_partner_provider_offer_recipients rec
		WHERE rec.offer_id = $1 AND rec.client_response_id = $2`, offerID, clientResponseID)
	return scanRecipient(row)
}

// ListRecipientsForMembership returns a provider's actionable offers.
func (r *OfferRepository) ListRecipientsForMembership(ctx context.Context, organizationID string, membershipID string, limit int) ([]models.OfferRecipient, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+recipientColumns+`
		FROM fleet_partner_provider_offer_recipients
		WHERE organization_id = $1 AND membership_id = $2 AND status = 'PENDING'
		ORDER BY created_at DESC, id
		LIMIT $3`, organizationID, membershipID, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list recipients: %w", err)
	}
	defer rows.Close()

	var recipients []models.OfferRecipient
	for rows.Next() {
		recipient, err := scanRecipientRows(rows)
		if err != nil {
			return nil, err
		}
		recipients = append(recipients, *recipient)
	}
	return recipients, rows.Err()
}

// SetRecipientStatus resolves one provider's recipient row.
//
// A decline resolves only this recipient: the organization's offer stays open
// to every other provider.
func (r *OfferRepository) SetRecipientStatus(ctx context.Context, q database.Querier, recipientID string, status string, expectedVersion int64, clientResponseID string, reasonCode string, reasonText string) error {
	var clientPtr, codePtr, textPtr *string
	if clientResponseID != "" {
		clientPtr = &clientResponseID
	}
	if reasonCode != "" {
		codePtr = &reasonCode
	}
	if reasonText != "" {
		textPtr = &reasonText
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_offer_recipients SET
			status = $2, responded_at = now(),
			client_response_id = COALESCE($4, client_response_id),
			reason_code = COALESCE($5, reason_code),
			reason_text = COALESCE($6, reason_text),
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $3`,
		recipientID, status, expectedVersion, clientPtr, codePtr, textPtr)
	if err != nil {
		return fmt.Errorf("marketplace: set recipient status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// MarkLosersOfRace resolves every other pending recipient once one provider
// has won, so no one else is left holding an actionable offer.
func (r *OfferRepository) MarkLosersOfRace(ctx context.Context, q database.Querier, offerID string, winnerRecipientID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_offer_recipients SET
			status = 'LOST_RACE', version = version + 1, updated_at = now()
		WHERE offer_id = $1 AND id <> $2 AND status = 'PENDING'`, offerID, winnerRecipientID)
	if err != nil {
		return fmt.Errorf("marketplace: mark race losers: %w", err)
	}
	return nil
}

// RecordRecipientDelivery stores the notification delivery outcome.
func (r *OfferRepository) RecordRecipientDelivery(ctx context.Context, recipientID string, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_provider_offer_recipients SET
			delivery_status = $2, delivered_at = CASE WHEN $2 = 'ACCEPTED' THEN now() ELSE delivered_at END,
			updated_at = now()
		WHERE id = $1`, recipientID, status)
	if err != nil {
		return fmt.Errorf("marketplace: record recipient delivery: %w", err)
	}
	return nil
}

// ---- Acceptance ----

// CreateAcceptance writes the immutable organization acceptance. The unique
// constraint on offer_id makes a second acceptance impossible.
func (r *OfferRepository) CreateAcceptance(ctx context.Context, q database.Querier, organizationID string, acceptance *models.Acceptance) error {
	if acceptance.ID == "" {
		acceptance.ID = ids.New("oac")
	}
	acceptance.OrganizationID = organizationID
	snapshot, err := json.Marshal(orEmptyMap(acceptance.Snapshot))
	if err != nil {
		return fmt.Errorf("marketplace: marshal acceptance snapshot: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_acceptances
			(id, organization_id, offer_id, accepted_by_user_id, accepted_by_membership_id,
			 acceptance_source, source_reservation_id, source_idempotency_key, snapshot,
			 request_id, correlation_id, accepted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())`,
		acceptance.ID, organizationID, acceptance.OfferID, acceptance.AcceptedByUserID,
		acceptance.AcceptedByMembershipID, acceptance.AcceptanceSource,
		acceptance.SourceReservationID, acceptance.SourceIdempotencyKey, snapshot,
		acceptance.RequestID, acceptance.CorrelationID)
	if err != nil {
		return fmt.Errorf("marketplace: create acceptance: %w", err)
	}
	return nil
}

// GetAcceptance loads the acceptance for an offer.
func (r *OfferRepository) GetAcceptance(ctx context.Context, organizationID string, offerID string) (*models.Acceptance, error) {
	var acceptance models.Acceptance
	var snapshot []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, offer_id, accepted_by_user_id, accepted_by_membership_id,
			acceptance_source, source_reservation_id, source_idempotency_key, snapshot,
			request_id, correlation_id, accepted_at
		FROM fleet_partner_organization_acceptances
		WHERE organization_id = $1 AND offer_id = $2`, organizationID, offerID).
		Scan(&acceptance.ID, &acceptance.OrganizationID, &acceptance.OfferID,
			&acceptance.AcceptedByUserID, &acceptance.AcceptedByMembershipID,
			&acceptance.AcceptanceSource, &acceptance.SourceReservationID,
			&acceptance.SourceIdempotencyKey, &snapshot, &acceptance.RequestID,
			&acceptance.CorrelationID, &acceptance.AcceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan acceptance: %w", err)
	}
	_ = json.Unmarshal(snapshot, &acceptance.Snapshot)
	return &acceptance, nil
}

func scanOffer(row pgx.Row) (*models.Offer, error) {
	var offer models.Offer
	err := row.Scan(&offer.ID, &offer.OrganizationID, &offer.RequestProjectionID,
		&offer.OperatingMode, &offer.Status, &offer.OfferedAt, &offer.ExpiresAt,
		&offer.ReservedAt, &offer.AcceptedAt, &offer.DeclinedAt, &offer.CancelledAt,
		&offer.ResolvedActorType, &offer.ResolvedActorID, &offer.SourceReservationID,
		&offer.ReasonCode, &offer.ReasonText, &offer.Version, &offer.CreatedAt, &offer.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan offer: %w", err)
	}
	return &offer, nil
}

func scanRecipient(row pgx.Row) (*models.OfferRecipient, error) {
	var recipient models.OfferRecipient
	err := row.Scan(&recipient.ID, &recipient.OrganizationID, &recipient.OfferID,
		&recipient.MembershipID, &recipient.Status, &recipient.DeliveryStatus,
		&recipient.DeliveredAt, &recipient.ViewedAt, &recipient.RespondedAt,
		&recipient.ClientResponseID, &recipient.ReasonCode, &recipient.ReasonText,
		&recipient.ExpiresAt, &recipient.Version, &recipient.CreatedAt, &recipient.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan recipient: %w", err)
	}
	return &recipient, nil
}

func scanRecipientRows(rows pgx.Rows) (*models.OfferRecipient, error) {
	var recipient models.OfferRecipient
	err := rows.Scan(&recipient.ID, &recipient.OrganizationID, &recipient.OfferID,
		&recipient.MembershipID, &recipient.Status, &recipient.DeliveryStatus,
		&recipient.DeliveredAt, &recipient.ViewedAt, &recipient.RespondedAt,
		&recipient.ClientResponseID, &recipient.ReasonCode, &recipient.ReasonText,
		&recipient.ExpiresAt, &recipient.Version, &recipient.CreatedAt, &recipient.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan recipient: %w", err)
	}
	return &recipient, nil
}

var _ = time.Now
