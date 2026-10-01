package bookingrepo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/custodyops"
)

// Shared multi-manifest custody for commerce haulage.
//
// A haulage booking that executes a Vendor commerce mission carries sealed
// vendor manifests. Who holds them is answered by the shared custodyops
// ledger - the same model Dispatch and Taxi use - and is written in the SAME
// transaction as the booking transition it follows, so the booking can never
// say the cargo was collected while custody says it was not.
//
// A booking with no commerce mission has no manifests and is untouched, and a
// repository without custody wired behaves exactly as before.

// WithCustody attaches the shared custody model.
func (r *PostgresBookingRepository) WithCustody(custody *custodyops.Repository) *PostgresBookingRepository {
	r.custody = custody
	return r
}

// commerceCustodyTx resolves the custody identity of a booking. ok is false
// for an ordinary customer haulage, which carries no vendor manifest.
func commerceCustodyTx(ctx context.Context, tx pgx.Tx, bookingID string) (missionRef, tenantRef string, ok bool, err error) {
	err = tx.QueryRow(ctx, `SELECT mission_ref, parent_order_ref FROM commerce_missions WHERE booking_id=$1::uuid`, bookingID).Scan(&missionRef, &tenantRef)
	if err == pgx.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return missionRef, tenantRef, true, nil
}

// collectCustodyTx records that the provider now holds every manifest still
// waiting at a pickup stop of this mission.
func (r *PostgresBookingRepository) collectCustodyTx(ctx context.Context, tx pgx.Tx, bookingID, providerID string, now time.Time) error {
	if r.custody == nil || providerID == "" {
		return nil
	}
	missionRef, tenantRef, ok, err := commerceCustodyTx(ctx, tx, bookingID)
	if err != nil || !ok {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT s.stop_ref, m.manifest_ref, m.manifest_version
		FROM custody_stop_manifests m JOIN custody_mission_stops s
		  ON s.mission_ref=m.mission_ref AND s.stop_ref=m.pickup_stop_ref
		WHERE m.mission_ref=$1 AND m.status='REGISTERED'
		ORDER BY s.sequence, m.manifest_ref`, missionRef)
	if err != nil {
		return err
	}
	byStop := map[string][]custodyops.ManifestKey{}
	var order []string
	for rows.Next() {
		var stopRef string
		var key custodyops.ManifestKey
		if err = rows.Scan(&stopRef, &key.ManifestRef, &key.ManifestVersion); err != nil {
			rows.Close()
			return err
		}
		if _, seen := byStop[stopRef]; !seen {
			order = append(order, stopRef)
		}
		byStop[stopRef] = append(byStop[stopRef], key)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, stopRef := range order {
		if _, err = r.custody.CollectManifestsTx(ctx, tx, custodyops.CollectManifestsCommand{
			OperationRef: "haulage:" + bookingID + ":collect:" + stopRef, CorrelationID: missionRef,
			MissionRef: missionRef, TenantRef: tenantRef, StopRef: stopRef, ProviderRef: providerID,
			HandoverChallengeRef: "pickup:" + bookingID + ":" + stopRef, HandoverVerifiedByRef: providerID,
			SourceVersion: now.UTC().Unix(), Manifests: byStop[stopRef],
		}); err != nil {
			return err
		}
	}
	return nil
}

// deliverCustodyTx accounts for every required manifest at delivery. A
// mission whose manifests are not all accounted for is refused rather than
// recorded as delivered.
func (r *PostgresBookingRepository) deliverCustodyTx(ctx context.Context, tx pgx.Tx, bookingID, providerID string, now time.Time) error {
	if r.custody == nil {
		return nil
	}
	missionRef, tenantRef, ok, err := commerceCustodyTx(ctx, tx, bookingID)
	if err != nil || !ok {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT manifest_ref, manifest_version FROM custody_stop_manifests
		WHERE mission_ref=$1 AND status='COLLECTED' ORDER BY manifest_ref`, missionRef)
	if err != nil {
		return err
	}
	var keys []custodyops.ManifestKey
	for rows.Next() {
		var key custodyops.ManifestKey
		if err = rows.Scan(&key.ManifestRef, &key.ManifestVersion); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(keys) == 0 {
		return err
	}
	var dropoff string
	if err = tx.QueryRow(ctx, `SELECT stop_ref FROM custody_mission_stops WHERE mission_ref=$1 AND stop_type='DROPOFF'`, missionRef).Scan(&dropoff); err != nil {
		return err
	}
	result, err := r.custody.DeliverManifestsTx(ctx, tx, custodyops.DeliverManifestsCommand{
		OperationRef: "haulage:" + bookingID + ":deliver", CorrelationID: missionRef, MissionRef: missionRef,
		TenantRef: tenantRef, DropoffStopRef: dropoff, ProviderRef: providerID,
		DeliveryChallengeRef: "delivery:" + missionRef, SourceVersion: now.UTC().Unix(), Manifests: keys,
	})
	if err != nil {
		return err
	}
	if result.State != "DELIVERED" {
		return custodyops.ErrManifestsUnaccounted
	}
	return nil
}
