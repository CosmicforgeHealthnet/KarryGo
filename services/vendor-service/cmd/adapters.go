package main

import (
	"context"
	"errors"
	"net/http"

	"cosmicforge/logistics/shared/go/apperrors"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/events"

	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	crm "cosmicforge/logistics/services/vendor-service/internal/features/crm"
	food "cosmicforge/logistics/services/vendor-service/internal/features/food"
	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	production "cosmicforge/logistics/services/vendor-service/internal/features/production"
	purchasing "cosmicforge/logistics/services/vendor-service/internal/features/purchasing"
	shop "cosmicforge/logistics/services/vendor-service/internal/features/shop"
	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	vendors "cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	mediaintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/media"
	verificationintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/verification"
	walletintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/wallet"
)

type productionScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a productionScopeAdapter) ResolveScope(ctx context.Context, staffID, permission string) (production.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return production.Scope{}, err
	}
	var scope production.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			return production.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

type productionAuditAdapter struct{ recorder audit.Recorder }

func (a productionAuditAdapter) Record(ctx context.Context, entry production.AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{ActorType: "vendor_staff", ActorID: entry.ActorID, Action: entry.Action, AggregateType: entry.AggregateType, AggregateID: entry.AggregateID, VendorID: entry.VendorID, BranchID: entry.BranchID, Before: entry.Before, After: entry.After, Reason: entry.Reason})
}

// This file is the composition root's set of small adapters wiring one
// feature's port interface onto another feature's real repositories/services
// ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â the pattern docs/architecture/03_VENDOR_SERVICE_MODULE_MAP.md documents
// (mirrors fleet-partner-service's CRMPort/CatalogPort adapters). Each
// adapter is a few lines of type conversion; none contains business logic.

// userResolverAdapter implements staffusecases.UserResolver over auth's real
// UserRepository ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â lets StaffService create/find login identities without
// staff/usecases importing internal/features/auth/repositories directly.
type userResolverAdapter struct {
	users *vendorauthrepositories.UserRepository
}

type vendorAuthorizationAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a vendorAuthorizationAdapter) Authorize(ctx context.Context, staffID, permission string, branchID *string) (bool, error) {
	decision, err := a.evaluator.Authorize(ctx, staffID, permission, staffusecases.Target{BranchID: branchID})
	return decision.Allowed, err
}

type vendorAuditAdapter struct {
	recorder audit.Recorder
}

type verificationAuthorityAdapter struct {
	client *verificationintegration.Client
}

func (a verificationAuthorityAdapter) CreateCase(ctx context.Context, in vendors.VerificationCaseInput) (vendors.VerificationCaseResult, error) {
	r, e := a.client.Create(ctx, verificationintegration.CreateInput{ClientReference: in.ClientReference, BusinessID: in.BusinessID, Dimension: string(in.Dimension), SubjectType: in.SubjectType, SubjectRef: in.SubjectRef, DocumentRefs: in.DocumentRefs})
	return vendors.VerificationCaseResult{SourceRef: r.SourceRef, SourceVersion: r.SourceVersion, Status: vendors.VerificationState(r.Status), ObservedAt: r.ObservedAt, ExpiresAt: r.ExpiresAt, ReasonCode: r.ReasonCode, Remediation: r.Remediation}, e
}
func (a verificationAuthorityAdapter) GetCaseByClientReference(ctx context.Context, reference string) (vendors.VerificationCaseResult, error) {
	r, e := a.client.GetByClientReference(ctx, reference)
	return vendors.VerificationCaseResult{SourceRef: r.SourceRef, SourceVersion: r.SourceVersion, Status: vendors.VerificationState(r.Status), ObservedAt: r.ObservedAt, ExpiresAt: r.ExpiresAt, ReasonCode: r.ReasonCode, Remediation: r.Remediation}, e
}

type bankAuthorityAdapter struct{ client *walletintegration.Client }

func (a bankAuthorityAdapter) ResolveAndRegister(ctx context.Context, businessID, accountNumber, bankCode, key string) (vendors.BankAuthorityResult, error) {
	r, e := a.client.ResolveAndRegister(ctx, businessID, accountNumber, bankCode, key)
	e = walletBankError(e)
	return vendors.BankAuthorityResult{ExternalRef: r.ExternalRef, BankCode: r.BankCode, MaskedAccount: r.MaskedAccount, ResolvedAccountName: r.ResolvedAccountName, SourceVersion: r.SourceVersion, Status: vendors.VerificationState(r.Status), ObservedAt: r.ObservedAt}, e
}
func (a bankAuthorityAdapter) GetProfile(ctx context.Context, businessID string) (vendors.BankAuthorityResult, error) {
	r, e := a.client.Get(ctx, businessID)
	e = walletBankError(e)
	return vendors.BankAuthorityResult{ExternalRef: r.ExternalRef, BankCode: r.BankCode, MaskedAccount: r.MaskedAccount, ResolvedAccountName: r.ResolvedAccountName, SourceVersion: r.SourceVersion, Status: vendors.VerificationState(r.Status), ObservedAt: r.ObservedAt}, e
}

