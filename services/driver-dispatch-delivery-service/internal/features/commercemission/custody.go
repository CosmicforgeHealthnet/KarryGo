package commercemission

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

// Shared multi-manifest custody.
//
// Dispatch keeps its own parcel-level execution record; custody of a
// MANIFEST - who holds a vendor's sealed consignment right now - belongs to
// the shared custodyops model used by Dispatch, Taxi and Hauling alike. Both
// are written in the SAME transaction, so a parcel can never be picked up in
// dispatch's record while the manifest sits uncollected in custody.
//
// Custody is optional at wiring time: a nil repository skips every call here
// and leaves dispatch exactly as it behaved before.

// CustodyDropoffStopRef is the mission's single final stop in the shared
// custody topology. Dispatch stores only pickup stops of its own.
const CustodyDropoffStopRef = "dropoff"

// WithCustody attaches the shared custody model and the media authorizer its
// evidence requires.
func (r *Repository) WithCustody(custody *custodyops.Repository, media custodyops.MediaAuthorizer) *Repository {
	r.custody, r.custodyMedia = custody, media
	return r
}

func custodyReceiverChallengeRef(missionRef string) string { return "delivery:" + missionRef }

// custodyRegistration builds the shared mission topology from the commerce
// request: every pickup stop in route order, one manifest per stop, and the
// packages that manifest covers.
func custodyRegistration(missionRef, tenantRef, operationRef, correlationID string, ordered []dispatchcommerce.PickupStop, parcels []dispatchcommerce.Parcel) custodyops.RegisterMissionCommand {
	command := custodyops.RegisterMissionCommand{
		OperationRef: operationRef, CorrelationID: correlationID, MissionRef: missionRef, TenantRef: tenantRef,
		ReceiverChallengeRef: custodyReceiverChallengeRef(missionRef), SourceVersion: 1,
	}
	packages := map[string][]custodyops.PackageInput{}
	for _, parcel := range parcels {
		key := parcel.ManifestRef
		packages[key] = append(packages[key], custodyops.PackageInput{
			PackageRef: parcel.ParcelRef, PackageCount: parcel.PackageCount,
			WeightGrams: parcel.WeightGrams, VolumeML: parcel.VolumeMl,
		})
	}
	for sequence, stop := range ordered {
		command.Stops = append(command.Stops, custodyops.StopInput{StopRef: stop.StopRef, StopType: "PICKUP", Sequence: sequence + 1})
		command.Manifests = append(command.Manifests, custodyops.ManifestInput{
			ManifestRef: stop.ManifestRef, ManifestVersion: stop.ManifestVersion, PickupStopRef: stop.StopRef,
			SourceAuthority: "VENDOR", SourceManifestChecksum: stop.ManifestRef, VendorRef: stop.VendorRef,
			VendorBranchRef: stop.BranchRef, Packages: packages[stop.ManifestRef],
		})
	}
	command.Stops = append(command.Stops, custodyops.StopInput{StopRef: CustodyDropoffStopRef, StopType: "DROPOFF", Sequence: len(ordered) + 1})
	return command
}

// manifestKeysForStop reads the authoritative manifest identity of one stop.
// Custody is asked about what the mission record actually holds, never about
// what a caller claims.
func manifestKeysForStop(ctx context.Context, tx pgx.Tx, missionID, stopRef string) ([]custodyops.ManifestKey, error) {
	var key custodyops.ManifestKey
	if err := tx.QueryRow(ctx, `SELECT manifest_ref, manifest_version FROM commerce_mission_stops
		WHERE commerce_mission_id=$1::uuid AND stop_ref=$2`, missionID, stopRef).Scan(&key.ManifestRef, &key.ManifestVersion); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key.ManifestRef) == "" {
		return nil, nil
	}
	return []custodyops.ManifestKey{key}, nil
}

