package commercemission

import (
	"context"
	"errors"
	"net/http"
	"time"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProviderEvidenceView struct {
	ID              string    `json:"evidence_id"`
	Type            string    `json:"evidence_type"`
	ParcelRefs      []string  `json:"parcel_refs"`
	CapturedAt      time.Time `json:"captured_at"`
	AttachmentCount int       `json:"attachment_count"`
	Description     string    `json:"description"`
}
type ProviderExceptionOption struct {
	Reason          ExceptionReason  `json:"reason_code"`
	RecoveryActions []RecoveryAction `json:"recovery_actions"`
	PhotoRequired   bool             `json:"photo_required"`
}
type ProviderExceptionView struct {
	ID         string    `json:"exception_id"`
	Reason     string    `json:"reason_code"`
	Recovery   string    `json:"recovery_action"`
	State      string    `json:"state"`
	ParcelRefs []string  `json:"parcel_refs"`
	CreatedAt  time.Time `json:"created_at"`
}

func providerExceptionOptions(state string, authorized bool) []ProviderExceptionOption {
	options := []ProviderExceptionOption{}
	if !authorized || state == "COMPLETED" || state == "CANCELLED" || state == "RETURNED" {
		return options
	}
	for _, reason := range []ExceptionReason{ReasonVendorNotReady, ReasonPackageMismatch, ReasonProhibitedItem, ReasonRestrictedItem, ReasonDamagedPackage, ReasonReceiverUnavailable, ReasonReceiverRefused, ReasonWrongAddress, ReasonAddressInaccessible, ReasonVehicleBreakdown, ReasonProviderIllness, ReasonWeatherRoad, ReasonSecurityRisk, ReasonSafetyIncident} {
		if reason == ReasonVendorNotReady && state != "ASSIGNED" && state != "PICKING_UP" {
			continue
		}
		option := ProviderExceptionOption{Reason: reason, PhotoRequired: reason == ReasonDamagedPackage || reason == ReasonPackageMismatch, RecoveryActions: []RecoveryAction{}}
		for _, action := range []RecoveryAction{RecoveryWait, RecoveryRetryPickup, RecoveryRetryDelivery, RecoveryReschedule, RecoveryReplan, RecoveryReturnSender, RecoveryReturnVendor, RecoveryProviderReplacement, RecoverySupport} {
			if RecoveryAllowed(reason, action, false) {
				option.RecoveryActions = append(option.RecoveryActions, action)
			}
		}
		options = append(options, option)
	}
	return options
}

type ProviderReturnView struct {
	ID           string   `json:"return_id"`
	ProductClass string   `json:"product_class"`
	Disposition  string   `json:"disposition"`
	State        string   `json:"state"`
	ParcelRefs   []string `json:"parcel_refs"`
}
type ProviderTransferView struct {
	Ref          string    `json:"transfer_ref"`
	State        string    `json:"state"`
	Role         string    `json:"role"`
	ParcelRefs   []string  `json:"parcel_refs"`
	PackageCount int64     `json:"package_count"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type ProviderMissionView struct {
	Evidence           []ProviderEvidenceView    `json:"evidence"`
	ExceptionOptions   []ProviderExceptionOption `json:"exception_options"`
	Exceptions         []ProviderExceptionView   `json:"exceptions"`
	TransferParcelRefs []string                  `json:"transfer_parcel_refs"`
	ReturnParcelRefs   []string                  `json:"return_parcel_refs"`
	Returns            []ProviderReturnView      `json:"returns"`
	Transfer           *ProviderTransferView     `json:"transfer,omitempty"`
	CanStartTransfer   bool                      `json:"can_start_transfer"`

	PickupActionsAllowed bool                          `json:"pickup_actions_allowed"`
	MissionRef           string                        `json:"mission_ref"`
	TripRef              string                        `json:"trip_ref"`
	State                string                        `json:"state"`
	Version              int64                         `json:"version"`
	Stops                []dispatchcommerce.StopView   `json:"stops"`
	Parcels              []dispatchcommerce.ParcelView `json:"parcels"`
}

func (r *Repository) ProviderMissionForTrip(ctx context.Context, providerID, tripID string) (ProviderMissionView, error) {
	if _, err := uuid.Parse(tripID); err != nil {
		return ProviderMissionView{}, apperrors.BadRequest("Delivery reference is invalid.", nil)
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return ProviderMissionView{}, err
	}
	defer tx.Rollback(ctx)
	record, err := scanMultiStopMission(tx.QueryRow(ctx, `SELECT `+multiStopMissionColumns+` FROM commerce_missions
 WHERE ((trip_id=$1::uuid AND current_provider_id=$2::uuid) OR (replacement_trip_id=$1::uuid AND replacement_provider_id=$2::uuid)) AND plan_version>0 FOR SHARE`, tripID, providerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderMissionView{}, apperrors.NotFound("Assigned Commerce mission not found.", nil)
	}
	if err != nil {
		return ProviderMissionView{}, err
	}
	var replacement, currentProvider, replacementProvider, replacementTrip string
	if err = tx.QueryRow(ctx, `SELECT replacement_state,COALESCE(current_provider_id::text,''),COALESCE(replacement_provider_id::text,''),COALESCE(replacement_trip_id::text,'') FROM commerce_missions WHERE id=$1::uuid`, record.ID).Scan(&replacement, &currentProvider, &replacementProvider, &replacementTrip); err != nil {
		return ProviderMissionView{}, err
	}
	view, err := loadMissionViewWith(ctx, tx, record)
	if err != nil {
		return ProviderMissionView{}, err
	}
	result := ProviderMissionView{MissionRef: view.MissionRef, TripRef: tripID, State: view.State, Version: view.Version, Stops: view.Stops, Parcels: view.Parcels, Returns: []ProviderReturnView{}, ReturnParcelRefs: []string{}}
	active := view.State != "COMPLETED" && view.State != "CANCELLED" && view.State != "RETURNED"
	result.PickupActionsAllowed = providerID == currentProvider && replacement == "NONE" && active && view.State != "PICKUP_BLOCKED"
	for _, parcel := range view.Parcels {
		if parcel.CustodianType == "PROVIDER" && parcel.CustodianRef == providerID && (parcel.Status == "PICKED_UP" || parcel.Status == "IN_TRANSIT" || parcel.Status == "RETURN_REQUIRED") {
			result.ReturnParcelRefs = append(result.ReturnParcelRefs, parcel.ParcelRef)
		}
	}
	rows, err := tx.Query(ctx, `SELECT id::text,product_class,disposition,state,parcel_refs FROM commerce_return_operations WHERE commerce_mission_id=$1::uuid ORDER BY created_at`, record.ID)
	if err != nil {
		return ProviderMissionView{}, err
	}
	for rows.Next() {
		var item ProviderReturnView
		if err = rows.Scan(&item.ID, &item.ProductClass, &item.Disposition, &item.State, &item.ParcelRefs); err != nil {
			rows.Close()
			return ProviderMissionView{}, err
		}
		result.Returns = append(result.Returns, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ProviderMissionView{}, err
	}
	rows.Close()
	result.TransferParcelRefs = append([]string{}, result.ReturnParcelRefs...)
	result.CanStartTransfer = providerID == currentProvider && replacementProvider != "" && replacementTrip != "" && (replacement == "ASSIGNED" || replacement == "TRANSFER_PENDING") && len(result.ReturnParcelRefs) > 0 && active
	var transfer ProviderTransferView
	var outgoing, incoming string
	err = tx.QueryRow(ctx, `SELECT transfer_ref,state,old_provider_id::text,new_provider_id::text,parcel_refs,package_count,challenge_expires_at FROM commerce_provider_transfers WHERE commerce_mission_id=$1::uuid AND (old_provider_id=$2::uuid OR new_provider_id=$2::uuid) ORDER BY created_at DESC LIMIT 1`, record.ID, providerID).Scan(&transfer.Ref, &transfer.State, &outgoing, &incoming, &transfer.ParcelRefs, &transfer.PackageCount, &transfer.ExpiresAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ProviderMissionView{}, err
	}
	if err == nil {
		transfer.Role = "INCOMING"
		if outgoing == providerID {
			transfer.Role = "OUTGOING"
		}
		result.Transfer = &transfer
	}
	if !active || providerID != currentProvider || replacement != "NONE" {
		result.ReturnParcelRefs = []string{}
	}
	result.ExceptionOptions = providerExceptionOptions(view.State, providerID == currentProvider)
	result.Exceptions = []ProviderExceptionView{}
	exceptionRows, err := tx.Query(ctx, `SELECT id::text,reason_code,recovery_action,state,parcel_refs,created_at FROM commerce_delivery_exceptions WHERE commerce_mission_id=$1::uuid ORDER BY created_at DESC LIMIT 100`, record.ID)
	if err != nil {
		return ProviderMissionView{}, err
	}
	for exceptionRows.Next() {
		var item ProviderExceptionView
		if err = exceptionRows.Scan(&item.ID, &item.Reason, &item.Recovery, &item.State, &item.ParcelRefs, &item.CreatedAt); err != nil {
			exceptionRows.Close()
			return ProviderMissionView{}, err
		}
		result.Exceptions = append(result.Exceptions, item)
	}
	if err = exceptionRows.Err(); err != nil {
		exceptionRows.Close()
		return ProviderMissionView{}, err
	}
	exceptionRows.Close()
	result.Evidence = []ProviderEvidenceView{}
	evidenceRows, err := tx.Query(ctx, `SELECT id::text,evidence_type,parcel_refs,captured_at,cardinality(media_refs),description FROM commerce_mission_evidence WHERE commerce_mission_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 100`, record.ID)
	if err != nil {
		return ProviderMissionView{}, err
	}
	for evidenceRows.Next() {
		var item ProviderEvidenceView
		if err = evidenceRows.Scan(&item.ID, &item.Type, &item.ParcelRefs, &item.CapturedAt, &item.AttachmentCount, &item.Description); err != nil {
			evidenceRows.Close()
			return ProviderMissionView{}, err
		}
		result.Evidence = append(result.Evidence, item)
	}
	if err = evidenceRows.Err(); err != nil {
		evidenceRows.Close()
		return ProviderMissionView{}, err
	}
	evidenceRows.Close()
	if err = tx.Commit(ctx); err != nil {
		return ProviderMissionView{}, err
	}
	return result, nil
}

func (h *MultiStopMissionHandler) readProviderTrip(c *gin.Context) {
	store, ok := h.service.store.(interface {
		ProviderMissionForTrip(context.Context, string, string) (ProviderMissionView, error)
	})
	if !ok {
		httpx.Abort(c, apperrors.Unavailable("Commerce delivery details are unavailable.", nil))
		return
	}
	view, err := store.ProviderMissionForTrip(c.Request.Context(), authhttp.DispatchRiderID(c), c.Param("tripId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[ProviderMissionView]{Success: true, Data: view})
}
