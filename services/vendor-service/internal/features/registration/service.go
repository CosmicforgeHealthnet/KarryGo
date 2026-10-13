package registration

import (
	"context"
	auth "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type Service struct {
	db      *pgxpool.Pool
	enabled bool
}

func NewService(db *pgxpool.Pool, enabled bool) *Service { return &Service{db: db, enabled: enabled} }

func (s *Service) RegisterOwner(ctx context.Context, userID string, in auth.RegistrationInput) error {
	if !s.enabled {
		return apperrors.Forbidden("Self-registration is unavailable on this server.", nil)
	}
	business := vendors.Business{LegalName: strings.TrimSpace(in.LegalName), Currency: strings.ToUpper(strings.TrimSpace(in.Currency)), Timezone: strings.TrimSpace(in.Timezone)}
	createBusiness := business.LegalName != "" || business.Currency != "" || business.Timezone != ""
	if createBusiness {
		if err := vendors.ValidateBusiness(business); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM vendor_users WHERE id=$1 FOR UPDATE`, userID).Scan(&locked); err != nil {
		return err
	}
	var existing bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff WHERE user_id=$1)`, userID).Scan(&existing); err != nil {
		return err
	}
	if existing {
		return tx.Commit(ctx)
	}
	business.ID = "vb_" + uuid.NewString()
	business.OwnerRef = "vs_" + uuid.NewString()
	business.Status = vendors.BusinessDraft
	business.Version = 1
	if _, err = tx.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`, business.OwnerRef, business.ID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id) VALUES($1,$2,'role_owner')`, "vsa_"+uuid.NewString(), business.OwnerRef); err != nil {
		return err
	}
	if createBusiness {
		if _, err = tx.Exec(ctx, `INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone) VALUES($1,$2,'DRAFT',$3,$4,$5)`, business.ID, business.LegalName, business.OwnerRef, business.Currency, business.Timezone); err != nil {
			return err
		}
	}

	staff := map[string]any{"id": business.OwnerRef, "vendor_id": business.ID, "user_id": userID, "status": "active", "version": 1}
	payload, err := json.Marshal(staff)
	if err != nil {
		return err
	}
	correlation := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO vendor_outbox(id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,vendor_id,correlation_id,payload) VALUES($1,'vendor.staff.created',1,'vendor_staff',$2,1,$3,$4,$5::jsonb)`, "ve_"+uuid.NewString(), business.OwnerRef, business.ID, correlation, string(payload)); err != nil {
		return err
	}
	if createBusiness {
		businessPayload, marshalErr := json.Marshal(business)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO vendor_outbox(id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,vendor_id,correlation_id,payload) VALUES($1,'vendor.business.created',1,'vendor_business',$2,1,$2,$3,$4::jsonb)`, "ve_"+uuid.NewString(), business.ID, correlation, string(businessPayload)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO vendor_audit_log(id,vendor_id,actor_type,actor_id,action,aggregate_type,aggregate_id,after_data) VALUES($1,$2,'vendor_user',$3,'vendor.registration.completed','vendor_staff',$4,$5::jsonb)`, "va_"+uuid.NewString(), business.ID, userID, business.OwnerRef, string(payload)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
