package main

// Authoritative mission read-back.
//
// This is the half of the commit contract that makes just-in-time dispatch
// safe. After an ambiguous create -- a timeout, a reset connection, a response
// nobody could parse -- Vendor does not know whether a vehicle was dispatched.
// There are exactly two safe things to do and retrying the create is not one of
// them: ask the authority, or wait and ask again.
//
// So the contract below is deliberately narrow and its THREE outcomes are
// distinct, because collapsing any two of them causes a real failure:
//
//	found=true            the mission exists; adopt it, never create again
//	found=false, err=nil  DEFINITELY absent; a retry under the same key is safe
//	err != nil            unknown; keep reconciling, create nothing
//
// The dangerous collapse is the second into the third. An unreachable authority
// reported as "absent" is a second vehicle.

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

// ReadMission asks the executing authority whether it holds this mission.
//
// It implements orders.MissionReadAuthority on the same adapter that creates
// missions, so the read and the create are routed to the same client by the
// same rules. A second routing table would eventually send the question to a
// different authority than the one that was asked.
func (a *section17FMissionAuthority) ReadMission(
	ctx context.Context, service orders.LogisticsService, missionRef string,
) (orders.MissionResult, bool, error) {
	if a == nil {
		return orders.MissionResult{}, false, errors.New("no mission authority is configured")
	}
	reference := strings.TrimSpace(missionRef)
	if reference == "" {
		return orders.MissionResult{}, false, errors.New("a mission reference is required")
	}

	if client := a.legacyClientFor(service); client != nil {
		if !client.Configured() {
			return orders.MissionResult{}, false, commercemission.ErrNotConfigured
		}
		mission, err := client.ReadMission(ctx, reference)
		if err != nil {
			if errors.Is(err, commercemission.ErrMissionNotFound) {
				// The authority answered, and its answer is no. This is the
				// only path that permits a retry.
				return orders.MissionResult{}, false, nil
			}
			return orders.MissionResult{}, false, err
		}
		return haulingMissionResult(mission), true, nil
	}

	if a.dispatch == nil {
		return orders.MissionResult{}, false, commercemission.ErrNotConfigured
	}
	if !a.dispatch.Configured() {
		return orders.MissionResult{}, false, dispatchcommerce.ErrNotConfigured
	}
	// Dispatch is asked by OPERATION reference, which is the reference Vendor
	// sent as OperationRef when it created the mission. Asking by the
	// authority's own canonical mission id would be asking with a value Vendor
	// does not have until after the create succeeded -- precisely the case
	// where it has nothing.
	mission, err := a.dispatch.ReadOperation(ctx, reference)
	if err != nil {
		if errors.Is(err, dispatchcommerce.ErrMissionNotFound) {
			return orders.MissionResult{}, false, nil
		}
		return orders.MissionResult{}, false, err
	}
	if strings.TrimSpace(mission.MissionRef) == "" {
		// An operation with no canonical reference is not a mission anybody is
		// executing. Treating it as found would adopt a reference that names
		// nothing.
		return orders.MissionResult{}, false, nil
	}
	return orders.MissionResult{
		ExternalRef: mission.MissionRef, Status: orders.MissionAccepted,
	}, true, nil
}
