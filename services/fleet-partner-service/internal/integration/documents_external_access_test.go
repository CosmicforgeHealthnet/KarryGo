package integration_test

import (
	"context"
	"testing"

	documentmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
)

// --- provider-facing document access ---------------------------------------
//
// The provider surface is reached only through the HMAC service-relay
// pattern: a driver service has already authenticated the provider's own
// session and asserts its identity here. Fleet independently resolves that
// identity to a live membership before disclosing anything, exactly as
// usecases.Service.ResolveProviderMembership documents.

// providerVisibleDocument uploads a SAFE, PROVIDER_VISIBLE document owned by
// one provider membership.
func providerVisibleDocument(t *testing.T, f *documentFixture, submitter string,
	ownerType string, membershipID string, visibility string) *documentusecases.UploadTicket {
	t.Helper()
	ticket, err := f.service.RequestUpload(context.Background(), documentusecases.RequestUploadInput{
		Actor: f.actor(submitter), OwnerType: ownerType, OwnerID: membershipID,
		Category: "COMPLIANCE", DocumentType: "PROVIDER_ID_CARD",
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		Visibility:     visibility,
		IdempotencyKey: "doc-provider-" + membershipID + "-" + visibility + "-" + randomHex(),
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	f.media.completeUpload(ticket.Document.MediaObjectID, "SAFE", []byte("%PDF-1.7 provider doc"))
	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil); err != nil {
		t.Fatalf("deliver upload event: %v", err)
	}
	return ticket
}

func accessAuditCount(t *testing.T, f *documentFixture, documentID string, result string) int {
	t.Helper()
	var count int
	query := "SELECT count(*) FROM fleet_partner_document_access_audit WHERE result = $1"
	args := []any{result}
	if documentID != "" {
		query += " AND document_id = $2"
		args = append(args, documentID)
	}
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count access audit: %v", err)
	}
	return count
}

// TestProviderDocumentAccess_ScopedToOwnMembership proves a provider can only
// list, read and download documents anchored to its own membership, at the
// PROVIDER_VISIBLE tier, and that both allowed and denied attempts are
// recorded in the access audit.
func TestProviderDocumentAccess_ScopedToOwnMembership(t *testing.T) {
	f := newDocumentFixture(t, "Doc Provider Access Co", "DOC-PROV-1")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")

	own := seedProviderMembership(t, f.pool, f.orgID, "prov_own_"+randomHex())
	other := seedProviderMembership(t, f.pool, f.orgID, "prov_other_"+randomHex())

	visible := providerVisibleDocument(t, f, submitter, documentmodels.OwnerProviderLink,
		own.ID, documentmodels.VisibilityProviderVisible)
	internalOnly := providerVisibleDocument(t, f, submitter, documentmodels.OwnerProviderLink,
		own.ID, documentmodels.VisibilityInternal)
	othersDocument := providerVisibleDocument(t, f, submitter, documentmodels.OwnerProviderLink,
		other.ID, documentmodels.VisibilityProviderVisible)

	// List returns only this membership's PROVIDER_VISIBLE document.
	documents, err := f.service.ListProviderDocuments(context.Background(),
		own.ProviderService, *own.ProviderID, "cor-"+randomHex())
	if err != nil {
		t.Fatalf("list provider documents: %v", err)
	}
	if len(documents) != 1 || documents[0].ID != visible.Document.ID {
		t.Fatalf("list = %+v, want exactly the one PROVIDER_VISIBLE document", documents)
	}

	// Reading the owner's own INTERNAL-visibility document is refused: the
	// tier itself is the entire access decision, and there is no permission
	// that could widen it for an external caller.
	if _, err := f.service.GetProviderDocument(context.Background(), own.ProviderService,
		*own.ProviderID, internalOnly.Document.ID, "cor-"+randomHex()); err == nil {
		t.Fatal("an INTERNAL-visibility document must not be reachable from the provider surface")
	}

	// Reading another provider's document, even at PROVIDER_VISIBLE, is
	// refused: ownership is checked independently of visibility.
	if _, err := f.service.GetProviderDocument(context.Background(), own.ProviderService,
		*own.ProviderID, othersDocument.Document.ID, "cor-"+randomHex()); err == nil {
		t.Fatal("another provider's document must not be reachable")
	}

	// Reading the owner's own PROVIDER_VISIBLE document succeeds.
	document, err := f.service.GetProviderDocument(context.Background(), own.ProviderService,
		*own.ProviderID, visible.Document.ID, "cor-"+randomHex())
	if err != nil {
		t.Fatalf("get own provider document: %v", err)
	}
	if document.ID != visible.Document.ID {
		t.Fatalf("got document %s, want %s", document.ID, visible.Document.ID)
	}

	// A download grant can be authorized for the owner's own document.
	grant, err := f.service.AuthorizeProviderDownload(context.Background(), own.ProviderService,
		*own.ProviderID, visible.Document.ID, "cor-"+randomHex(), "", "")
	if err != nil {
		t.Fatalf("authorize provider download: %v", err)
	}
	if grant.Token == "" {
		t.Fatal("no download grant was issued")
	}

	// An unknown provider identity resolves to NotFound, not a 5xx.
	if _, err := f.service.ListProviderDocuments(context.Background(), own.ProviderService,
		"prov_does_not_exist", "cor-"+randomHex()); err == nil {
		t.Fatal("an unknown provider identity must be refused")
	}

	// Every attempt above — allowed and denied — was written to the
	// append-only access audit.
	if allowed := accessAuditCount(t, f, "", documentmodels.AccessResultAllowed); allowed < 3 {
		t.Fatalf("allowed access audit rows = %d, want at least 3", allowed)
	}
	if denied := accessAuditCount(t, f, "", documentmodels.AccessResultDenied); denied < 2 {
		t.Fatalf("denied access audit rows = %d, want at least 2", denied)
	}
}

