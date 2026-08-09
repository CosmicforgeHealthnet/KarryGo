package usecases

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// The provider-facing and customer-facing document surfaces.
//
// Both are reached only through the HMAC service-relay pattern: a driver
// service or customer-service has already authenticated its own caller (a
// provider's own session, a customer's own session) and vouches for the
// identity asserted in the request. Fleet never trusts that assertion at
// face value — it independently resolves the identity to something it
// actually owns (a live membership, an order's customer) — and every
// access, allowed or denied, is written to the append-only access audit.
//
// Only documents at the matching visibility tier are ever reachable here:
// PROVIDER_VISIBLE for a provider, CUSTOMER_VISIBLE for a customer.
// INTERNAL, RESTRICTED and COMPANY_CONFIDENTIAL documents have no external
// route at all. There is no permission that could widen that, because a
// provider or customer caller holds no RBAC role in the first place — the
// visibility tier itself is the entire access decision.

// ExternalActor names who external access is being recorded against, for the
// audit trail. It is never used for authorization by itself — every method
// below independently resolves and re-checks ownership before granting
// anything.
type ExternalActor struct {
	SubjectType      string // models.AccessSubjectProvider or AccessSubjectCustomer
	SubjectReference string // membership id or customer id
	ActorReference   string // the caller-asserted raw identity, for the audit trail only
}

// recordAccess writes one access-audit entry, best-effort.
//
// A failure to write the audit row must not itself change whether access
// was granted, and must not surface to the external caller as if their
// request failed — the write path already committed (or refused) the read
// decision by the time this runs.
func (s *Service) recordAccess(ctx context.Context, organizationID string, documentID string,
	actor ExternalActor, visibility string, action string, allowed bool, denialReason string,
	correlationID string) {
	result := models.AccessResultDenied
	if allowed {
		result = models.AccessResultAllowed
	}
	_ = s.opts.Documents.RecordAccess(ctx, nil, &models.AccessAuditEntry{
		OrganizationID: organizationID, DocumentID: documentID,
		ActorType: actor.SubjectType, ActorReference: actor.ActorReference,
		SubjectType: actor.SubjectType, SubjectReference: actor.SubjectReference,
		Visibility: visibility, Action: action, Result: result,
		DenialReasonCode: denialReason, CorrelationID: correlationID,
	})
}

// --- provider-facing ------------------------------------------------------

// ResolveProviderMembership resolves an external provider identity to the
// live membership and organization it controls.
//
// Exposed so the HTTP layer can resolve once and reuse the result, without
// needing to know how membership resolution works internally.
func (s *Service) ResolveProviderMembership(ctx context.Context, providerService string,
	providerID string) (organizationID string, membershipID string, err error) {
	if s.opts.Memberships == nil {
		return "", "", errorcodes.Unavailable(errorcodes.FleetServiceUnavailable,
			"Provider document access is not enabled for this deployment.")
	}
	organizationID, membershipID, err = s.opts.Memberships.FindLiveMembership(ctx, providerService, providerID)
	if errors.Is(err, ErrMembershipNotFound) {
		return "", "", errorcodes.NotFound(errorcodes.DocumentNotFound, "Provider not found.")
	}
	if err != nil {
		return "", "", err
	}
	if organizationID == "" || membershipID == "" {
		return "", "", errorcodes.NotFound(errorcodes.DocumentNotFound, "Provider not found.")
	}
	return organizationID, membershipID, nil
}

func providerActor(providerService string, providerID string, membershipID string) ExternalActor {
	return ExternalActor{
		SubjectType: models.AccessSubjectProvider, SubjectReference: membershipID,
		ActorReference: providerService + ":" + providerID,
	}
}

// providerOwnsDocument reports whether a document is anchored to this
// provider's membership. Both PROVIDER_LINK and VEHICLE_LINK anchor to the
// same membership id.
func providerOwnsDocument(document *models.Document, membershipID string) bool {
	switch document.OwnerType {
	case models.OwnerProviderLink, models.OwnerVehicleLink:
		return document.OwnerID == membershipID
	default:
		return false
	}
}

