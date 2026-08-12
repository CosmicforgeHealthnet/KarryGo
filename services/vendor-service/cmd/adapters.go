package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/events"

	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	crm "cosmicforge/logistics/services/vendor-service/internal/features/crm"
	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	vendors "cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	mediaintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/media"
	verificationintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/verification"
	walletintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/wallet"
)

// This file is the composition root's set of small adapters wiring one
// feature's port interface onto another feature's real repositories/services
// — the pattern docs/architecture/03_VENDOR_SERVICE_MODULE_MAP.md documents
// (mirrors fleet-partner-service's CRMPort/CatalogPort adapters). Each
// adapter is a few lines of type conversion; none contains business logic.

// userResolverAdapter implements staffusecases.UserResolver over auth's real
// UserRepository — lets StaffService create/find login identities without
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
	return vendors.BankAuthorityResult{ExternalRef: r.ExternalRef, BankCode: r.BankCode, MaskedAccount: r.MaskedAccount, ResolvedAccountName: r.ResolvedAccountName, SourceVersion: r.SourceVersion, Status: vendors.VerificationState(r.Status), ObservedAt: r.ObservedAt}, e
}
func (a bankAuthorityAdapter) GetProfile(ctx context.Context, businessID string) (vendors.BankAuthorityResult, error) {
	r, e := a.client.Get(ctx, businessID)
	return vendors.BankAuthorityResult{ExternalRef: r.ExternalRef, BankCode: r.BankCode, MaskedAccount: r.MaskedAccount, ResolvedAccountName: r.ResolvedAccountName, SourceVersion: r.SourceVersion, Status: vendors.VerificationState(r.Status), ObservedAt: r.ObservedAt}, e
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
// staff's real StaffRepository — lets AuthService check staff status/
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
// docs/architecture/08_EVENT_OWNERSHIP.md) — events are appended durably and
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
