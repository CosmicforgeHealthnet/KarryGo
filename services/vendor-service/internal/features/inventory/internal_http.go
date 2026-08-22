package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/platform/servicenonce"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type InternalInventoryAuthority interface {
	ItemBranch(
		context.Context,
		string,
		string,
	) (
		string,
		error,
	)

	Availability(
		context.Context,
		Actor,
		BranchScope,
		string,
	) (
		Balance,
		error,
	)

	CreateReservation(
		context.Context,
		Actor,
		BranchScope,
		CreateReservationInput,
	) (
		Reservation,
		error,
	)

	ReservationBranch(
		context.Context,
		string,
		string,
	) (
		string,
		error,
	)

	GetReservation(
		context.Context,
		Actor,
		BranchScope,
		string,
	) (
		Reservation,
		error,
	)

	ConfirmReservation(
		context.Context,
		Actor,
		BranchScope,
		string,
	) (
		Reservation,
		error,
	)

	CommitReservation(
		context.Context,
		Actor,
		BranchScope,
		string,
	) (
		Reservation,
		error,
	)

	ReleaseReservation(
		context.Context,
		Actor,
		BranchScope,
		string,
	) (
		Reservation,
		error,
	)

	Revision(
		context.Context,
		Actor,
		BranchScope,
		string,
	) (
		int64,
		time.Time,
		error,
	)
}

type internalBranchResolver interface {
	BusinessForBranch(
		context.Context,
		string,
	) (
		string,
		error,
	)
}

type internalNonceConsumer interface {
	Consume(
		context.Context,
		string,
		string,
	) error
}

type postgresInternalBranchResolver struct {
	db *pgxpool.Pool
}

func (r postgresInternalBranchResolver) BusinessForBranch(
	ctx context.Context,
	branchID string,
) (
	string,
	error,
) {
	var businessID string

	err :=
		r.db.QueryRow(
			ctx,
			`SELECT business_id
             FROM vendor_branches
             WHERE id=$1`,
			strings.TrimSpace(
				branchID,
			),
		).Scan(
			&businessID,
		)

	return businessID, err
}

type InternalHandler struct {
	service InternalInventoryAuthority

	branches internalBranchResolver

	nonces internalNonceConsumer

	allowed map[string]bool
}

func callerMap(
	callers []string,
) map[string]bool {
	allowed :=
		map[string]bool{}

	for _, caller := range callers {
		caller =
			strings.TrimSpace(
				caller,
			)

		if caller != "" {
			allowed[caller] = true
		}
	}

	return allowed
}

func NewInternalInventoryHandler(
	service *StockService,
	callers []string,
) *InternalHandler {
	var branches internalBranchResolver
	var nonces internalNonceConsumer

	if service != nil &&
		service.db != nil {

		branches =
			postgresInternalBranchResolver{
				db: service.db,
			}

		nonces =
			servicenonce.NewStore(
				service.db,
			)
	}

	return &InternalHandler{
		service:  service,
		branches: branches,
		nonces:   nonces,
		allowed:  callerMap(callers),
	}
}

func newInternalInventoryHandlerWithDependencies(
	service InternalInventoryAuthority,
	branches internalBranchResolver,
	nonces internalNonceConsumer,
	callers []string,
) *InternalHandler {
	return &InternalHandler{
		service:  service,
		branches: branches,
		nonces:   nonces,
		allowed:  callerMap(callers),
	}
}

func (h *InternalHandler) allowCaller(
	c *gin.Context,
) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)

	caller = strings.TrimSpace(caller)

	if h == nil || !h.allowed[caller] {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	c.Next()
}
func (h *InternalHandler) requireNonce(
	c *gin.Context,
) {
	if h == nil ||
		h.nonces == nil {

		c.AbortWithStatus(
			http.StatusUnauthorized,
		)

		return
	}

	value, _ :=
		c.Get(
			"service_name",
		)

	caller, _ :=
		value.(string)

	nonce :=
		strings.TrimSpace(
			c.GetHeader(
				serviceauth.HeaderNonce,
			),
		)

	if nonce == "" {
		c.AbortWithStatus(
			http.StatusUnauthorized,
		)

		return
	}

	if err :=
		h.nonces.Consume(
			c,
			caller,
			nonce,
		); err != nil {

		c.AbortWithStatus(
			http.StatusUnauthorized,
		)

		return
	}

	c.Next()
}

