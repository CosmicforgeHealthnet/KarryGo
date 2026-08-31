package finance_projection

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func financeActor(c *gin.Context) (Actor, error) {
	value, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	return Actor{StaffID: value.StaffID, VendorID: value.VendorID, SessionID: value.SessionID}, nil
}

func financeOK(c *gin.Context, value any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
}

func (h *Handler) Summary(c *gin.Context) {
	actor, err := financeActor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	value, err := h.service.Summary(c, actor)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	financeOK(c, value)
}

func (h *Handler) Transactions(c *gin.Context) {
	actor, err := financeActor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	value, err := h.service.Transactions(c, actor, limit)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	financeOK(c, value)
}

func (h *Handler) BankAccounts(c *gin.Context) {
	actor, err := financeActor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	value, err := h.service.BankAccounts(c, actor)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	financeOK(c, value)
}

func (h *Handler) Withdrawals(c *gin.Context) {
	actor, err := financeActor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	value, err := h.service.Withdrawals(c, actor)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	financeOK(c, value)
}

func (h *Handler) Withdrawal(c *gin.Context) {
	actor, err := financeActor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	value, err := h.service.Withdrawal(c, actor, c.Param("reference"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	financeOK(c, value)
}

type withdrawalRequest struct {
	BankAccountID string `json:"bank_account_id" binding:"required"`
	AmountKobo    int64  `json:"amount_kobo" binding:"required"`
	Currency      string `json:"currency"`
	StepUpGrantID string `json:"step_up_grant_id" binding:"required"`
}

func (h *Handler) RequestWithdrawal(c *gin.Context) {
	actor, err := financeActor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var request withdrawalRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.RespondError(c, apperrors.BadRequest("Request body must contain one JSON value.", err))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	value, err := h.service.RequestWithdrawal(c, actor, WithdrawalCommand{BankAccountID: request.BankAccountID, AmountKobo: request.AmountKobo, Currency: request.Currency, StepUpGrantID: request.StepUpGrantID, IdempotencyKey: key})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": value})
}

func RegisterRoutes(root *gin.RouterGroup, handler *Handler, enabled bool, stores ...*idempotency.Store) {
	if !enabled || handler == nil {
		return
	}
	finance := root.Group("/finance")
	if len(stores) > 0 {
		finance.Use(idempotency.CommandMiddleware(stores[0]))
	}
	finance.GET("/overview", handler.Summary)
	finance.GET("/transactions", handler.Transactions)
	finance.GET("/bank-accounts", handler.BankAccounts)
	finance.GET("/withdrawals", handler.Withdrawals)
	finance.GET("/withdrawals/:reference", handler.Withdrawal)
	finance.POST("/withdrawals", handler.RequestWithdrawal)
}
