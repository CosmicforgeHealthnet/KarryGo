package usecases

import (
	"context"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"time"
)

// OwnerCaseAuthority combines current native case/subject proof with an actual
// modern Media ownership and case binding. Legacy assets are never queried.
type OwnerCaseAuthority struct {
	Media   *Service
	Policy  *adminpolicy.Engine
	Sources map[string]*admincase.Client
}

func (a *OwnerCaseAuthority) AuthorizeCase(ctx context.Context, actor CaseActor, q CaseRequest) (CaseProof, error) {
	var empty CaseProof
	spec, ok := admincase.Find(q.CaseOwner, q.CaseKind)
	if a == nil || a.Media == nil || a.Media.opts.Pool == nil || a.Policy == nil || !ok || q.SourceCapability != spec.Permission || !q.Valid() {
		return empty, adminpolicy.ErrDenied
	}
	now := a.Media.opts.Now().UTC()
	if !actor.ExpiresAt.After(now) {
		return empty, adminpolicy.ErrDenied
	}
	if err := a.Policy.AuthorizeDelegation(actor.Delegation, spec.Permission, q.Purpose, q.Scope, now); err != nil {
		return empty, err
	}
	client := a.Sources[q.CaseOwner]
	if client == nil {
		return empty, adminpolicy.ErrBlocked
	}
	rule, err := a.Policy.Rule(spec.Permission, now)
	if err != nil || rule.Owner != q.CaseOwner || rule.SourceContract != spec.SourceContract {
		return empty, adminpolicy.ErrBlocked
	}
	role := ""
	for _, candidate := range actor.Delegation.Roles {
		if admincase.Contains(rule.Roles, candidate) {
			role = candidate
			break
		}
	}
	if role == "" {
		return empty, adminpolicy.ErrDenied
	}
	query := adminread.Query{Scope: adminread.Scope{Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}, Staff: actor.Delegation.Staff, Role: role, Purpose: q.Purpose, PolicyID: a.Policy.ID(), PolicyVersion: a.Policy.Version(), PolicyChecksum: a.Policy.Checksum(), Start: now.Add(-time.Minute), End: now, Limit: 1}
	proof, err := client.Authorize(ctx, query, q.CaseKind, q.CaseReference, q.SubjectType, q.SubjectReference)
	if err != nil {
		return empty, err
	}
	// Only the modern table can establish the object identity. Its authenticated
	// owner and typed case binding form the authoritative modern association.
	object, err := a.Media.opts.Objects.GetObject(ctx, a.Media.opts.Pool, q.MediaObjectID)
	if err != nil {
		return empty, ErrNotFound
	}
	if object.OrganizationID != q.Scope.Organization || object.OwnerService != q.CaseOwner || object.OwnerType != q.SubjectType || object.OwnerReference != q.SubjectReference || !admincase.Contains(spec.Purposes, object.UploadPurpose) {
		return empty, ErrNotFound
	}
	object.SecondaryBindings, err = a.Media.opts.Objects.SecondaryBindings(ctx, a.Media.opts.Pool, object.ID)
	if err != nil || !object.BoundTo(spec.BindingType, q.CaseReference) {
		return empty, ErrNotFound
	}
	return CaseProof{Facts: adminpolicy.Facts{Owner: proof.Owner, Reference: proof.CaseReference, SourceContract: proof.SourceContract, Scope: proof.Scope}, CaseKind: proof.CaseKind, SubjectType: proof.SubjectType, SubjectReference: proof.SubjectReference, MediaObjectID: object.ID, ObjectOwnerService: object.OwnerService, ObjectOwnerType: object.OwnerType, ObjectOwnerReference: object.OwnerReference, ObjectPurpose: object.UploadPurpose, BindingType: spec.BindingType, BindingID: q.CaseReference, NativeFingerprint: proof.NativeFingerprint, ExpiresAt: proof.ExpiresAt}, nil
}
