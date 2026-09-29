package customershare

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB                 *pgxpool.Pool
	Logistics          journeyshare.Reader
	Enabled            bool
	EnabledAuthorities map[string]bool
}

func (s *Service) Read(ctx context.Context, ref, customer string) (journeyshare.Projection, error) {
	if !s.Enabled {
		return journeyshare.Projection{}, apperrors.Unavailable("Commerce sharing is unavailable.", nil)
	}
	var missionRef, authority, product, status string
	err := s.DB.QueryRow(ctx, `SELECT m.mission_ref,m.service,o.domain,m.status FROM commerce_order_missions m JOIN commerce_parent_orders o ON o.id=m.parent_order_id WHERE (m.id=$1 OR m.mission_ref=$1) AND m.customer_id=$2 AND o.customer_id=$2`, ref, customer).Scan(&missionRef, &authority, &product, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.readOwnDelivery(ctx, ref, customer)
	}
	if err != nil {
		return journeyshare.Projection{}, apperrors.Unavailable("Mission tracking is unavailable.", nil)
	}
	if status != "ACCEPTED" {
		return journeyshare.Projection{}, apperrors.NotFound("Mission is not shareable.", nil)
	}
	authority, ok := executorAuthority(authority)
	if !ok {
		return journeyshare.Projection{}, apperrors.Unavailable("Mission executor is unsupported.", nil)
	}
	if s.EnabledAuthorities != nil && !s.EnabledAuthorities[authority] {
		return journeyshare.Projection{}, apperrors.Unavailable("Mission authority is unavailable.", nil)
	}
	if s.Logistics == nil || (product != "FOOD" && product != "SHOP") {
		return journeyshare.Projection{}, apperrors.Unavailable("Mission tracking is unavailable.", nil)
	}
	p, err := s.Logistics.Read(ctx, authority, missionRef, customer)
	if err != nil {
		return p, err
	}
	if p.CustomerID != customer || p.SourceRef != missionRef || p.Authority != authority || p.SourceVersion < 1 {
		return journeyshare.Projection{}, apperrors.NotFound("Mission was not found.", nil)
	}
	p.Authority = "VENDOR_COMMERCE"
	p.SourceRef = ref
	p.ProductCode = product
	return p, nil
}

// executorAuthority maps a persisted mission service to the owning read
// authority. Anything unrecognised is refused rather than guessed.
func executorAuthority(service string) (string, bool) {
	switch service {
	case "DISPATCH", "HAULING":
		return service, true
	case "TAXI", "TAXI_GOODS":
		// The persisted TAXI commerce authority means Goods, never passenger Ride.
		return "TAXI_GOODS", true
	}
	return "", false
}

// Vendor owns this runtime and its staff assignment. No authoritative location
// or assigned vehicle exists in this aggregate; status remains useful alone.
func (s *Service) readOwnDelivery(ctx context.Context, ref, customer string) (journeyshare.Projection, error) {
	p := journeyshare.Projection{Authority: "VENDOR_COMMERCE", SourceRef: ref, Location: journeyshare.Location{State: "UNAVAILABLE"}}
	if !s.EnabledAuthorities["VENDOR_OWN_DELIVERY"] {
		return p, apperrors.Unavailable("Vendor delivery tracking is unavailable.", nil)
	}
	err := s.DB.QueryRow(ctx, `SELECT o.customer_id,o.domain,r.stage,GREATEST(m.version,r.version),r.updated_at
 FROM commerce_vendor_missions m
 JOIN commerce_fulfilment_groups g ON g.id=m.group_id
 JOIN commerce_parent_orders o ON o.id=g.parent_order_id AND o.customer_id=g.customer_id
 JOIN commerce_fulfilment_plan_missions slot ON slot.id=m.plan_mission_id AND slot.plan_id=m.plan_id AND slot.group_id=m.group_id AND slot.owner='VENDOR' AND slot.authority='VENDOR'
 JOIN commerce_vendor_mission_runtime r ON r.vendor_mission_id=m.id
 WHERE m.id=$1 AND o.customer_id=$2`, ref, customer).Scan(&p.CustomerID, &p.ProductCode, &p.Status, &p.SourceVersion, &p.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.NotFound("Mission was not found.", nil)
	}
	if err != nil {
		return p, apperrors.Unavailable("Vendor delivery tracking is unavailable.", nil)
	}
	if p.ProductCode != "FOOD" && p.ProductCode != "SHOP" {
		return p, apperrors.NotFound("Mission was not found.", nil)
	}
	p.Terminal = p.Status == "COMPLETED" || p.Status == "RETURNED" || p.Status == "FAILED" || p.Status == "FALLBACK_COMPLETED"
	p.Eligible = !p.Terminal && p.Status != "FALLBACK_STARTED"
	p.PublicStatus = p.Status
	p.Progress = journeyshare.Progress{Stage: p.Status}
	if !p.Terminal {
		p.CompletedAt = nil
	}
	// Staff IDs, branch/business IDs, handover/proof refs and failure reasons
	// deliberately never enter the DTO. Display details are omitted until an
	// explicitly customer-safe staff display field is available.
	return p, nil
}
