package customershare

import (
	"cosmicforge/logistics/shared/go/journeyshare"
	"github.com/gin-gonic/gin"
)

func Register(root *gin.Engine, s *Service, boundary []gin.HandlerFunc) {
	journeyshare.RegisterRead(root, "/internal/vendor/v1/customer-share/missions/:sourceRef", s.Read, boundary...)
}