// walletBankError turns a definitive wallet refusal into an API-facing error so
// a mistyped account reads as "check your details" instead of an unknown
// outcome. Transport failures and 5xx stay ambiguous and untouched.
func walletBankError(err error) error {
	var authErr *walletintegration.AuthorityError
	if !errors.As(err, &authErr) || !authErr.Refused() {
		return err
	}
	switch authErr.Status {
	case http.StatusNotFound:
		return apperrors.NotFound("Bank profile not found.", err)
	case http.StatusUnauthorized, http.StatusForbidden:
		return apperrors.Unavailable("Bank verification is temporarily unavailable. Please try again later.", err)
	}
	message := authErr.Message
	if message == "" {
		message = "We could not confirm this bank account. Check the bank and account number and try again."
	}
	return apperrors.New(http.StatusUnprocessableEntity, apperrors.CodeValidationFailed, message, err)
}

type mediaAuthorityAdapter struct{ client *mediaintegration.Client }

func (a mediaAuthorityAdapter) GetObject(ctx context.Context, businessID, ref string) (vendors.MediaObject, error) {
	o, err := a.client.GetObject(ctx, businessID, ref)
	return vendors.MediaObject{ID: o.ID, OwnerReference: o.OwnerReference, UploadPurpose: o.UploadPurpose, UploadStatus: o.UploadStatus, SafetyStatus: o.SafetyStatus, DeletionState: o.DeletionState, SourceVersion: o.SourceVersion}, err
}

func (a vendorAuditAdapter) Record(ctx context.Context, entry vendors.AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{ActorType: "vendor_staff", ActorID: entry.ActorID, Action: entry.Action, AggregateType: entry.AggregateType, AggregateID: entry.AggregateID, VendorID: entry.VendorID, BranchID: entry.BranchID, Before: entry.Before, After: entry.After, Reason: entry.Reason})
}

func (a userResolverAdapter) ResolveOrCreateByPhone(ctx context.Context, phone string) (staffusecases.UserRef, error) {
	u, err := a.users.UpsertByPhone(ctx, phone)
	if err != nil {
		return staffusecases.UserRef{}, err
	}
	return staffusecases.UserRef{ID: u.ID}, nil
}

func (a userResolverAdapter) ResolveOrCreateByEmail(ctx context.Context, email string) (staffusecases.UserRef, error) {
	u, err := a.users.UpsertByEmail(ctx, email)
	if err != nil {
		return staffusecases.UserRef{}, err
	}
	return staffusecases.UserRef{ID: u.ID}, nil
}

// staffResolverAdapter implements vendorauthusecases.StaffResolver over
// staff's real StaffRepository ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â lets AuthService check staff status/
// affiliation without auth/usecases importing
// internal/features/staff/repositories directly.
type staffResolverAdapter struct {
	staff *staffrepositories.StaffRepository
}

func toAffiliation(s staffmodels.Staff) vendorauthusecases.StaffAffiliation {
	return vendorauthusecases.StaffAffiliation{StaffID: s.ID, VendorID: s.VendorID, UserID: s.UserID, Active: s.IsActive()}
}

func (a staffResolverAdapter) ListActiveByUser(ctx context.Context, userID string) ([]vendorauthusecases.StaffAffiliation, error) {
	list, err := a.staff.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]vendorauthusecases.StaffAffiliation, len(list))
	for i, s := range list {
		out[i] = toAffiliation(s)
	}
	return out, nil
}

func (a staffResolverAdapter) GetByVendorAndUser(ctx context.Context, vendorID, userID string) (vendorauthusecases.StaffAffiliation, error) {
	s, err := a.staff.GetByVendorAndUser(ctx, vendorID, userID)
	if err != nil {
		return vendorauthusecases.StaffAffiliation{}, err
	}
	return toAffiliation(s), nil
}

func (a staffResolverAdapter) GetByID(ctx context.Context, staffID string) (vendorauthusecases.StaffAffiliation, error) {
	s, err := a.staff.GetByID(ctx, staffID)
	if err != nil {
		return vendorauthusecases.StaffAffiliation{}, err
	}
	return toAffiliation(s), nil
}

// auditAdapter implements staffusecases.AuditRecorder over the real
// audit.PostgresRecorder.
type auditAdapter struct {
	recorder audit.Recorder
}

