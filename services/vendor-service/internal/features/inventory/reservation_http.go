package inventory

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

func reservationHTTPError(
	err error,
) error {
	switch {
	case errors.Is(
		err,
		ErrReservationIdempotencyConflict,
	):
		return apperrors.Conflict(
			"This reservation source reference was already used with different reservation details.",
			err,
		)

	case errors.Is(
		err,
		ErrInsufficientStock,
	):
		return apperrors.Conflict(
			"Insufficient sellable stock is available for this reservation.",
			err,
		)

	case errors.Is(
		err,
		ErrReservationStateConflict,
	):
		return apperrors.Conflict(
			"The reservation can no longer perform this transition.",
			err,
		)

	case errors.Is(
		err,
		ErrReservationExpired,
	):
		return apperrors.Conflict(
			"The reservation has expired.",
			err,
		)

	case errors.Is(
		err,
		ErrReservationStockExpired,
	):
		return apperrors.Conflict(
			"Reserved stock expired before it could be committed.",
			err,
		)

	case errors.Is(
		err,
		ErrReservationNotFound,
	):
		return apperrors.NotFound(
			"Inventory reservation was not found.",
			err,
		)

	case errors.Is(
		err,
		ErrInvalidQuantity,
	),
		errors.Is(
			err,
			ErrReservationSourceInvalid,
		):
		return apperrors.BadRequest(
			"The reservation request is invalid.",
			err,
		)

	default:
		return err
	}
}

func (h *Handler) authorizedBranchPermission(
	c *gin.Context,
	branchID string,
	permission string,
) (
	Actor,
	BranchScope,
	bool,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	if h.scopes == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Inventory access is not permitted.",
				nil,
			),
		)

		return Actor{},
			BranchScope{},
			false
	}

	access, err :=
		h.scopes.ResolveScope(
			c,
			actor.StaffID,
			permission,
		)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	if !access.Covers(
		branchID,
	) {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Inventory access is not permitted for this branch.",
				nil,
			),
		)

		return Actor{},
			BranchScope{},
			false
	}

	scope, err :=
		ExactBranchScope(
			actor.BusinessID,
			branchID,
		)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	return actor,
		scope,
		true
}

func (h *Handler) createReservation(
	c *gin.Context,
) {
	var input CreateReservationInput

	if err := c.ShouldBindJSON(
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"The reservation request body is invalid.",
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			input.BranchID,
			PermissionInventoryReserve,
		)

	if !ok {
		return
	}

	out, err :=
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

	inventoryRespond(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) getReservation(
	c *gin.Context,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	reservationID :=
		c.Param(
			"reservationId",
		)

	branchID, err :=
		h.service.ReservationBranch(
			c,
			actor.BusinessID,
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

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			branchID,
			PermissionInventoryRead,
		)

	if !ok {
		return
	}

	out, err :=
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

	inventoryRespond(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) reservationTransition(
	target string,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, err :=
			h.actor(c)

		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		reservationID :=
			c.Param(
				"reservationId",
			)

		branchID, err :=
			h.service.ReservationBranch(
				c,
				actor.BusinessID,
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

		actor,
			scope,
			ok :=
			h.authorizedBranchPermission(
				c,
				branchID,
				PermissionInventoryReserve,
			)

		if !ok {
			return
		}

		var out Reservation

		switch target {
		case ReservationConfirmed:
			out, err =
				h.service.ConfirmReservation(
					c,
					actor,
					scope,
					reservationID,
				)

		case ReservationCommitted:
			out, err =
				h.service.CommitReservation(
					c,
					actor,
					scope,
					reservationID,
				)

		case ReservationReleased:
			out, err =
				h.service.ReleaseReservation(
					c,
					actor,
					scope,
					reservationID,
				)

		case ReservationCancelled:
			out, err =
				h.service.CancelReservation(
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

		inventoryRespond(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *Handler) sellability(
	c *gin.Context,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	itemID :=
		c.Param(
			"inventoryItemId",
		)

	branchID, err :=
		h.service.ItemBranch(
			c,
			actor.BusinessID,
			itemID,
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

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			branchID,
			PermissionInventoryRead,
		)

	if !ok {
		return
	}

	quantity, err :=
		strconv.ParseInt(
			c.DefaultQuery(
				"quantity",
				"1",
			),
			10,
			64,
		)

	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"quantity must be an integer.",
				err,
			),
		)

		return
	}

	out, err :=
		h.service.AuthoritativeSellability(
			c,
			actor,
			scope,
			itemID,
			quantity,
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

	inventoryRespond(
		c,
		http.StatusOK,
		out,
	)
}

func RegisterReservationRoutes(
	root *gin.RouterGroup,
	h *Handler,
	flags Flags,
) {
	if !flags.Enabled ||
		h == nil {
		return
	}

	reservations :=
		root.Group(
			"/inventory/reservations",
		)

	reservations.POST(
		"",
		h.createReservation,
	)

	reservations.GET(
		"/:reservationId",
		h.getReservation,
	)

	reservations.POST(
		"/:reservationId/confirm",
		h.reservationTransition(
			ReservationConfirmed,
		),
	)

	reservations.POST(
		"/:reservationId/commit",
		h.reservationTransition(
			ReservationCommitted,
		),
	)

	reservations.POST(
		"/:reservationId/release",
		h.reservationTransition(
			ReservationReleased,
		),
	)

	reservations.POST(
		"/:reservationId/cancel",
		h.reservationTransition(
			ReservationCancelled,
		),
	)

	root.GET(
		"/inventory/items/:inventoryItemId/sellability",
		h.sellability,
	)
}
