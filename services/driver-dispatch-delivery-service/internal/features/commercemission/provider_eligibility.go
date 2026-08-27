package commercemission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

// CheckProviderForBooking extends the normal marketplace acceptance gate only
// for a versioned Commerce mission. Affiliation is deliberately absent from
// the query: affiliated and independent providers use the same public-market
// vehicle and standing rules.
func (r *Repository) CheckProviderForBooking(ctx context.Context, bookingID, providerID string) error {
	if r == nil || r.db == nil {
		return apperrors.Unavailable("Provider vehicle eligibility is unavailable.", nil)
	}
	var commerce bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_missions WHERE booking_id=$1::uuid AND plan_version>0 AND runtime_state IN ('SEARCHING','QUEUED','RETRYING','REPLACEMENT_REQUIRED'))`, bookingID).Scan(&commerce)
	if err != nil {
		return err
	}
	if !commerce {
		return nil
	}
	var eligible bool
	err = r.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM providers p JOIN bikes b ON b.provider_id=p.id
		WHERE p.id=$1::uuid AND p.is_active AND p.onboarding_complete
		  AND p.verification_status IN ('verified','fully_approved')
		  AND b.is_active AND b.verification_status='verified'
		  AND b.bike_type IN ('motorcycle','dispatch_bike','scooter','electric_bike')
		  AND (SELECT count(DISTINCT d.document_type) FROM bike_documents d
		       WHERE d.bike_id=b.id AND d.document_type IN ('registration','insurance')
		         AND (d.expiry_date IS NULL OR d.expiry_date>=CURRENT_DATE))=2
	)`, providerID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return apperrors.New(409, "PROVIDER_VEHICLE_INELIGIBLE", "An active verified motorcycle with current documents is required.", nil)
	}
	return nil
}

func (r *Repository) JobCardForBooking(ctx context.Context, bookingID string) (*dispatchcommerce.JobCard, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	record, err := scanMultiStopMission(r.db.QueryRow(ctx, `SELECT `+multiStopMissionColumns+` FROM commerce_missions WHERE booking_id=$1::uuid AND plan_version>0`, bookingID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	view, err := r.loadMissionView(ctx, record)
	if err != nil {
		return nil, err
	}
	var snapshot dispatchcommerce.MissionCreateRequest
	var raw []byte
	if err = r.db.QueryRow(ctx, `SELECT request_snapshot FROM commerce_missions WHERE id=$1::uuid`, record.ID).Scan(&raw); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	return &dispatchcommerce.JobCard{Commerce: true, MissionRef: view.MissionRef,
		ServiceClass: record.ServiceClass, Stops: view.Stops, Receiver: snapshot.Receiver,
		ParcelCount: len(view.Parcels), ProofRequirements: []string{
			"EXACT_PARCEL_AND_PACKAGE_CONFIRMATION_PER_STOP",
			"VENDOR_PICKUP_OTP_PER_STOP",
			"OPAQUE_MEDIA_REFERENCE_WHEN_POLICY_REQUIRES",
			"RECEIVER_HANDOVER_PROOF",
			"PROVIDER_TRANSFER_PROOF_WHEN_REPLACEMENT_IS_POST_CUSTODY",
		}}, nil
}