// TestProviderDocumentAccess_MembershipLookupDisabled proves the surface
// fails closed, rather than open, when no membership port is configured.
func TestProviderDocumentAccess_MembershipLookupDisabled(t *testing.T) {
	f := newDocumentFixture(t, "Doc Provider Disabled Co", "DOC-PROV-2")
	disabled := documentusecases.NewService(documentusecases.Options{
		Pool: f.pool, Documents: nil, Media: f.client, Flags: documentusecases.Flags{DownloadEnabled: true},
	})
	if _, _, err := disabled.ResolveProviderMembership(context.Background(),
		"driver-dispatch-delivery-service", "prov_x"); err == nil {
		t.Fatal("expected a fail-closed error when no membership port is configured")
	}
}

// --- customer-facing document access ----------------------------------------

// seedCompanyCustomer inserts a minimal fleet_partner_company_customers row
// directly, bypassing the full CRM service: this suite only needs a stable
// customer id to prove document ownership scoping, not CRM behaviour.
func seedCompanyCustomer(t *testing.T, f *documentFixture, name string) string {
	t.Helper()
	id := "cus_" + randomHex()
	_, err := f.pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_company_customers
			(id, organization_id, customer_type, legal_name, display_name,
			 normalized_name, duplicate_fingerprint, status, default_currency, created_by)
		VALUES ($1, $2, 'BUSINESS', $3, $3, $3, $1, 'ACTIVE', 'NGN', 'test')`,
		id, f.orgID, name)
	if err != nil {
		t.Fatalf("seed company customer: %v", err)
	}
	return id
}

// seedPrivateOrder inserts a minimal fleet_partner_private_orders row
// directly, bypassing the full orders/catalog services: this suite only
// needs a stable order id owned by a known customer.
func seedPrivateOrder(t *testing.T, f *documentFixture, customerID string) string {
	t.Helper()
	catalogItemID := "svc_" + randomHex()
	_, err := f.pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_service_catalog_items
			(id, organization_id, name, code, category, created_by)
		VALUES ($1, $2, 'Same Day Delivery', $1, 'SAME_DAY_DELIVERY', 'test')`,
		catalogItemID, f.orgID)
	if err != nil {
		t.Fatalf("seed catalog item: %v", err)
	}
	orderID := "ord_" + randomHex()
	_, err = f.pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_private_orders
			(id, organization_id, order_number, customer_id, source,
			 service_catalog_item_id, timezone, created_by)
		VALUES ($1, $2, $1, $3, 'MANUAL', $4, 'Africa/Lagos', 'test')`,
		orderID, f.orgID, customerID, catalogItemID)
	if err != nil {
		t.Fatalf("seed private order: %v", err)
	}
	return orderID
}

// customerVisibleDocument uploads a SAFE, CUSTOMER_VISIBLE document owned by
// one private order.
func customerVisibleDocument(t *testing.T, f *documentFixture, submitter string,
	orderID string, visibility string) *documentusecases.UploadTicket {
	t.Helper()
	ticket, err := f.service.RequestUpload(context.Background(), documentusecases.RequestUploadInput{
		Actor: f.actor(submitter), OwnerType: documentmodels.OwnerOrder, OwnerID: orderID,
		Category: "PROOF", DocumentType: "PROOF_OF_DELIVERY",
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		Visibility:     visibility,
		IdempotencyKey: "doc-order-" + orderID + "-" + visibility + "-" + randomHex(),
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	f.media.completeUpload(ticket.Document.MediaObjectID, "SAFE", []byte("%PDF-1.7 order doc"))
	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil); err != nil {
		t.Fatalf("deliver upload event: %v", err)
	}
	return ticket
}

// TestCustomerDocumentAccess_ScopedToOwnOrder proves a customer can only
// list, read and download CUSTOMER_VISIBLE documents attached to an order it
// actually owns, and that "order exists but is not yours" and "order does
// not exist" are indistinguishable.
func TestCustomerDocumentAccess_ScopedToOwnOrder(t *testing.T) {
	f := newDocumentFixture(t, "Doc Customer Access Co", "DOC-CUST-1")
	submitter := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "submitter")

	owner := seedCompanyCustomer(t, f, "Owning Customer")
	stranger := seedCompanyCustomer(t, f, "Stranger Customer")
	orderID := seedPrivateOrder(t, f, owner)

	visible := customerVisibleDocument(t, f, submitter, orderID, documentmodels.VisibilityCustomerVisible)
	internalOnly := customerVisibleDocument(t, f, submitter, orderID, documentmodels.VisibilityInternal)

	// The owning customer sees exactly the CUSTOMER_VISIBLE document.
	documents, err := f.service.ListCustomerDocuments(context.Background(), f.orgID, orderID, owner,
		"cor-"+randomHex())
	if err != nil {
		t.Fatalf("list customer documents: %v", err)
	}
	if len(documents) != 1 || documents[0].ID != visible.Document.ID {
		t.Fatalf("list = %+v, want exactly the one CUSTOMER_VISIBLE document", documents)
	}

	// A stranger asserting the same order id is refused identically to a
	// nonexistent order.
	_, strangerErr := f.service.ListCustomerDocuments(context.Background(), f.orgID, orderID, stranger,
		"cor-"+randomHex())
	_, missingErr := f.service.ListCustomerDocuments(context.Background(), f.orgID, "ord_does_not_exist",
		stranger, "cor-"+randomHex())
	if strangerErr == nil || missingErr == nil {
		t.Fatal("both a wrong-owner order and a nonexistent order must be refused")
	}
	if strangerErr.Error() != missingErr.Error() {
		t.Fatalf("wrong-owner and nonexistent order errors differ: %q vs %q",
			strangerErr.Error(), missingErr.Error())
	}

	// The owning customer cannot reach the INTERNAL-visibility document on
	// their own order.
	if _, err := f.service.GetCustomerDocument(context.Background(), f.orgID, orderID, owner,
		internalOnly.Document.ID, "cor-"+randomHex()); err == nil {
		t.Fatal("an INTERNAL-visibility document must not be reachable from the customer surface")
	}

	// The owning customer can read their own CUSTOMER_VISIBLE document.
	if _, err := f.service.GetCustomerDocument(context.Background(), f.orgID, orderID, owner,
		visible.Document.ID, "cor-"+randomHex()); err != nil {
		t.Fatalf("get own customer document: %v", err)
	}

	// The owning customer can download their own CUSTOMER_VISIBLE document.
	grant, err := f.service.AuthorizeCustomerDownload(context.Background(), f.orgID, orderID, owner,
		visible.Document.ID, "cor-"+randomHex(), "", "")
	if err != nil {
		t.Fatalf("authorize customer download: %v", err)
	}
	if grant.Token == "" {
		t.Fatal("no download grant was issued")
	}

	if allowed := accessAuditCount(t, f, visible.Document.ID, documentmodels.AccessResultAllowed); allowed < 2 {
		t.Fatalf("allowed access audit rows for the visible document = %d, want at least 2", allowed)
	}
}
