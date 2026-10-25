// Package referralhttp exposes referral-service's HTTP surface: internal
// (service-signed) routes and, from later steps, the customer/driver routes.
package referralhttp

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/referral-service/internal/database"
	"cosmicforge/logistics/services/referral-service/internal/events"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxEventBody = 1 << 20

// InternalOptions configures the service-signed routes.
type InternalOptions struct {
	Service  *referralusecases.Service
	Verifier *serviceauth.Verifier
	// NonceStore records each signed request's nonce to reject replays.
	NonceStore         database.Querier
	AdminCallers       []string
	LegacyAdminEnabled bool
	// WithdrawalCheckCallers may ask whether a driver may cash out
	// (payment-wallet-service).
	WithdrawalCheckCallers []string
}

// RegisterInternalRoutes mounts /internal/referral/v1 on the root router.
// Every route requires a valid service signature and a fresh nonce; each
// group additionally names which callers may use it.
func RegisterInternalRoutes(router *gin.Engine, opts InternalOptions) {
	h := &internalHandler{service: opts.Service}
	group := router.Group("/internal/referral/v1")
	group.Use(serviceauth.Middleware(opts.Verifier), events.RejectReplayedNonce(opts.NonceStore))

	group.POST("/events", events.RequireCallers(referralusecases.Producers()...), h.ingestEvent)
	group.POST("/withdrawal-eligibility", events.RequireCallers(opts.WithdrawalCheckCallers...), h.withdrawalEligibility)

	if opts.LegacyAdminEnabled {
		admin := group.Group("", events.RequireCallers(opts.AdminCallers...))
		admin.GET("/activity/:actorType/:actorId", h.activity)
		admin.GET("/admin/accounts/:actorType/:actorId", h.accountOverview)
		admin.GET("/admin/flags", h.listFlags)
		admin.POST("/admin/flags/:id/resolve", h.resolveFlag)
		admin.POST("/admin/persons/:id/:action", h.personAction)
		admin.POST("/admin/referrals/:id/:action", h.referralAction)
		admin.POST("/admin/rewards/:id/reverse", h.reverseReward)
		admin.GET("/admin/reconciliation", h.openMismatches)
	}
}

type internalHandler struct {
	service *referralusecases.Service
}

// ingestEvent receives one relayed outbox event. Any 2xx or 409 tells the
// producer's relay the event is settled; anything else makes it retry.
func (h *internalHandler) ingestEvent(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxEventBody+1))
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Event body could not be read.", err))
		return
	}
	if len(raw) > maxEventBody {
		httpx.Abort(c, apperrors.BadRequest("Event body is too large.", nil))
		return
	}
	outcome, err := h.service.HandleEvent(c.Request.Context(), c.GetString("service_name"), raw)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": outcome}})
}

// activity answers "how many paid jobs did this account do in the activity
// window ending at as_of?" for operators.
func (h *internalHandler) activity(c *gin.Context) {
	var asOf time.Time
	if raw := c.Query("as_of"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("as_of must be an RFC 3339 time.", err))
			return
		}
		asOf = parsed
	}
	count, err := h.service.ActivityCount(c.Request.Context(), c.Param("actorType"), c.Param("actorId"), asOf)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"paid_jobs": count}})
}

// adminBody is what every admin action carries: who is acting and why.
type adminBody struct {
	PerformedBy string `json:"performed_by"`
	Reason      string `json:"reason"`
	// ReversePaid, on disqualify and block, also takes back paid rewards.
	ReversePaid bool `json:"reverse_paid"`
	// Outcome, on resolving a flag: cleared or upheld.
	Outcome string `json:"outcome"`
}

func bindAdmin(c *gin.Context) (adminBody, referralusecases.AdminRequest, bool) {
	var body adminBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body must be JSON with performed_by and reason.", err))
		return body, referralusecases.AdminRequest{}, false
	}
	return body, referralusecases.AdminRequest{
		Caller: c.GetString("service_name"), PerformedBy: body.PerformedBy, Reason: body.Reason, ReversePaid: body.ReversePaid,
	}, true
}

func done(c *gin.Context, data gin.H) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *internalHandler) accountOverview(c *gin.Context) {
	view, err := h.service.AccountOverview(c.Request.Context(), c.Param("actorType"), c.Param("actorId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

func (h *internalHandler) listFlags(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	flags, err := h.service.ListFlags(c.Request.Context(), c.DefaultQuery("status", "open"), limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if flags == nil {
		flags = []referralmodels.Flag{}
	}
	done(c, gin.H{"flags": flags})
}

func (h *internalHandler) resolveFlag(c *gin.Context) {
	body, req, ok := bindAdmin(c)
	if !ok {
		return
	}
	if err := h.service.ResolveFlag(c.Request.Context(), c.Param("id"), body.Outcome, req); err != nil {
		httpx.Abort(c, err)
		return
	}
	done(c, gin.H{"flag_id": c.Param("id"), "status": body.Outcome})
}

func (h *internalHandler) personAction(c *gin.Context) {
	_, req, ok := bindAdmin(c)
	if !ok {
		return
	}
	var err error
	switch action := c.Param("action"); action {
	case "freeze":
		err = h.service.FreezePerson(c.Request.Context(), c.Param("id"), req)
	case "clear":
		err = h.service.ClearPerson(c.Request.Context(), c.Param("id"), req)
	case "block":
		err = h.service.BlockPerson(c.Request.Context(), c.Param("id"), req)
	default:
		err = apperrors.NotFound("Unknown person action "+action+"; use freeze, clear or block.", nil)
	}
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	done(c, gin.H{"person_id": c.Param("id"), "action": c.Param("action")})
}

func (h *internalHandler) referralAction(c *gin.Context) {
	_, req, ok := bindAdmin(c)
	if !ok {
		return
	}
	var err error
	switch action := c.Param("action"); action {
	case "freeze":
		err = h.service.FreezeReferral(c.Request.Context(), c.Param("id"), req)
	case "clear":
		err = h.service.ClearReferral(c.Request.Context(), c.Param("id"), req)
	case "disqualify":
		err = h.service.DisqualifyReferral(c.Request.Context(), c.Param("id"), req)
	default:
		err = apperrors.NotFound("Unknown referral action "+action+"; use freeze, clear or disqualify.", nil)
	}
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	done(c, gin.H{"referral_id": c.Param("id"), "action": c.Param("action")})
}

func (h *internalHandler) reverseReward(c *gin.Context) {
	_, req, ok := bindAdmin(c)
	if !ok {
		return
	}
	if err := h.service.ReverseReward(c.Request.Context(), c.Param("id"), req); err != nil {
		httpx.Abort(c, err)
		return
	}
	done(c, gin.H{"reward_id": c.Param("id"), "action": "reverse"})
}

// withdrawalEligibility answers payment-wallet just before a driver's
// referral cash-out is reserved.
func (h *internalHandler) withdrawalEligibility(c *gin.Context) {
	var request referralcontract.WithdrawalEligibilityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	answer, err := h.service.CheckWithdrawal(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": answer})
}

func (h *internalHandler) openMismatches(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	mismatches, err := h.service.OpenMismatches(c.Request.Context(), limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if mismatches == nil {
		mismatches = []referralrepositories.Mismatch{}
	}
	done(c, gin.H{"mismatches": mismatches})
}