func missionCustodyIdentity(ctx context.Context, tx pgx.Tx, missionID string) (missionRef, tenantRef string, err error) {
	err = tx.QueryRow(ctx, `SELECT canonical_ref, parent_order_ref FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&missionRef, &tenantRef)
	return missionRef, tenantRef, err
}

// collectCustodyTx records that this provider now holds the manifests
// collected at one stop, in the same transaction as the pickup itself.
func (r *Repository) collectCustodyTx(ctx context.Context, tx pgx.Tx, missionID, stopRef, providerID, operationRef, challengeRef, verifiedBy string, sourceVersion int64) error {
	if r.custody == nil {
		return nil
	}
	missionRef, tenantRef, err := missionCustodyIdentity(ctx, tx, missionID)
	if err != nil {
		return err
	}
	keys, err := manifestKeysForStop(ctx, tx, missionID, stopRef)
	if err != nil || len(keys) == 0 {
		return err
	}
	_, err = r.custody.CollectManifestsTx(ctx, tx, custodyops.CollectManifestsCommand{
		OperationRef: operationRef, CorrelationID: missionRef, MissionRef: missionRef, TenantRef: tenantRef,
		StopRef: stopRef, ProviderRef: providerID, HandoverChallengeRef: challengeRef,
		HandoverVerifiedByRef: verifiedBy, SourceVersion: sourceVersion, Manifests: keys,
	})
	return err
}

// deliverCustodyTx accounts for every required manifest at delivery. When
// custody reports the mission is not DELIVERED, some required manifest is
// still unaccounted for and completion is refused rather than recorded.
func (r *Repository) deliverCustodyTx(ctx context.Context, tx pgx.Tx, missionID, providerID, operationRef string, sourceVersion int64) error {
	if r.custody == nil {
		return nil
	}
	missionRef, tenantRef, err := missionCustodyIdentity(ctx, tx, missionID)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT manifest_ref, manifest_version FROM commerce_mission_stops
		WHERE commerce_mission_id=$1::uuid AND manifest_ref <> ''`, missionID)
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
	result, err := r.custody.DeliverManifestsTx(ctx, tx, custodyops.DeliverManifestsCommand{
		OperationRef: operationRef, CorrelationID: missionRef, MissionRef: missionRef, TenantRef: tenantRef,
		DropoffStopRef: CustodyDropoffStopRef, ProviderRef: providerID,
		DeliveryChallengeRef: custodyReceiverChallengeRef(missionRef), SourceVersion: sourceVersion, Manifests: keys,
	})
	if err != nil {
		return err
	}
	if result.State != "DELIVERED" {
		return custodyops.ErrManifestsUnaccounted
	}
	return nil
}

// assignCustodyReplacementTx tells the shared ledger that this mission needs
// a new custodian and who it will be, in the same transaction dispatch
// assigns its own replacement provider. Custody and dispatch therefore never
// disagree about whether a transfer is pending.
func (r *Repository) assignCustodyReplacementTx(ctx context.Context, tx pgx.Tx, missionID, oldProvider, newProvider, operationRef string, now time.Time) error {
	if r.custody == nil || oldProvider == "" || newProvider == "" {
		return nil
	}
	missionRef, tenantRef, err := missionCustodyIdentity(ctx, tx, missionID)
	if err != nil {
		return err
	}
	held, packages, err := heldManifestsTx(ctx, tx, missionRef, oldProvider)
	if err != nil || len(held) == 0 {
		// Nothing is in this provider's custody, so there is nothing to hand
		// over. Dispatch's own replacement proceeds unchanged.
		return err
	}
	if _, err = r.custody.ReportIncidentTx(ctx, tx, custodyops.IncidentCommand{
		OperationRef: operationRef + ":incident", MissionRef: missionRef, TenantRef: tenantRef,
		ProviderRef: oldProvider, Reason: custodyops.IncidentBreakdown, OccurredAt: now,
	}); err != nil && !errors.Is(err, custodyops.ErrConflict) {
		return err
	}
	_, err = r.custody.AssignManifestReplacementTx(ctx, tx, custodyops.AssignReplacementCommand{
		OperationRef: operationRef + ":replacement", MissionRef: missionRef, TenantRef: tenantRef,
		OldProviderRef: oldProvider, NewProviderRef: newProvider, VehicleClass: "MOTORCYCLE",
		CapacityCount: packages, ComplianceRef: "dispatch:" + newProvider, AvailabilityRef: "dispatch:" + newProvider,
		ManifestRef: held[0].ManifestRef, ManifestVersion: held[0].ManifestVersion,
	})
	if errors.Is(err, custodyops.ErrConflict) {
		// The ledger already expects this transfer (a retry of the same
		// reconciliation). That is not a failure.
		return nil
	}
	return err
}