func (a auditAdapter) Record(ctx context.Context, entry staffusecases.AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{
		ActorType: entry.ActorType, ActorID: entry.ActorID, Action: entry.Action,
		AggregateType: entry.AggregateType, AggregateID: entry.AggregateID,
		VendorID: entry.VendorID, BranchID: entry.BranchID, Before: entry.Before, After: entry.After, Reason: entry.Reason,
	})
}

// staffEventAdapter implements staffusecases.EventPublisher over the real
// transactional outbox. No consumer exists yet for these event types (see
// docs/architecture/08_EVENT_OWNERSHIP.md) ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â events are appended durably and
// will simply accumulate as PENDING until a real Publisher/destination is
// configured for a future section, never silently dropped.
type staffEventAdapter struct {
	outbox *events.OutboxRepository
	db     *pgxpool.Pool
}

func (a staffEventAdapter) PublishStaffEvent(ctx context.Context, eventType, vendorID, staffID string, payload any) error {
	envelope, err := events.NewEnvelope(eventType, 1, "vendor_staff", staffID, 0, vendorID, "", "", payload, vendorID == "")
	if err != nil {
		return err
	}
	return a.outbox.Append(ctx, a.db, envelope)
}

// crmScopeAdapter implements crm.ScopeResolver over the Section 5
// authorization evaluator.
//
// It answers a slightly different question from the plain Authorize check: CRM
// needs to know *at which scope* a permission is held, because a vendor-wide
// grant may see business totals while a branch-scoped grant may only ever see
// its own branches' slice of a relationship.
type crmScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a crmScopeAdapter) ResolveScope(ctx context.Context, staffID, permission string) (crm.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return crm.Scope{}, err
	}
	var scope crm.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			// A vendor-wide grant subsumes every branch-scoped one.
			return crm.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

// foodScopeAdapter implements food.ScopeResolver over the Section 5
// authorization evaluator.
//
// Section 9A asks the same question CRM does ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â at which scope is this
// permission held ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â because a menu belongs to a branch and a branch-scoped
// grant must never reach another branch's catalogue.
type foodScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a foodScopeAdapter) ResolveScope(ctx context.Context, staffID, permission string) (food.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return food.Scope{}, err
	}
	var scope food.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			// A vendor-wide grant subsumes every branch-scoped one.
			return food.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

// foodAuditAdapter implements food.AuditRecorder over the real recorder.
type foodAuditAdapter struct {
	recorder audit.Recorder
}

func (a foodAuditAdapter) Record(ctx context.Context, entry food.AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{
		ActorType: "vendor_staff", ActorID: entry.ActorID, Action: entry.Action,
		AggregateType: entry.AggregateType, AggregateID: entry.AggregateID,
		VendorID: entry.VendorID, BranchID: entry.BranchID,
		Before: entry.Before, After: entry.After, Reason: entry.Reason,
	})
}

// foodMediaAdapter reuses the existing Media private-API client. Section 9A
// adds no new media contract: it asks the same ownership question Section 7
// already asks for onboarding documents.
type foodMediaAdapter struct{ client *mediaintegration.Client }

func (a foodMediaAdapter) GetObject(ctx context.Context, businessID, ref string) (food.MediaObject, error) {
	o, err := a.client.GetObject(ctx, businessID, ref)
	return food.MediaObject{BindingType: o.BindingType, BindingID: o.BindingID, Visibility: o.Visibility, OwnerService: o.OwnerService, ID: o.ID, OwnerReference: o.OwnerReference, UploadPurpose: o.UploadPurpose,
		UploadStatus: o.UploadStatus, SafetyStatus: o.SafetyStatus, DeletionState: o.DeletionState,
		SourceVersion: o.SourceVersion}, err
}

type shopScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a shopScopeAdapter) ResolveScope(ctx context.Context, staffID, permission string) (shop.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return shop.Scope{}, err
	}
	var scope shop.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			return shop.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

type shopAuditAdapter struct{ recorder audit.Recorder }

func (a shopAuditAdapter) Record(ctx context.Context, entry shop.AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{
		ActorType: "vendor_staff", ActorID: entry.ActorID, Action: entry.Action,
		AggregateType: entry.AggregateType, AggregateID: entry.AggregateID,
		VendorID: entry.VendorID, BranchID: entry.BranchID,
		Before: entry.Before, After: entry.After, Reason: entry.Reason,
	})
}

type shopMediaAdapter struct{ client *mediaintegration.Client }

func (a shopMediaAdapter) GetObject(ctx context.Context, businessID, ref string) (shop.MediaObject, error) {
	o, err := a.client.GetObject(ctx, businessID, ref)
	return shop.MediaObject{BindingType: o.BindingType, BindingID: o.BindingID, Visibility: o.Visibility, OwnerService: o.OwnerService, ID: o.ID, OwnerReference: o.OwnerReference, UploadPurpose: o.UploadPurpose,
		UploadStatus: o.UploadStatus, SafetyStatus: o.SafetyStatus, DeletionState: o.DeletionState,
		SourceVersion: o.SourceVersion}, err
}