// ListProviderDocuments returns the PROVIDER_VISIBLE documents owned by one
// provider's membership: a merge of PROVIDER_LINK and VEHICLE_LINK owned
// documents, since both anchor to the same membership.
func (s *Service) ListProviderDocuments(ctx context.Context, providerService string, providerID string,
	correlationID string) ([]models.Document, error) {
	organizationID, membershipID, err := s.ResolveProviderMembership(ctx, providerService, providerID)
	if err != nil {
		return nil, err
	}
	actor := providerActor(providerService, providerID, membershipID)

	query := tenant.Query{OrganizationID: organizationID, Limit: 200}
	documents := []models.Document{}
	for _, ownerType := range []string{models.OwnerProviderLink, models.OwnerVehicleLink} {
		page, listErr := s.opts.Documents.List(ctx, query, repositories.ListFilter{
			OwnerType: ownerType, OwnerID: membershipID, Visibility: models.VisibilityProviderVisible,
		})
		if listErr != nil {
			return nil, listErr
		}
		documents = append(documents, page...)
	}
	s.recordAccess(ctx, organizationID, "", actor, models.VisibilityProviderVisible,
		models.AccessActionList, true, "", correlationID)
	return documents, nil
}

// GetProviderDocument returns one PROVIDER_VISIBLE document owned by this
// provider's membership.
//
// A document that exists but belongs to someone else, or is not at the
// provider-visible tier, or is archived, answers exactly the same NotFound a
// nonexistent document would: an external caller must never be able to tell
// "this exists but is not yours" from "this does not exist".
func (s *Service) GetProviderDocument(ctx context.Context, providerService string, providerID string,
	documentID string, correlationID string) (*models.Document, error) {
	organizationID, membershipID, err := s.ResolveProviderMembership(ctx, providerService, providerID)
	if err != nil {
		return nil, err
	}
	actor := providerActor(providerService, providerID, membershipID)

	document, err := s.opts.Documents.GetByID(ctx, nil, organizationID, documentID)
	if errors.Is(err, repositories.ErrNotFound) {
		s.recordAccess(ctx, organizationID, documentID, actor, "", models.AccessActionRead,
			false, "not_found", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	if err != nil {
		return nil, err
	}
	if !providerOwnsDocument(document, membershipID) ||
		document.Visibility != models.VisibilityProviderVisible || document.Archived() {
		s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
			models.AccessActionRead, false, "not_visible", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
		models.AccessActionRead, true, "", correlationID)
	return document, nil
}

// AuthorizeProviderDownload issues a download grant for one
// PROVIDER_VISIBLE document owned by this provider's membership.
//
// Ownership and visibility are checked here, independently of and before
// AuthorizeDownload's own lifecycle/safety checks, because AuthorizeDownload
// alone has no notion of who is asking — only whether the document itself is
// in a releasable state.
func (s *Service) AuthorizeProviderDownload(ctx context.Context, providerService string, providerID string,
	documentID string, correlationID string, causationID string, requestID string) (*DownloadGrant, error) {
	organizationID, membershipID, err := s.ResolveProviderMembership(ctx, providerService, providerID)
	if err != nil {
		return nil, err
	}
	actor := providerActor(providerService, providerID, membershipID)

	document, err := s.opts.Documents.GetByID(ctx, nil, organizationID, documentID)
	if errors.Is(err, repositories.ErrNotFound) {
		s.recordAccess(ctx, organizationID, documentID, actor, "", models.AccessActionDownloadRequest,
			false, "not_found", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	if err != nil {
		return nil, err
	}
	if !providerOwnsDocument(document, membershipID) || document.Visibility != models.VisibilityProviderVisible {
		s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
			models.AccessActionDownloadRequest, false, "not_visible", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}

	grant, downloadErr := s.AuthorizeDownload(ctx, Actor{
		OrganizationID: organizationID, ActorType: models.ActorProvider, ActorID: membershipID,
	}, documentID, correlationID, causationID, requestID)
	reason := ""
	if downloadErr != nil {
		reason = "refused"
	}
	s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
		models.AccessActionDownloadRequest, downloadErr == nil, reason, correlationID)
	return grant, downloadErr
}

// --- customer-facing -------------------------------------------------------

func customerActor(customerID string, orderID string) ExternalActor {
	return ExternalActor{
		SubjectType: models.AccessSubjectCustomer, SubjectReference: customerID,
		ActorReference: "order:" + orderID,
	}
}

// requireCustomerOwnsOrder proves the asserted customer actually owns the
// order a document is scoped to, before anything about the order's
// documents is disclosed.
func (s *Service) requireCustomerOwnsOrder(ctx context.Context, organizationID string, orderID string,
	customerID string) error {
	owner, err := s.opts.Documents.CustomerForOrder(ctx, nil, organizationID, orderID)
	if errors.Is(err, repositories.ErrNotFound) {
		return errorcodes.NotFound(errorcodes.DocumentNotFound, "Order not found.")
	}
	if err != nil {
		return err
	}
	if owner != customerID {
		// Answered identically to "order not found": a customer must never be
		// able to tell "this order exists but is not yours" from "this order
		// does not exist".
		return errorcodes.NotFound(errorcodes.DocumentNotFound, "Order not found.")
	}
	return nil
}

// ListCustomerDocuments returns the CUSTOMER_VISIBLE documents attached to
// one order the customer owns.
func (s *Service) ListCustomerDocuments(ctx context.Context, organizationID string, orderID string,
	customerID string, correlationID string) ([]models.Document, error) {
	actor := customerActor(customerID, orderID)
	if err := s.requireCustomerOwnsOrder(ctx, organizationID, orderID, customerID); err != nil {
		s.recordAccess(ctx, organizationID, "", actor, models.VisibilityCustomerVisible,
			models.AccessActionList, false, "not_found", correlationID)
		return nil, err
	}

	documents, err := s.opts.Documents.List(ctx, tenant.Query{OrganizationID: organizationID, Limit: 200},
		repositories.ListFilter{OwnerType: models.OwnerOrder, OwnerID: orderID,
			Visibility: models.VisibilityCustomerVisible})
	if err != nil {
		return nil, err
	}
	s.recordAccess(ctx, organizationID, "", actor, models.VisibilityCustomerVisible,
		models.AccessActionList, true, "", correlationID)
	return documents, nil
}

// GetCustomerDocument returns one CUSTOMER_VISIBLE document attached to an
// order the customer owns.
func (s *Service) GetCustomerDocument(ctx context.Context, organizationID string, orderID string,
	customerID string, documentID string, correlationID string) (*models.Document, error) {
	actor := customerActor(customerID, orderID)
	if err := s.requireCustomerOwnsOrder(ctx, organizationID, orderID, customerID); err != nil {
		s.recordAccess(ctx, organizationID, documentID, actor, "", models.AccessActionRead,
			false, "not_found", correlationID)
		return nil, err
	}

	document, err := s.opts.Documents.GetByID(ctx, nil, organizationID, documentID)
	if errors.Is(err, repositories.ErrNotFound) {
		s.recordAccess(ctx, organizationID, documentID, actor, "", models.AccessActionRead,
			false, "not_found", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	if err != nil {
		return nil, err
	}
	if document.OwnerType != models.OwnerOrder || document.OwnerID != orderID ||
		document.Visibility != models.VisibilityCustomerVisible || document.Archived() {
		s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
			models.AccessActionRead, false, "not_visible", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
		models.AccessActionRead, true, "", correlationID)
	return document, nil
}

// AuthorizeCustomerDownload issues a download grant for one CUSTOMER_VISIBLE
// document attached to an order the customer owns.
func (s *Service) AuthorizeCustomerDownload(ctx context.Context, organizationID string, orderID string,
	customerID string, documentID string, correlationID string, causationID string,
	requestID string) (*DownloadGrant, error) {
	actor := customerActor(customerID, orderID)
	if err := s.requireCustomerOwnsOrder(ctx, organizationID, orderID, customerID); err != nil {
		s.recordAccess(ctx, organizationID, documentID, actor, "", models.AccessActionDownloadRequest,
			false, "not_found", correlationID)
		return nil, err
	}

	document, err := s.opts.Documents.GetByID(ctx, nil, organizationID, documentID)
	if errors.Is(err, repositories.ErrNotFound) {
		s.recordAccess(ctx, organizationID, documentID, actor, "", models.AccessActionDownloadRequest,
			false, "not_found", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}
	if err != nil {
		return nil, err
	}
	if document.OwnerType != models.OwnerOrder || document.OwnerID != orderID ||
		document.Visibility != models.VisibilityCustomerVisible {
		s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
			models.AccessActionDownloadRequest, false, "not_visible", correlationID)
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
	}

	grant, downloadErr := s.AuthorizeDownload(ctx, Actor{
		OrganizationID: organizationID, ActorType: "CUSTOMER", ActorID: customerID,
	}, documentID, correlationID, causationID, requestID)
	reason := ""
	if downloadErr != nil {
		reason = "refused"
	}
	s.recordAccess(ctx, organizationID, documentID, actor, document.Visibility,
		models.AccessActionDownloadRequest, downloadErr == nil, reason, correlationID)
	return grant, downloadErr
}