func internalInventoryRespond(
	c *gin.Context,
	status int,
	data any,
) {
	c.JSON(
		status,
		gin.H{
			"success": true,
			"data":    data,
		},
	)
}

func internalInventoryNotFound(
	c *gin.Context,
) {
	c.AbortWithStatus(
		http.StatusNotFound,
	)
}

func decodeInternalInventory(
	c *gin.Context,
	target any,
) error {
	raw,
		err :=
		io.ReadAll(
			io.LimitReader(
				c.Request.Body,
				1<<20,
			),
		)

	if err != nil {
		return err
	}

	decoder :=
		json.NewDecoder(
			bytes.NewReader(
				raw,
			),
		)

	decoder.DisallowUnknownFields()

	if err =
		decoder.Decode(
			target,
		); err != nil {

		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	return nil
}

func (h *InternalHandler) actorForBranch(
	c *gin.Context,
) (
	Actor,
	BranchScope,
	bool,
) {
	if h == nil ||
		h.service == nil ||
		h.branches == nil {

		c.AbortWithStatus(
			http.StatusServiceUnavailable,
		)

		return Actor{},
			BranchScope{},
			false
	}

	branchID :=
		strings.TrimSpace(
			c.Param(
				"branchId",
			),
		)

	businessID,
		err :=
		h.branches.
			BusinessForBranch(
				c,
				branchID,
			)

	if err != nil ||
		strings.TrimSpace(
			businessID,
		) == "" {

		internalInventoryNotFound(c)

		return Actor{},
			BranchScope{},
			false
	}

	value, _ :=
		c.Get(
			"service_name",
		)

	caller, _ :=
		value.(string)

	requestID :=
		httpx.GetRequestID(
			c,
		)

	actor :=
		Actor{
			ActorRef: "service:" +
				strings.TrimSpace(
					caller,
				),

			BusinessID: strings.TrimSpace(
				businessID,
			),

			CorrelationID: requestID,

			RequestID: requestID,
		}

	scope,
		err :=
		ExactBranchScope(
			actor.BusinessID,
			branchID,
		)

	if err != nil {
		c.AbortWithStatus(
			http.StatusForbidden,
		)

		return Actor{},
			BranchScope{},
			false
	}

	return actor,
		scope,
		true
}

func (h *InternalHandler) availability(
	c *gin.Context,
) {
	actor,
		scope,
		ok :=
		h.actorForBranch(c)

	if !ok {
		return
	}

	itemID :=
		strings.TrimSpace(
			c.Param(
				"inventoryItemId",
			),
		)

	itemBranch,
		err :=
		h.service.ItemBranch(
			c,
			actor.BusinessID,
			itemID,
		)

	if err != nil ||
		itemBranch !=
			strings.TrimSpace(
				c.Param(
					"branchId",
				),
			) {

		internalInventoryNotFound(c)
		return
	}

	out,
		err :=
		h.service.Availability(
			c,
			actor,
			scope,
			itemID,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	internalInventoryRespond(
		c,
		http.StatusOK,
		out,
	)
}

func (h *InternalHandler) reserve(
	c *gin.Context,
) {
	actor,
		scope,
		ok :=
		h.actorForBranch(c)

	if !ok {
		return
	}

	var input CreateReservationInput

	if err :=
		decodeInternalInventory(
			c,
			&input,
		); err != nil {

		httpx.RespondError(c, err)
		return
	}

	pathBranch :=
		strings.TrimSpace(
			c.Param(
				"branchId",
			),
		)

	if input.BranchID != "" &&
		strings.TrimSpace(
			input.BranchID,
		) != pathBranch {

		internalInventoryNotFound(c)
		return
	}

	input.BranchID =
		pathBranch

	for _, line := range input.Lines {

		itemBranch,
			err :=
			h.service.ItemBranch(
				c,
				actor.BusinessID,
				line.InventoryItemID,
			)

		if err != nil ||
			itemBranch != pathBranch {

			internalInventoryNotFound(c)
			return
		}
	}

	out,
		err :=
		h.service.CreateReservation(
			c,
			actor,
			scope,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			reservationHTTPError(
				err,
			),
		)

		return
	}

	internalInventoryRespond(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *InternalHandler) boundReservation(
	c *gin.Context,
) (
	Actor,
	BranchScope,
	string,
	bool,
) {
	actor,
		scope,
		ok :=
		h.actorForBranch(c)

	if !ok {
		return Actor{},
			BranchScope{},
			"",
			false
	}

	reservationID :=
		strings.TrimSpace(
			c.Param(
				"reservationId",
			),
		)

	branchID,
		err :=
		h.service.ReservationBranch(
			c,
			actor.BusinessID,
			reservationID,
		)

	if err != nil ||
		branchID !=
			strings.TrimSpace(
				c.Param(
					"branchId",
				),
			) {

		internalInventoryNotFound(c)

		return Actor{},
			BranchScope{},
			"",
			false
	}

	return actor,
		scope,
		reservationID,
		true
}

func (h *InternalHandler) reservation(
	c *gin.Context,
) {
	actor,
		scope,
		reservationID,
		ok :=
		h.boundReservation(c)

	if !ok {
		return
	}

	out,
		err :=
		h.service.GetReservation(
			c,
			actor,
			scope,
			reservationID,
		)

	if err != nil {
		httpx.RespondError(
			c,
			reservationHTTPError(
				err,
			),
		)

		return
	}

	internalInventoryRespond(
		c,
		http.StatusOK,
		out,
	)
}

func (h *InternalHandler) reservationTransition(
	target string,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor,
			scope,
			reservationID,
			ok :=
			h.boundReservation(c)

		if !ok {
			return
		}

		var out Reservation
		var err error

		switch target {
		case ReservationConfirmed:
			out,
				err =
				h.service.
					ConfirmReservation(
						c,
						actor,
						scope,
						reservationID,
					)

		case ReservationCommitted:
			out,
				err =
				h.service.
					CommitReservation(
						c,
						actor,
						scope,
						reservationID,
					)

		case ReservationReleased:
			out,
				err =
				h.service.
					ReleaseReservation(
						c,
						actor,
						scope,
						reservationID,
					)

		default:
			err =
				ErrReservationStateConflict
		}

		if err != nil {
			httpx.RespondError(
				c,
				reservationHTTPError(
					err,
				),
			)

			return
		}

		internalInventoryRespond(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *InternalHandler) revision(
	c *gin.Context,
) {
	actor,
		scope,
		ok :=
		h.actorForBranch(c)

	if !ok {
		return
	}

	branchID :=
		strings.TrimSpace(
			c.Param(
				"branchId",
			),
		)

	revision,
		updatedAt,
		err :=
		h.service.Revision(
			c,
			actor,
			scope,
			branchID,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	internalInventoryRespond(
		c,
		http.StatusOK,
		gin.H{
			"revision": revision,

			"updated_at": updatedAt,
		},
	)
}

func RegisterInternalRoutes(
	router *gin.Engine,
	h *InternalHandler,
	verifier *serviceauth.Verifier,
	flags Flags,
) {
	if router == nil ||
		h == nil ||
		verifier == nil ||
		!flags.Enabled {

		return
	}

	group :=
		router.Group(
			"/internal/vendor/v1/inventory",
		)

	group.Use(
		serviceauth.Middleware(
			verifier,
		),
		h.allowCaller,
		h.requireNonce,
	)

	branches :=
		group.Group(
			"/branches/:branchId",
		)

	branches.GET(
		"/items/:inventoryItemId/availability",
		h.availability,
	)

	branches.GET(
		"/revision",
		h.revision,
	)

	branches.GET(
		"/reservations/:reservationId",
		h.reservation,
	)

	if !flags.WritesEnabled {
		return
	}

	branches.POST(
		"/reservations",
		h.reserve,
	)

	branches.POST(
		"/reservations/:reservationId/confirm",
		h.reservationTransition(
			ReservationConfirmed,
		),
	)

	branches.POST(
		"/reservations/:reservationId/commit",
		h.reservationTransition(
			ReservationCommitted,
		),
	)

	branches.POST(
		"/reservations/:reservationId/release",
		h.reservationTransition(
			ReservationReleased,
		),
	)
}
