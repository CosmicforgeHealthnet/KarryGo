// Package http exposes pricing's administrative surface.
//
// Every route here is internal and signed: administration reaches pricing as an
// HMAC-signed request from admin-backoffice-service or the ops CLI, naming the
// person responsible in performed_by. Customers and drivers never reach these.
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	approvalmodels "cosmicforge/logistics/services/pricing-service/internal/features/approvals/models"
	geomodels "cosmicforge/logistics/services/pricing-service/internal/features/geography/models"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/operators"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// respond maps a domain error onto the platform's error envelope.
//
// The mapping matters for operators: "you cannot approve your own change" must
// arrive as a 403 with that sentence, not as a 500 with a constraint name.
func respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, approvalmodels.ErrSelfApproval):
		httpx.Abort(c, apperrors.Forbidden("You cannot approve a change you requested. A second operator must decide it.", err))
	case errors.Is(err, operators.ErrMissingRole):
		httpx.Abort(c, apperrors.Forbidden("This operator does not hold the role required for this action.", err))
	case errors.Is(err, operators.ErrUnknownOperator):
		httpx.Abort(c, apperrors.Forbidden("performed_by is not a known active pricing operator.", err))
	case errors.Is(err, errAdminScope):
		httpx.Abort(c, apperrors.Forbidden("The delegated Admin scope does not authorize this Pricing resource.", err))
	case errors.Is(err, approvalmodels.ErrSubjectChanged):
		httpx.Abort(c, apperrors.Conflict("The policy body changed after it was approved. Submit it for approval again.", err))
	case errors.Is(err, approvalmodels.ErrAlreadyDecided):
		httpx.Abort(c, apperrors.Conflict("This request has already been decided.", err))
	case errors.Is(err, policymodels.ErrNotDraft):
		httpx.Abort(c, apperrors.Conflict("Only a draft can be edited. Create a new version instead.", err))
	case errors.Is(err, policymodels.ErrNotApproved):
		httpx.Abort(c, apperrors.Conflict("Only an approved version can be activated.", err))
	case errors.Is(err, policymodels.ErrStaleVersion):
		httpx.Abort(c, apperrors.Conflict("The pricing policy changed after it was read. Read the current source version before retrying.", err))
	case errors.Is(err, policymodels.ErrNoActivePolicy):
		httpx.Abort(c, apperrors.NotFound("No active pricing policy for that kind, place and time.", err))
	case errors.Is(err, policymodels.ErrNotFound),
		errors.Is(err, approvalmodels.ErrNotFound),
		errors.Is(err, geomodels.ErrNotFound):
		httpx.Abort(c, apperrors.NotFound("Not found.", err))
	default:
		// A validation failure from the engine or a model reads as a sentence
		// already; surface it rather than hiding it behind "internal error".
		httpx.Abort(c, apperrors.BadRequest(err.Error(), err))
	}
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}
