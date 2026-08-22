package fulfilment

import "context"

// Narrow ports. Every adapter lives in the composition root, so this package
// never imports Food, Shop, Checkout or any logistics integration, and a
// planning decision can be tested without any of them.

// LogisticsAuthority resolves the ACTUAL physical profile of a line from the
// catalogue authority that owns it.
//
// It answers physics, not commerce: no price, no availability, no stock. A
// line the authority holds no profile for comes back with Known false, which
// the aggregation reports as missing evidence rather than treating as zero.
//
// Resolution is batched per group because a group is planned as a whole; a
// per-line call would turn one plan into an N+1 against the catalogue.
type LogisticsAuthority interface {
	// ResolveLineProfiles returns profiles keyed by CheckoutLineID. A missing
	// key is a normal answer, not an error.
	//
	// businessID is the server-resolved Section 17B group evidence and is
	// passed explicitly so the adapter can scope every catalogue read to the
	// owning tenant — a line may never resolve against another vendor's
	// catalogue.
	ResolveLineProfiles(
		ctx context.Context,
		businessID string,
		lines []LineLogistics,
	) (map[string]LineProfile, error)
}

// BranchAuthority resolves the pickup point and reach of one branch.
//
// vendor_branch_service_areas (Section 6) is the single authority for reach;
// this port is how planning reads it without importing the vendors feature.
type BranchAuthority interface {
	// ResolveBranch returns the branch pickup location and its service area.
	//
	// found false means the branch does not exist for this tenant, which is a
	// refusal. hasArea false means the branch exists but has stated no
	// service area, which is a different refusal — a branch with no stated
	// reach has not stated an unlimited one.
	ResolveBranch(
		ctx context.Context,
		businessID string,
		branchID string,
	) (branch Branch, found bool, err error)
}

// Branch is the planning view of a vendor branch: where a mission is picked
// up from, and how far the vendor says it reaches.
type Branch struct {
	BusinessID string
	BranchID   string

	Status string

	Latitude  float64
	Longitude float64

	HasServiceArea        bool
	AreaCenterLatitude    float64
	AreaCenterLongitude   float64
	AreaRadiusMeters      int
	AcceptingNewOrders    bool
	HasOperationalSetting bool
}

// Active reports whether the branch may currently be planned against at all.
func (b Branch) Active() bool { return b.Status == "ACTIVE" }
