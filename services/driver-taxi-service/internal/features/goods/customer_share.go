package goods

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/serviceauth"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// RegisterCustomerShare mounts a distinct Goods contract using the existing
// Vendor-only HMAC and durable nonce boundary. Passenger tracking is unrelated.
func RegisterCustomerShare(router *gin.Engine, service *Service, verifier *serviceauth.Verifier) {
	if verifier == nil || service == nil {
		return
	}
	handler := &Handler{service: service}
	journeyshare.RegisterRead(router, "/internal/taxi/v1/commerce-share/missions/:sourceRef", service.CustomerShare, serviceauth.Middleware(verifier), handler.vendorOnlyAndNonce())
}

func (s *Service) CustomerShare(ctx context.Context, ref, customer string) (journeyshare.Projection, error) {
	p := journeyshare.Projection{Authority: "TAXI_GOODS", SourceRef: ref, ProductCode: "COMMERCE_MISSION", Location: journeyshare.Location{State: "UNAVAILABLE"}}
	if s == nil || !s.enabled || s.repo == nil {
		return p, apperrors.Unavailable("Taxi Goods tracking is unavailable.", nil)
	}
	var name, make, model, plate string
	err := s.repo.db.QueryRow(ctx, `SELECT m.customer_ref,m.status,m.version,COALESCE(m.completed_at,m.cancelled_at),
 COALESCE(p.first_name,''),COALESCE(v.make,''),COALESCE(v.model,''),COALESCE(v.plate_number,'')
 FROM taxi_goods_missions m LEFT JOIN taxi_providers p ON p.id=m.provider_id
 LEFT JOIN vehicles v ON v.id=m.vehicle_id AND v.provider_id=m.provider_id AND v.status='active'
 WHERE m.source_service='vendor-service' AND m.mission_ref=$1 AND m.customer_ref=$2`, ref, customer).
		Scan(&p.CustomerID, &p.Status, &p.SourceVersion, &p.CompletedAt, &name, &make, &model, &plate)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.NotFound("Mission was not found.", nil)
	}
	if err != nil {
		return p, apperrors.Unavailable("Taxi Goods tracking is unavailable.", nil)
	}
	p.Terminal = p.Status == StatusCompleted || p.Status == StatusCancelled || p.Status == StatusFailed || p.Status == StatusUnmatched
	p.Eligible = !p.Terminal
	p.PublicStatus = p.Status
	p.Progress = journeyshare.Progress{Stage: p.Status}
	if name != "" {
		p.Provider = &journeyshare.Provider{DisplayName: name}
		if make != "" || model != "" || plate != "" {
			p.Provider.Vehicle = &journeyshare.Vehicle{Make: make, Model: model, Plate: plate}
		}
	}
	// Goods has no authoritative mission-bound telemetry ingestion. Its global
	// availability and passenger telemetry must never substitute for that source.
	return p, nil
}