// crmAuditAdapter implements crm.AuditRecorder over the real recorder.
type crmAuditAdapter struct {
	recorder audit.Recorder
}

func (a crmAuditAdapter) Record(ctx context.Context, entry crm.AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{
		ActorType: "vendor_staff", ActorID: entry.ActorID, Action: entry.Action,
		AggregateType: entry.AggregateType, AggregateID: entry.AggregateID,
		VendorID: entry.VendorID, BranchID: entry.BranchID,
		Before: entry.Before, After: entry.After, Reason: entry.Reason,
	})
}

type inventoryScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a inventoryScopeAdapter) ResolveScope(
	ctx context.Context,
	staffID string,
	permission string,
) (inventory.AccessScope, error) {
	grants, err :=
		a.evaluator.EffectivePermissions(
			ctx,
			staffID,
		)

	if err != nil {
		return inventory.AccessScope{},
			err
	}

	var scope inventory.AccessScope

	for _, grant := range grants {

		if grant.PermissionCode !=
			permission {
			continue
		}

		if grant.BranchID == nil {
			return inventory.AccessScope{
				Global: true,
			}, nil
		}

		scope.BranchIDs =
			append(
				scope.BranchIDs,
				*grant.BranchID,
			)
	}

	return scope, nil
}

type inventoryEventAdapter struct {
	outbox *events.OutboxRepository
}

func (a inventoryEventAdapter) PublishInventoryEvent(
	ctx context.Context,
	tx inventory.InventoryTransaction,
	event inventory.Event,
) error {
	postgresTx, ok :=
		tx.(inventory.PostgresTransactionAccessor)

	if !ok {
		return inventory.ErrDependencyMissing
	}

	envelope, err :=
		events.NewEnvelope(
			event.Type,
			1,
			event.AggregateType,
			event.AggregateRef,
			event.Revision,
			event.BusinessID,
			event.CorrelationID,
			event.CausationID,
			event.Payload,
			false,
		)

	if err != nil {
		return err
	}

	return a.outbox.Append(
		ctx,
		postgresTx.PGX(),
		envelope,
	)
}

type foodInventoryAvailabilityAdapter struct {
	inventory *inventory.StockService
}

func (
	a foodInventoryAvailabilityAdapter,
) ResolveFoodAvailability(
	ctx context.Context,
	request food.InventoryAvailabilityRequest,
) (
	food.InventoryAvailabilityDecision,
	error,
) {
	if a.inventory == nil {
		return food.InventoryAvailabilityDecision{
			InventoryControlled: true,
			Support:             food.InventorySupportNotSupported,
			Sellable:            false,
			Reason:              "INVENTORY_AUTHORITY_NOT_CONNECTED",
		}, nil
	}

	result, err :=
		a.inventory.ResolveFoodAvailability(
			ctx,
			inventory.FoodAvailabilityRequest{
				BusinessID: request.BusinessID,

				BranchID: request.BranchID,

				FoodItemRef: request.FoodItemRef,

				Mode: request.Mode,

				InventoryRecipeRef: request.InventoryRecipeRef,
			},
		)

	if err != nil {
		return food.InventoryAvailabilityDecision{},
			err
	}

	return food.InventoryAvailabilityDecision{
		InventoryControlled: result.InventoryControlled,

		Support: result.Support,

		Sellable: result.Sellable,

		Reason: result.Reason,

		InventoryRevision: result.InventoryRevision,
	}, nil
}

type purchasingScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a purchasingScopeAdapter) ResolveScope(
	ctx context.Context,
	staffID string,
	permission string,
) (purchasing.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(
		ctx,
		staffID,
	)
	if err != nil {
		return purchasing.Scope{}, err
	}

	var scope purchasing.Scope

	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}

		if grant.BranchID == nil {
			return purchasing.Scope{
				Global: true,
			}, nil
		}

		scope.BranchIDs = append(
			scope.BranchIDs,
			*grant.BranchID,
		)
	}

	return scope, nil
}

func (a mediaAuthorityAdapter) AuthorizeOnboardingUpload(ctx context.Context, businessID string, in vendors.OnboardingUploadInput, key string) (vendors.OnboardingUploadGrant, error) {
	grant, err := a.client.AuthorizeOnboardingUpload(ctx, businessID, string(in.Dimension), in.MIMEType, in.ByteSize, key)
	return vendors.OnboardingUploadGrant{SessionID: grant.SessionID, MediaRef: grant.MediaRef,
		URL: grant.URL, Token: grant.Token, ExpiresAt: grant.ExpiresAt,
		MaxBytes: grant.MaxBytes, AllowedMIMETypes: grant.AllowedMIMETypes}, err
}
