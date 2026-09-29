package customershare

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"github.com/gin-gonic/gin"
)

func Register(root *gin.Engine, s *Service, customerBoundary, vendorBoundary []gin.HandlerFunc) {
	journeyshare.RegisterRead(root, "/internal/dispatch/v1/customer-share/trips/:sourceRef", s.Read, customerBoundary...)
	journeyshare.RegisterRead(root, "/internal/dispatch/v1/commerce-share/missions/:sourceRef", s.ReadCommerce, vendorBoundary...)
}
func (s *Service) ReadCommerce(ctx context.Context, ref, customer string) (journeyshare.Projection, error) {
	var source string
	if err := s.DB.QueryRow(ctx, `SELECT t.id::text FROM commerce_missions m JOIN trips t ON t.booking_id=m.booking_id WHERE m.mission_ref=$1 AND t.customer_id::text=$2`, ref, customer).Scan(&source); err != nil {
		return journeyshare.Projection{}, apperrors.NotFound("Journey was not found.", nil)
	}
	p, err := s.Read(ctx, source, customer)
	p.SourceRef = ref
	return p, err
}
