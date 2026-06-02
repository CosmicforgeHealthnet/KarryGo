package customer

import (
	"github.com/gin-gonic/gin"

	"karrygo/backend/internal/platform/httpx"
)

type ProfileController struct {
	service *ProfileService
}

func NewProfileController(service *ProfileService) *ProfileController {
	return &ProfileController{service: service}
}

func (controller *ProfileController) GetMe(c *gin.Context) {
	profile, err := controller.service.GetMe(c.Request.Context(), CustomerIdentityFromContext(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	httpx.RespondSuccess(c, 0, "Customer profile loaded.", profile)
}