// heldManifestsTx reads exactly what one provider currently holds.
func heldManifestsTx(ctx context.Context, tx pgx.Tx, missionRef, providerRef string) ([]custodyops.ManifestKey, int64, error) {
	rows, err := tx.Query(ctx, `SELECT h.manifest_ref, h.manifest_version, COALESCE(sum(p.package_count),0)
		FROM custody_manifest_holdings h
		LEFT JOIN custody_manifest_packages p ON p.mission_ref=h.mission_ref AND p.manifest_ref=h.manifest_ref AND p.manifest_version=h.manifest_version
		WHERE h.mission_ref=$1 AND h.custodian_ref=$2 AND h.released_at IS NULL
		GROUP BY h.manifest_ref, h.manifest_version ORDER BY h.manifest_ref`, missionRef, providerRef)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var keys []custodyops.ManifestKey
	var packages int64
	for rows.Next() {
		var key custodyops.ManifestKey
		var count int64
		if err = rows.Scan(&key.ManifestRef, &key.ManifestVersion, &count); err != nil {
			return nil, 0, err
		}
		keys = append(keys, key)
		packages += count
	}
	return keys, packages, rows.Err()
}

// transferCustodyTx moves exactly the manifests the old provider holds to the
// replacement, in the same transaction as dispatch's own transfer. There is
// no window in which dispatch says the new provider carries the consignment
// while custody still says the old one does.
func (r *Repository) transferCustodyTx(ctx context.Context, tx pgx.Tx, missionID, transferRef, oldProvider, newProvider, evidenceRef, operationRef string, sourceVersion int64) error {
	if r.custody == nil || evidenceRef == "" {
		return nil
	}
	missionRef, tenantRef, err := missionCustodyIdentity(ctx, tx, missionID)
	if err != nil {
		return err
	}
	keys, _, err := heldManifestsTx(ctx, tx, missionRef, oldProvider)
	if err != nil || len(keys) == 0 {
		return err
	}
	_, err = r.custody.TransferManifestsTx(ctx, tx, custodyops.TransferManifestsCommand{
		OperationRef: operationRef + ":transfer", CorrelationID: missionRef, MissionRef: missionRef, TenantRef: tenantRef,
		TransferRef: transferRef, OldProviderRef: oldProvider, NewProviderRef: newProvider, EvidenceRef: evidenceRef,
		OldProviderConfirm: true, NewProviderConfirm: true,
		NewReceiverChallengeRef: custodyReceiverChallengeRef(missionRef), SourceVersion: sourceVersion, Manifests: keys,
	})
	return err
}

// recordCustodyTransferEvidence writes the custody evidence the shared
// transfer requires. It runs before the transfer transaction because evidence
// is an immutable record: a transfer that then fails leaves evidence of an
// attempt, never a custody change nobody performed.
func (r *Repository) recordCustodyTransferEvidence(ctx context.Context, missionRef, operationRef, providerID, mediaRef string, parcelRefs []string, now time.Time) (string, error) {
	if r.custody == nil || r.custodyMedia == nil || mediaRef == "" {
		return "", nil
	}
	var tenantRef string
	if err := r.db.QueryRow(ctx, `SELECT parent_order_ref FROM commerce_missions WHERE canonical_ref=$1`, missionRef).Scan(&tenantRef); err != nil {
		return "", err
	}
	view, err := r.custody.ReadMissionCustody(ctx, missionRef, tenantRef)
	if err != nil {
		return "", err
	}
	var held custodyops.ManifestKey
	for _, holding := range view.Holdings {
		if holding.CustodianRef == providerID {
			held = holding.ManifestKey
			break
		}
	}
	if held.ManifestRef == "" {
		return "", nil
	}
	evidence, err := r.custody.RecordEvidence(ctx, custodyops.EvidenceCommand{
		OperationRef: operationRef + ":transfer-evidence", MissionRef: missionRef, TenantRef: tenantRef,
		ManifestRef: held.ManifestRef, ManifestVersion: held.ManifestVersion, ParcelRefs: parcelRefs,
		EvidenceType: custodyops.EvidenceProviderTransfer, MediaRef: mediaRef, ProviderRef: providerID,
		ActorRef: providerID, CapturedAt: now, AccessPolicy: "INTERNAL",
	}, r.custodyMedia)
	if err != nil {
		return "", err
	}
	return evidence.ID, nil
}
