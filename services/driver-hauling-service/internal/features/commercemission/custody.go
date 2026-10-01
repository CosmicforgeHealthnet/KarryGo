package commercemission

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/custodyops"
)

// Shared multi-manifest custody registration.
//
// A commerce haulage carries sealed vendor manifests, so the mission is
// registered in the shared custodyops ledger in the SAME transaction that
// creates it: a committed commerce mission always has a custody record, and a
// custody record never exists for a mission that was never created.

// CustodyDropoffStopRef is the mission's single final stop.
const CustodyDropoffStopRef = "dropoff"

// WithCustody attaches the shared custody model. nil keeps the pre-custody
// behaviour.
func (r *Repository) WithCustody(custody *custodyops.Repository) *Repository {
	r.custody = custody
	return r
}

// custodyRegistration builds the shared topology from the commerce request:
// every pickup stop in order, the manifest sealed at each, and its packages.
func custodyRegistration(request commercemission.Request) (custodyops.RegisterMissionCommand, bool) {
	command := custodyops.RegisterMissionCommand{
		OperationRef: request.OperationReference(), CorrelationID: request.ParentOrderRef,
		MissionRef: request.MissionRef, TenantRef: request.ParentOrderRef,
		ReceiverChallengeRef: "delivery:" + request.MissionRef, SourceVersion: 1,
	}
	packages := map[string][]custodyops.PackageInput{}
	for _, parcel := range request.Parcels {
		packages[parcel.ManifestRef] = append(packages[parcel.ManifestRef], custodyops.PackageInput{
			PackageRef: parcel.ParcelRef, PackageCount: parcel.PackageCount,
			WeightGrams: parcel.WeightGrams,
		})
	}
	stops := request.PickupStops
	if len(stops) == 0 {
		// A legacy single-pickup request carries its manifest on the request
		// itself; there is one stop and one manifest.
		if manifestRef(request) == "" {
			return custodyops.RegisterMissionCommand{}, false
		}
		stops = []commercemission.PickupStop{{
			StopRef: "pickup", ManifestRef: manifestRef(request), ManifestVersion: manifestVersion(request),
		}}
	}
	for sequence, stop := range stops {
		stopRef := strings.TrimSpace(stop.StopRef)
		if stopRef == "" || strings.TrimSpace(stop.ManifestRef) == "" || stop.ManifestVersion < 1 {
			return custodyops.RegisterMissionCommand{}, false
		}
		command.Stops = append(command.Stops, custodyops.StopInput{StopRef: stopRef, StopType: "PICKUP", Sequence: sequence + 1})
		command.Manifests = append(command.Manifests, custodyops.ManifestInput{
			ManifestRef: stop.ManifestRef, ManifestVersion: stop.ManifestVersion, PickupStopRef: stopRef,
			SourceAuthority: "VENDOR", SourceManifestChecksum: stop.ManifestRef,
			VendorRef: stop.VendorRef, VendorBranchRef: stop.BranchRef, Packages: packages[stop.ManifestRef],
		})
	}
	command.Stops = append(command.Stops, custodyops.StopInput{StopRef: CustodyDropoffStopRef, StopType: "DROPOFF", Sequence: len(stops) + 1})
	if len(command.Manifests) == 0 {
		return custodyops.RegisterMissionCommand{}, false
	}
	return command, true
}

// registerCustodyTx registers the mission when it carries manifests.
func (r *Repository) registerCustodyTx(ctx context.Context, tx pgx.Tx, request commercemission.Request) error {
	if r.custody == nil {
		return nil
	}
	command, ok := custodyRegistration(request)
	if !ok {
		return nil
	}
	_, err := r.custody.RegisterMissionTx(ctx, tx, command)
	return err
}
