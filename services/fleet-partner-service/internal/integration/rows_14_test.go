package integration_test

import (
	"context"
	"testing"
	"time"

	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
)

// Section 14 requirement rows that previously had no PostgreSQL test: the
// CRM list/get/history reads, opportunity/quotation/contract/SLA PATCH,
// quotation cancel/supersede/versions/documents, customer-link rejection,
// contract renew/history/versions/documents and SLA suspend/terminate/
// history/observations. These drive the real CRM use cases against
// PostgreSQL; route-level permissions are covered by the Section 14
// OpenAPI permission contract test.

func crmIDs[T any](items []T, id func(T) string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		out[id(item)] = true
	}
	return out
}

// TestRow14_2b_ContactList covers FLEET-14.2b: GET /customers/:id/contacts.
func TestRow14_2b_ContactList(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Contacts Ltd")
	mk := func(name, purpose string) crmmodels.Contact {
		c, err := f.service.CreateContact(ctx, f.tenant, customer.ID, crmusecases.CreateContactInput{FullName: name,
			Email: name + "@example.com", ContactPurpose: purpose, ConsentStatus: crmmodels.ConsentGranted, IdempotencyKey: "c:" + name})
		if err != nil {
			t.Fatalf("create contact %s: %v", name, err)
		}
		return c
	}
	ops := mk("ops", crmmodels.PurposeOperations)
	billing := mk("billing", crmmodels.PurposeBilling)
	old := mk("old", crmmodels.PurposeOperations)
	archived, err := f.service.ArchiveContact(ctx, f.tenant, customer.ID, old.ID, old.Version, "left_company", "archive-old", "")
	if err != nil {
		t.Fatal(err)
	}

	all, err := f.service.ListContacts(ctx, f.tenant, customer.ID, crmrepos.ContactFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	ids := crmIDs(all, func(c crmmodels.Contact) string { return c.ID })
	if len(all) != 3 || !ids[ops.ID] || !ids[billing.ID] || !ids[archived.ID] {
		t.Fatalf("contacts: %v", ids)
	}
	byPurpose, err := f.service.ListContacts(ctx, f.tenant, customer.ID, crmrepos.ContactFilter{ContactPurpose: crmmodels.PurposeBilling, Limit: 50})
	if err != nil || len(byPurpose) != 1 || byPurpose[0].ID != billing.ID {
		t.Fatalf("purpose filter: %+v %v", byPurpose, err)
	}
	byStatus, err := f.service.ListContacts(ctx, f.tenant, customer.ID, crmrepos.ContactFilter{Status: archived.Status, Limit: 50})
	if err != nil || len(byStatus) != 1 || byStatus[0].ID != old.ID {
		t.Fatalf("status filter: %+v %v", byStatus, err)
	}
	page, err := f.service.ListContacts(ctx, f.tenant, customer.ID, crmrepos.ContactFilter{Limit: 2})
	if err != nil || len(page) != 2 {
		t.Fatalf("page of 2: %d %v", len(page), err)
	}
	// Another organization cannot list this customer's contacts.
	if _, err = f.service.ListContacts(ctx, f.other, customer.ID, crmrepos.ContactFilter{Limit: 50}); crmErrorCode(err) != "customer_not_found" {
		t.Fatalf("cross-tenant contact list: %v", err)
	}
}

// TestRow14_3b_LocationListGet covers FLEET-14.3b: GET .../locations and
// GET .../locations/:id.
func TestRow14_3b_LocationListGet(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Locations Ltd")
	other := f.activeCustomer(t, "Other Locations Ltd")
	address := crmmodels.Address{Line1: "1 Wharf Road", City: "Apapa", StateOrRegion: "Lagos", CountryCode: "NG"}
	mk := func(customerID, kind, label string) crmmodels.Location {
		l, err := f.service.CreateLocation(ctx, f.tenant, customerID, crmusecases.CreateLocationInput{LocationType: kind, Label: label,
			Address: address, Timezone: "Africa/Lagos", IdempotencyKey: "l:" + customerID + label})
		if err != nil {
			t.Fatalf("create location %s: %v", label, err)
		}
		return l
	}
	depot := mk(customer.ID, crmmodels.LocationDepot, "Main depot")
	second := mk(customer.ID, crmmodels.LocationDepot, "Annex")
	foreignCustomerLocation := mk(other.ID, crmmodels.LocationDepot, "Elsewhere")

	all, err := f.service.ListLocations(ctx, f.tenant, customer.ID, crmrepos.LocationFilter{Limit: 50})
	ids := crmIDs(all, func(l crmmodels.Location) string { return l.ID })
	if err != nil || len(all) != 2 || !ids[depot.ID] || !ids[second.ID] || ids[foreignCustomerLocation.ID] {
		t.Fatalf("locations: %v %v", ids, err)
	}
	got, err := f.service.GetLocation(ctx, f.tenant, customer.ID, depot.ID)
	if err != nil || got.Label != "Main depot" || got.Address.City != "Apapa" || got.Timezone != "Africa/Lagos" {
		t.Fatalf("get location: %+v %v", got, err)
	}
	// A location is read through its own customer only.
	if _, err = f.service.GetLocation(ctx, f.tenant, other.ID, depot.ID); crmErrorCode(err) != "customer_location_not_found" {
		t.Fatalf("location via another customer: %v", err)
	}
	if _, err = f.service.GetLocation(ctx, f.other, customer.ID, depot.ID); err == nil {
		t.Fatal("cross-tenant location read must fail")
	}
	if _, err = f.service.ListLocations(ctx, f.other, customer.ID, crmrepos.LocationFilter{Limit: 50}); crmErrorCode(err) != "customer_not_found" {
		t.Fatalf("cross-tenant location list: %v", err)
	}
}

// TestRow14_4b_OpportunityListGetPatchHistory covers FLEET-14.4b:
// GET /sales/opportunities, GET/PATCH /:id and GET /:id/history.
func TestRow14_4b_OpportunityListGetPatchHistory(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Pipeline Ltd")
	other := f.activeCustomer(t, "Pipeline Two Ltd")
	first := f.customerOpportunity(t, customer.ID, "Lagos to Kano lane")
	second := f.customerOpportunity(t, other.ID, "Port shuttle")

	all, err := f.service.ListOpportunities(ctx, f.tenant, crmrepos.OpportunityFilter{Limit: 50})
	if ids := crmIDs(all, func(o crmmodels.Opportunity) string { return o.ID }); err != nil || len(all) != 2 || !ids[first.ID] || !ids[second.ID] {
		t.Fatalf("opportunities: %v %v", ids, err)
	}
	byCustomer, err := f.service.ListOpportunities(ctx, f.tenant, crmrepos.OpportunityFilter{CustomerID: other.ID, Limit: 50})
	if err != nil || len(byCustomer) != 1 || byCustomer[0].ID != second.ID {
		t.Fatalf("customer filter: %+v %v", byCustomer, err)
	}
	if foreign, err := f.service.ListOpportunities(ctx, f.other, crmrepos.OpportunityFilter{Limit: 50}); err != nil || len(foreign) != 0 {
		t.Fatalf("other organization opportunities: %+v %v", foreign, err)
	}
	got, err := f.service.GetOpportunity(ctx, f.tenant, first.ID)
	if err != nil || got.Title != "Lagos to Kano lane" || got.CustomerID != customer.ID {
		t.Fatalf("get opportunity: %+v %v", got, err)
	}
	if _, err = f.service.GetOpportunity(ctx, f.other, first.ID); crmErrorCode(err) != "opportunity_not_found" {
		t.Fatalf("cross-tenant opportunity: %v", err)
	}

	title := "Lagos to Kano dedicated lane"
	value := int64(250000000)
	currency := "NGN"
	probability := 6500
	if _, err = f.service.UpdateOpportunity(ctx, f.tenant, first.ID, crmusecases.UpdateOpportunityInput{Title: &title}); err == nil {
		t.Fatal("update without expectedVersion must be refused")
	}
	if _, err = f.service.UpdateOpportunity(ctx, f.tenant, first.ID, crmusecases.UpdateOpportunityInput{Title: &title, ExpectedVersion: 99}); err == nil {
		t.Fatal("stale opportunity version must conflict")
	}
	if _, err = f.service.UpdateOpportunity(ctx, f.other, first.ID, crmusecases.UpdateOpportunityInput{Title: &title, ExpectedVersion: first.Version}); crmErrorCode(err) != "opportunity_not_found" {
		t.Fatalf("cross-tenant update: %v", err)
	}
	updated, err := f.service.UpdateOpportunity(ctx, f.tenant, first.ID, crmusecases.UpdateOpportunityInput{Title: &title,
		ExpectedValueAmountMinor: &value, ExpectedValueCurrency: &currency, ProbabilityBasisPoints: &probability, ExpectedVersion: first.Version})
	if err != nil || updated.Title != title || updated.ExpectedValueAmountMinor == nil || *updated.ExpectedValueAmountMinor != value || updated.Version != first.Version+1 {
		t.Fatalf("update opportunity: %+v %v", updated, err)
	}
	// Money stays integer minor units with an ISO currency.
	if updated.ExpectedValueCurrency != "NGN" {
		t.Fatalf("expected value currency: %+v", updated.ExpectedValueCurrency)
	}
	won, err := f.service.TransitionOpportunity(ctx, f.tenant, first.ID, crmusecases.TransitionOpportunityInput{
		Stage: crmmodels.StageLost, ReasonCode: "price", ExpectedVersion: updated.Version, IdempotencyKey: "lost-1"})
	if err != nil {
		t.Fatalf("mark lost: %v", err)
	}
	// A closed opportunity no longer takes edits.
	if _, err = f.service.UpdateOpportunity(ctx, f.tenant, first.ID, crmusecases.UpdateOpportunityInput{Title: &title, ExpectedVersion: won.Version}); err == nil {
		t.Fatal("a lost opportunity must refuse edits")
	}
	// History is the stage trail: the move to LOST with its reason.
	history, err := f.service.OpportunityHistory(ctx, f.tenant, first.ID, emptyCursorValue(), false, 50)
	if err != nil || len(history) != 1 || history[0].PreviousStage != crmmodels.StageLead || history[0].NewStage != crmmodels.StageLost || history[0].ReasonCode != "price" {
		t.Fatalf("opportunity history: %+v %v", history, err)
	}
	if _, err = f.service.OpportunityHistory(ctx, f.other, first.ID, emptyCursorValue(), false, 50); crmErrorCode(err) != "opportunity_not_found" {
		t.Fatalf("cross-tenant history: %v", err)
	}
}

// TestRow14_5b_QuotationListAndPatch covers FLEET-14.5b: GET /quotations and
// PATCH /quotations/:id.
func TestRow14_5b_QuotationListAndPatch(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Quote List Ltd")
	draft := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))
	issued := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(96*time.Hour))

	all, err := f.service.ListQuotations(ctx, f.tenant, crmrepos.QuotationFilter{Limit: 50})
	if ids := crmIDs(all, func(q crmmodels.Quotation) string { return q.ID }); err != nil || len(all) != 2 || !ids[draft.Quotation.ID] || !ids[issued.Quotation.ID] {
		t.Fatalf("quotations: %v %v", ids, err)
	}
	drafts, err := f.service.ListQuotations(ctx, f.tenant, crmrepos.QuotationFilter{Status: crmmodels.QuotationDraft, Limit: 50})
	if err != nil || len(drafts) != 1 || drafts[0].ID != draft.Quotation.ID {
		t.Fatalf("status filter: %+v %v", drafts, err)
	}
	if foreign, err := f.service.ListQuotations(ctx, f.other, crmrepos.QuotationFilter{Limit: 50}); err != nil || len(foreign) != 0 {
		t.Fatalf("other organization quotations: %+v %v", foreign, err)
	}

	notes := "Customer asked for overnight standby"
	if _, err = f.service.UpdateQuotation(ctx, f.tenant, draft.Quotation.ID, crmusecases.UpdateQuotationInput{InternalNotes: &notes}); err == nil {
		t.Fatal("update without expectedVersion must be refused")
	}
	if _, err = f.service.UpdateQuotation(ctx, f.other, draft.Quotation.ID, crmusecases.UpdateQuotationInput{InternalNotes: &notes, ExpectedVersion: draft.Quotation.Version}); crmErrorCode(err) != "quotation_not_found" {
		t.Fatalf("cross-tenant update: %v", err)
	}
	if _, err = f.service.UpdateQuotation(ctx, f.tenant, draft.Quotation.ID, crmusecases.UpdateQuotationInput{InternalNotes: &notes, ExpectedVersion: 99}); err == nil {
		t.Fatal("stale quotation version must conflict")
	}
	updated, err := f.service.UpdateQuotation(ctx, f.tenant, draft.Quotation.ID, crmusecases.UpdateQuotationInput{
		InternalNotes: &notes, ReplaceLineItems: true, ExpectedVersion: draft.Quotation.Version,
		LineItems: []crmusecases.LineItemInput{{Description: "Dedicated 20t truck", Quantity: "3", UnitType: "TRIP", UnitAmountMinor: 400000, ManualReason: "renegotiated"}},
	})
	if err != nil {
		t.Fatalf("update quotation: %v", err)
	}
	// Totals are recomputed exactly from the manual line items: 3 x 400000.
	if updated.Quotation.TotalAmountMinor != 1200000 || updated.Quotation.Version <= draft.Quotation.Version || updated.Quotation.InternalNotes != notes {
		t.Fatalf("updated quotation: %+v", updated.Quotation)
	}
	// An issued quotation is immutable.
	if _, err = f.service.UpdateQuotation(ctx, f.tenant, issued.Quotation.ID, crmusecases.UpdateQuotationInput{InternalNotes: &notes, ExpectedVersion: issued.Quotation.Version}); err == nil {
		t.Fatal("an issued quotation must refuse edits")
	}
}

// TestRow14_5f_QuotationRejectByToken covers the untested part of FLEET-14.5f:
// rejection through the secure customer link.
func TestRow14_5f_QuotationRejectByToken(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Rejecting Ltd")
	issued := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))
	if issued.CustomerToken == "" {
		t.Fatal("issue did not mint a customer token")
	}
	if _, err := f.service.RejectQuotationByToken(ctx, "not-a-token", issued.Quotation.ID, crmusecases.RejectQuotationInput{ReasonCode: "too_expensive", IdempotencyKey: "rej-bad"}); err == nil {
		t.Fatal("an invalid token must not reject the quotation")
	}
	other := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(48*time.Hour))
	// A token is bound to its own quotation.
	if _, err := f.service.RejectQuotationByToken(ctx, issued.CustomerToken, other.Quotation.ID, crmusecases.RejectQuotationInput{ReasonCode: "too_expensive", IdempotencyKey: "rej-other"}); err == nil {
		t.Fatal("a token must not reject another quotation")
	}
	rejected, err := f.service.RejectQuotationByToken(ctx, issued.CustomerToken, issued.Quotation.ID, crmusecases.RejectQuotationInput{
		ReasonCode: "too_expensive", Note: "Budget cut", IdempotencyKey: "rej-1"})
	if err != nil || rejected.Status != crmmodels.QuotationRejected || rejected.ResolvedAt == nil {
		t.Fatalf("reject by token: %+v %v", rejected, err)
	}
	// A rejected quotation can no longer be accepted through the same link.
	if _, err = f.service.AcceptQuotationByToken(ctx, issued.CustomerToken, issued.Quotation.ID, crmusecases.AcceptQuotationInput{IdempotencyKey: "acc-after-reject"}); err == nil {
		t.Fatal("a rejected quotation must not be accepted")
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_quotation_history WHERE quotation_id=$1 AND new_status=$2`, issued.Quotation.ID, crmmodels.QuotationRejected); n != 1 {
		t.Fatalf("rejection history rows = %d", n)
	}
}

// TestRow14_5g_QuotationCancelAndSupersede covers FLEET-14.5g:
// POST /quotations/:id/cancel and /supersede.
func TestRow14_5g_QuotationCancelAndSupersede(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Withdrawn Ltd")
	toCancel := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))
	if _, err := f.service.CancelQuotation(ctx, f.tenant, toCancel.Quotation.ID, crmusecases.CancelQuotationInput{ReasonCode: "duplicate", IdempotencyKey: "cancel-nov"}); err == nil {
		t.Fatal("cancel without expectedVersion must be refused")
	}
	if _, err := f.service.CancelQuotation(ctx, f.other, toCancel.Quotation.ID, crmusecases.CancelQuotationInput{ReasonCode: "duplicate", ExpectedVersion: toCancel.Quotation.Version, IdempotencyKey: "cancel-x"}); crmErrorCode(err) != "quotation_not_found" {
		t.Fatalf("cross-tenant cancel: %v", err)
	}
	cancelled, err := f.service.CancelQuotation(ctx, f.tenant, toCancel.Quotation.ID, crmusecases.CancelQuotationInput{ReasonCode: "duplicate", ExpectedVersion: toCancel.Quotation.Version, IdempotencyKey: "cancel-1"})
	if err != nil || cancelled.Status != crmmodels.QuotationCancelled {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	// Cancelling again is an idempotent no-op: same row, no new version.
	again, err := f.service.CancelQuotation(ctx, f.tenant, toCancel.Quotation.ID, crmusecases.CancelQuotationInput{ReasonCode: "again", ExpectedVersion: cancelled.Version, IdempotencyKey: "cancel-2"})
	if err != nil || again.Version != cancelled.Version || again.Status != crmmodels.QuotationCancelled {
		t.Fatalf("repeat cancel: %+v %v", again, err)
	}
	// A cancelled quotation cannot be superseded or issued.
	if _, err = f.service.SupersedeQuotation(ctx, f.tenant, toCancel.Quotation.ID, crmusecases.SupersedeQuotationInput{ReplacementQuotationID: "qot_some_other", ReasonCode: "x", ExpectedVersion: cancelled.Version, IdempotencyKey: "sup-cancelled"}); crmErrorCode(err) != "quotation_already_resolved" {
		t.Fatalf("supersede a cancelled quotation: %v", err)
	}

	original := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(96*time.Hour))
	replacement := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(120*time.Hour))
	if _, err = f.service.SupersedeQuotation(ctx, f.tenant, original.Quotation.ID, crmusecases.SupersedeQuotationInput{ReplacementQuotationID: original.Quotation.ID, ReasonCode: "revised", ExpectedVersion: original.Quotation.Version, IdempotencyKey: "sup-self"}); err == nil {
		t.Fatal("a quotation must not supersede itself")
	}
	superseded, err := f.service.SupersedeQuotation(ctx, f.tenant, original.Quotation.ID, crmusecases.SupersedeQuotationInput{
		ReplacementQuotationID: replacement.Quotation.ID, ReasonCode: "revised_rates", ExpectedVersion: original.Quotation.Version, IdempotencyKey: "sup-1"})
	if err != nil || superseded.Status != crmmodels.QuotationSuperseded || superseded.SupersededByID != replacement.Quotation.ID {
		t.Fatalf("supersede: %+v %v", superseded, err)
	}
	// The withdrawn offer's customer link is revoked.
	if _, err = f.service.AcceptQuotationByToken(ctx, original.CustomerToken, original.Quotation.ID, crmusecases.AcceptQuotationInput{IdempotencyKey: "acc-superseded"}); err == nil {
		t.Fatal("a superseded quotation must not be accepted through its old link")
	}
	// Its issued version is preserved.
	if versions, err := f.service.QuotationVersions(ctx, f.tenant, original.Quotation.ID, 10); err != nil || len(versions) == 0 {
		t.Fatalf("superseded quotation lost its versions: %d %v", len(versions), err)
	}
}

// TestRow14_5h_QuotationVersionsHistoryDocuments covers FLEET-14.5h reads:
// GET .../versions, /versions/:v, /history and GET .../documents.
func TestRow14_5h_QuotationVersionsHistoryDocuments(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Versioned Ltd")
	issued := f.issuedQuotation(t, customer.ID, time.Now().UTC().Add(72*time.Hour))
	id := issued.Quotation.ID

	versions, err := f.service.QuotationVersions(ctx, f.tenant, id, 10)
	if err != nil || len(versions) != 1 || versions[0].QuotationID != id || versions[0].VersionNumber != 1 || versions[0].TotalAmountMinor != issued.Quotation.TotalAmountMinor {
		t.Fatalf("quotation versions: %+v %v", versions, err)
	}
	version, err := f.service.QuotationVersion(ctx, f.tenant, id, versions[0].ID)
	if err != nil || version.ContentFingerprint == "" || version.Currency != "NGN" {
		t.Fatalf("quotation version: %+v %v", version, err)
	}
	if _, err = f.service.QuotationVersion(ctx, f.other, id, versions[0].ID); err == nil {
		t.Fatal("cross-tenant version read must fail")
	}
	other := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(24*time.Hour))
	if _, err = f.service.QuotationVersion(ctx, f.tenant, other.Quotation.ID, versions[0].ID); err == nil {
		t.Fatal("a version must only be readable through its own quotation")
	}
	history, err := f.service.QuotationHistory(ctx, f.tenant, id, emptyCursorValue(), false, 50)
	if err != nil || len(history) < 3 {
		t.Fatalf("quotation history (create, submit/approve, issue): %d %v", len(history), err)
	}
	if _, err = f.service.QuotationHistory(ctx, f.other, id, emptyCursorValue(), false, 50); crmErrorCode(err) != "quotation_not_found" {
		t.Fatalf("cross-tenant history: %v", err)
	}
	draft := f.draftQuotation(t, customer.ID, time.Now().UTC().Add(48*time.Hour))
	if _, err = f.service.AttachQuotationDocument(ctx, f.tenant, draft.Quotation.ID, crmusecases.AttachQuotationDocumentInput{
		MediaObjectID: "media-q-1", DocumentPurpose: crmmodels.DocPurposeSupporting}); err != nil {
		t.Fatalf("attach quotation document: %v", err)
	}
	docs, err := f.service.QuotationDocuments(ctx, f.tenant, draft.Quotation.ID)
	if err != nil || len(docs) != 1 || docs[0].MediaObjectID != "media-q-1" || docs[0].SubjectID != draft.Quotation.ID {
		t.Fatalf("quotation documents: %+v %v", docs, err)
	}
	// Without a Media answer the document state is never assumed ready.
	if docs[0].ReadinessState == "READY" {
		t.Fatalf("document readiness assumed without Media: %+v", docs[0])
	}
	if _, err = f.service.QuotationDocuments(ctx, f.other, draft.Quotation.ID); crmErrorCode(err) != "quotation_not_found" {
		t.Fatalf("cross-tenant documents: %v", err)
	}
}

func (f *crmFixture) activeContract(t *testing.T, customerID, key string) crmmodels.Contract {
	t.Helper()
	ctx := context.Background()
	contract, err := f.service.CreateContract(ctx, f.tenant, crmusecases.CreateContractInput{CustomerID: customerID,
		EffectiveFrom: time.Now().UTC().Add(-time.Hour), CommercialSnapshot: crmmodels.JSONMap{"currency": "NGN", "monthlyMinorUnits": 5000000},
		TerminationTerms: crmmodels.JSONMap{"noticeDays": 30}, IdempotencyKey: "contract:" + key})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	submitted, err := f.service.SubmitContract(ctx, f.tenant, contract.ID, contract.Version, "submit:"+key, "", "")
	if err != nil {
		t.Fatalf("submit contract: %v", err)
	}
	active, _, err := f.service.ActivateContract(ctx, f.tenant, contract.ID, crmusecases.ActivateContractInput{SignatoryReferences: []string{"sig-" + key},
		ExpectedVersion: submitted.Version, IdempotencyKey: "activate:" + key})
	if err != nil {
		t.Fatalf("activate contract: %v", err)
	}
	return active
}

// TestRow14_7b_ContractListGetPatchRenewHistoryVersion covers FLEET-14.7b:
// GET /contracts, GET/PATCH /:id, POST /:id/renew, GET /:id/history and
// GET /:id/versions/:v.
func TestRow14_7b_ContractListGetPatchRenewHistoryVersion(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Contract Reads Ltd")
	draft, err := f.service.CreateContract(ctx, f.tenant, crmusecases.CreateContractInput{CustomerID: customer.ID,
		EffectiveFrom: time.Now().UTC().Add(24 * time.Hour), CommercialSnapshot: crmmodels.JSONMap{"currency": "NGN"}, IdempotencyKey: "contract-draft"})
	if err != nil {
		t.Fatal(err)
	}
	active := f.activeContract(t, customer.ID, "active")

	all, err := f.service.ListContracts(ctx, f.tenant, crmrepos.ContractFilter{Limit: 50})
	if ids := crmIDs(all, func(c crmmodels.Contract) string { return c.ID }); err != nil || len(all) != 2 || !ids[draft.ID] || !ids[active.ID] {
		t.Fatalf("contracts: %v %v", ids, err)
	}
	activeOnly, err := f.service.ListContracts(ctx, f.tenant, crmrepos.ContractFilter{Status: crmmodels.ContractActive, Limit: 50})
	if err != nil || len(activeOnly) != 1 || activeOnly[0].ID != active.ID {
		t.Fatalf("status filter: %+v %v", activeOnly, err)
	}
	if foreign, err := f.service.ListContracts(ctx, f.other, crmrepos.ContractFilter{Limit: 50}); err != nil || len(foreign) != 0 {
		t.Fatalf("other org contracts: %+v %v", foreign, err)
	}
	if got, err := f.service.GetContract(ctx, f.tenant, active.ID); err != nil || got.Status != crmmodels.ContractActive || got.ActiveVersionID == "" {
		t.Fatalf("get contract: %+v %v", got, err)
	}
	if _, err = f.service.GetContract(ctx, f.other, active.ID); crmErrorCode(err) != "contract_not_found" {
		t.Fatalf("cross-tenant contract: %v", err)
	}

	until := time.Now().UTC().Add(365 * 24 * time.Hour)
	if _, err = f.service.UpdateContract(ctx, f.tenant, draft.ID, crmusecases.UpdateContractInput{EffectiveTo: &until}); err == nil {
		t.Fatal("update without expectedVersion must be refused")
	}
	if _, err = f.service.UpdateContract(ctx, f.tenant, draft.ID, crmusecases.UpdateContractInput{EffectiveTo: &until, ExpectedVersion: 99}); err == nil {
		t.Fatal("stale contract version must conflict")
	}
	updated, err := f.service.UpdateContract(ctx, f.tenant, draft.ID, crmusecases.UpdateContractInput{EffectiveTo: &until,
		RenewalTerms: crmmodels.JSONMap{"autoRenew": false}, ExpectedVersion: draft.Version})
	if err != nil || updated.EffectiveTo == nil || updated.Version != draft.Version+1 {
		t.Fatalf("update contract: %+v %v", updated, err)
	}
	// An active contract is immutable; changes go through renewal.
	if _, err = f.service.UpdateContract(ctx, f.tenant, active.ID, crmusecases.UpdateContractInput{EffectiveTo: &until, ExpectedVersion: active.Version}); err == nil {
		t.Fatal("an active contract must refuse edits")
	}

	versions, err := f.service.ContractVersions(ctx, f.tenant, active.ID, 10)
	if err != nil || len(versions) != 1 {
		t.Fatalf("contract versions: %+v %v", versions, err)
	}
	version, err := f.service.ContractVersion(ctx, f.tenant, active.ID, versions[0].ID)
	if err != nil || version.ContractID != active.ID || version.ContentFingerprint == "" || len(version.SignatoryReferences) != 1 {
		t.Fatalf("contract version: %+v %v", version, err)
	}
	if _, err = f.service.ContractVersion(ctx, f.other, active.ID, versions[0].ID); err == nil {
		t.Fatal("cross-tenant contract version read must fail")
	}
	if _, err = f.service.ContractVersion(ctx, f.tenant, draft.ID, versions[0].ID); err == nil {
		t.Fatal("a version must only be readable through its own contract")
	}

	// Renewal: only an active/suspended contract renews, into a new draft
	// that supersedes it; the active version is untouched.
	if _, err = f.service.RenewContract(ctx, f.tenant, draft.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: updated.Version, IdempotencyKey: "renew-draft"}); crmErrorCode(err) != "contract_invalid_state" {
		t.Fatalf("renewing a draft: %v", err)
	}
	if _, err = f.service.RenewContract(ctx, f.tenant, active.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: active.Version}); err == nil {
		t.Fatal("renew without an idempotency key must be refused")
	}
	renewal, err := f.service.RenewContract(ctx, f.tenant, active.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: active.Version, IdempotencyKey: "renew-1"})
	if err != nil || renewal.Status != crmmodels.ContractDraft || renewal.SupersedesID != active.ID || renewal.ID == active.ID {
		t.Fatalf("renewal: %+v %v", renewal, err)
	}
	original, err := f.service.GetContract(ctx, f.tenant, active.ID)
	if err != nil || original.Status != crmmodels.ContractRenewalPending || original.ActiveVersionID != versions[0].ID {
		t.Fatalf("original after renewal: %+v %v", original, err)
	}
	// Replaying the same renewal must not open a second successor, and a
	// renewal against a stale version is refused.
	replay, err := f.service.RenewContract(ctx, f.tenant, active.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: active.Version, IdempotencyKey: "renew-1"})
	if err != nil || replay.ID != renewal.ID {
		t.Fatalf("renewal replay opened another successor: %+v %v", replay, err)
	}
	// A duplicate request against the version the renewal was opened from
	// gets the same successor; a request against the current version is
	// refused because a renewal is already open.
	if duplicate, err := f.service.RenewContract(ctx, f.tenant, active.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: active.Version, IdempotencyKey: "renew-2"}); err != nil || duplicate.ID != renewal.ID {
		t.Fatalf("duplicate renewal: %+v %v", duplicate, err)
	}
	if _, err = f.service.RenewContract(ctx, f.tenant, active.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: original.Version, IdempotencyKey: "renew-3"}); crmErrorCode(err) != "contract_version_conflict" {
		t.Fatalf("second renewal while one is open: %v", err)
	}
	if _, err = f.service.RenewContract(ctx, f.tenant, active.ID, crmusecases.RenewContractInput{EffectiveFrom: until, ExpectedVersion: 99, IdempotencyKey: "renew-4"}); crmErrorCode(err) != "contract_version_conflict" {
		t.Fatalf("renewal against an unknown version: %v", err)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_contracts WHERE organization_id=$1 AND supersedes_id=$2`, f.tenant.OrganizationID, active.ID); n != 1 {
		t.Fatalf("successor contracts = %d, want 1", n)
	}

	history, err := f.service.ContractHistory(ctx, f.tenant, active.ID, emptyCursorValue(), false, 50)
	if err != nil || len(history) < 3 {
		t.Fatalf("contract history: %d %v", len(history), err)
	}
	if history[len(history)-1].NewStatus == "" && history[0].NewStatus == "" {
		t.Fatalf("history rows carry no status: %+v", history)
	}
	if _, err = f.service.ContractHistory(ctx, f.other, active.ID, emptyCursorValue(), false, 50); crmErrorCode(err) != "contract_not_found" {
		t.Fatalf("cross-tenant contract history: %v", err)
	}
}

// TestRow14_7c_ContractDocuments covers FLEET-14.7c: GET/POST
// .../documents, .../:docId/replace and .../:docId/remove.
func TestRow14_7c_ContractDocuments(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "Contract Docs Ltd")
	contract, err := f.service.CreateContract(ctx, f.tenant, crmusecases.CreateContractInput{CustomerID: customer.ID,
		EffectiveFrom: time.Now().UTC().Add(24 * time.Hour), CommercialSnapshot: crmmodels.JSONMap{"currency": "NGN"}, IdempotencyKey: "contract-docs"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AttachContractDocument(ctx, f.tenant, contract.ID, crmusecases.AttachContractDocumentInput{DocumentPurpose: crmmodels.DocPurposeDraftContract}); crmErrorCode(err) != "contract_document_not_ready" {
		t.Fatalf("attach without media: %v", err)
	}
	if _, err = f.service.AttachContractDocument(ctx, f.tenant, contract.ID, crmusecases.AttachContractDocumentInput{MediaObjectID: "m-1", DocumentPurpose: crmmodels.DocPurposeQuotation}); crmErrorCode(err) != "contract_document_not_ready" {
		t.Fatalf("attach with a quotation purpose: %v", err)
	}
	if _, err = f.service.AttachContractDocument(ctx, f.other, contract.ID, crmusecases.AttachContractDocumentInput{MediaObjectID: "m-1", DocumentPurpose: crmmodels.DocPurposeDraftContract}); crmErrorCode(err) != "contract_not_found" {
		t.Fatalf("cross-tenant attach: %v", err)
	}
	draftDoc, err := f.service.AttachContractDocument(ctx, f.tenant, contract.ID, crmusecases.AttachContractDocumentInput{MediaObjectID: "media-draft-1", DocumentPurpose: crmmodels.DocPurposeDraftContract, IsMandatory: true})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	supporting, err := f.service.AttachContractDocument(ctx, f.tenant, contract.ID, crmusecases.AttachContractDocumentInput{MediaObjectID: "media-support-1", DocumentPurpose: crmmodels.DocPurposeSupporting})
	if err != nil {
		t.Fatalf("attach supporting: %v", err)
	}
	docs, err := f.service.ContractDocuments(ctx, f.tenant, contract.ID)
	if ids := crmIDs(docs, func(d crmmodels.DocumentReference) string { return d.ID }); err != nil || len(docs) != 2 || !ids[draftDoc.ID] || !ids[supporting.ID] {
		t.Fatalf("contract documents: %v %v", ids, err)
	}
	if _, err = f.service.ContractDocuments(ctx, f.other, contract.ID); crmErrorCode(err) != "contract_not_found" {
		t.Fatalf("cross-tenant document list: %v", err)
	}

	replacement, err := f.service.ReplaceContractDocument(ctx, f.tenant, contract.ID, draftDoc.ID, crmusecases.AttachContractDocumentInput{MediaObjectID: "media-draft-2"})
	if err != nil || replacement.MediaObjectID != "media-draft-2" || replacement.DocumentPurpose != crmmodels.DocPurposeDraftContract {
		t.Fatalf("replace: %+v %v", replacement, err)
	}
	if _, err = f.service.ReplaceContractDocument(ctx, f.other, contract.ID, replacement.ID, crmusecases.AttachContractDocumentInput{MediaObjectID: "media-x"}); err == nil {
		t.Fatal("cross-tenant replace must fail")
	}
	docs, _ = f.service.ContractDocuments(ctx, f.tenant, contract.ID)
	if ids := crmIDs(docs, func(d crmmodels.DocumentReference) string { return d.MediaObjectID }); len(docs) != 2 || !ids["media-draft-2"] || ids["media-draft-1"] {
		t.Fatalf("documents after replace: %v", ids)
	}

	if err = f.service.RemoveContractDocument(ctx, f.other, contract.ID, supporting.ID, "cleanup"); crmErrorCode(err) != "contract_not_found" {
		t.Fatalf("cross-tenant remove: %v", err)
	}
	if err = f.service.RemoveContractDocument(ctx, f.tenant, contract.ID, supporting.ID, "superseded draft"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err = f.service.RemoveContractDocument(ctx, f.tenant, contract.ID, supporting.ID, "again"); err == nil {
		t.Fatal("removing an already removed document must fail")
	}
	docs, _ = f.service.ContractDocuments(ctx, f.tenant, contract.ID)
	if len(docs) != 1 || docs[0].ID != replacement.ID {
		t.Fatalf("documents after remove: %+v", docs)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2 AND action LIKE 'crm.contract.document_%'`, f.tenant.OrganizationID, contract.ID); n < 4 {
		t.Fatalf("contract document audit records = %d", n)
	}
}

// TestRow14_8b_SLAListGetPatchSuspendTerminateHistoryObservations covers
// FLEET-14.8b.
func TestRow14_8b_SLAListGetPatchSuspendTerminateHistoryObservations(t *testing.T) {
	f := newCRMFixture(t, allCRMFlagsOn())
	ctx := context.Background()
	customer := f.activeCustomer(t, "SLA Reads Ltd")
	mk := func(metric, target, unit, key string) crmmodels.SLA {
		sla, err := f.service.CreateSLA(ctx, f.tenant, crmusecases.CreateSLAInput{CustomerID: customer.ID, MetricCode: metric,
			TargetValue: target, MeasurementUnit: unit, MeasurementWindow: "MONTHLY", EffectiveFrom: time.Now().UTC().Add(-time.Hour), IdempotencyKey: key})
		if err != nil {
			t.Fatalf("create SLA: %v", err)
		}
		return sla
	}
	onTime := mk(crmmodels.MetricDeliveryOnTime, "95", "PERCENT", "sla-on-time")
	pickup := mk(crmmodels.MetricPickupOnTime, "90", "PERCENT", "sla-pickup")

	all, err := f.service.ListSLAs(ctx, f.tenant, crmrepos.SLAFilter{Limit: 50})
	if ids := crmIDs(all, func(s crmmodels.SLA) string { return s.ID }); err != nil || len(all) != 2 || !ids[onTime.ID] || !ids[pickup.ID] {
		t.Fatalf("SLAs: %v %v", ids, err)
	}
	byMetric, err := f.service.ListSLAs(ctx, f.tenant, crmrepos.SLAFilter{MetricCode: crmmodels.MetricPickupOnTime, Limit: 50})
	if err != nil || len(byMetric) != 1 || byMetric[0].ID != pickup.ID {
		t.Fatalf("metric filter: %+v %v", byMetric, err)
	}
	if foreign, err := f.service.ListSLAs(ctx, f.other, crmrepos.SLAFilter{Limit: 50}); err != nil || len(foreign) != 0 {
		t.Fatalf("other org SLAs: %+v %v", foreign, err)
	}
	if got, err := f.service.GetSLA(ctx, f.tenant, onTime.ID); err != nil || got.TargetValue != "95" || got.Status != crmmodels.SLADraft {
		t.Fatalf("get SLA: %+v %v", got, err)
	}
	if _, err = f.service.GetSLA(ctx, f.other, onTime.ID); crmErrorCode(err) != "sla_not_found" {
		t.Fatalf("cross-tenant SLA: %v", err)
	}

	target := "97"
	if _, err = f.service.UpdateSLA(ctx, f.tenant, onTime.ID, crmusecases.UpdateSLAInput{TargetValue: &target}); err == nil {
		t.Fatal("update without expectedVersion must be refused")
	}
	if _, err = f.service.UpdateSLA(ctx, f.tenant, onTime.ID, crmusecases.UpdateSLAInput{TargetValue: &target, ExpectedVersion: 42}); err == nil {
		t.Fatal("stale SLA version must conflict")
	}
	updated, err := f.service.UpdateSLA(ctx, f.tenant, onTime.ID, crmusecases.UpdateSLAInput{TargetValue: &target, ExpectedVersion: onTime.Version})
	if err != nil || updated.TargetValue != "97" || updated.Version != onTime.Version+1 {
		t.Fatalf("update SLA: %+v %v", updated, err)
	}
	active, err := f.service.ActivateSLA(ctx, f.tenant, onTime.ID, crmusecases.SLALifecycleInput{ExpectedVersion: updated.Version, IdempotencyKey: "sla-activate"})
	if err != nil || active.Status != crmmodels.SLAActive {
		t.Fatalf("activate SLA: %+v %v", active, err)
	}
	// An active SLA's commitments are fixed.
	if _, err = f.service.UpdateSLA(ctx, f.tenant, onTime.ID, crmusecases.UpdateSLAInput{TargetValue: &target, ExpectedVersion: active.Version}); err == nil {
		t.Fatal("an active SLA must refuse edits")
	}

	measured := "93.5"
	now := time.Now().UTC()
	if _, err = f.service.IngestObservation(ctx, "driver-dispatch-delivery-service", crmusecases.IngestObservationInput{OrganizationID: f.tenant.OrganizationID,
		SLAID: onTime.ID, SourceType: "DELIVERY", SourceReference: "trip-14", MeasuredValue: &measured, MeasurementUnit: "PERCENT",
		MeasuredAt: now, WindowStart: now.Add(-time.Hour), WindowEnd: now, SourceVersion: 1, PayloadHash: "hash-14"}); err != nil {
		t.Fatalf("ingest observation: %v", err)
	}
	observations, err := f.service.SLAObservations(ctx, f.tenant, onTime.ID, "", emptyCursorValue(), false, 50)
	if err != nil || len(observations) != 1 || observations[0].SourceReference != "trip-14" || observations[0].Outcome == "" {
		t.Fatalf("SLA observations: %+v %v", observations, err)
	}
	if filtered, err := f.service.SLAObservations(ctx, f.tenant, onTime.ID, "NO_SUCH_OUTCOME", emptyCursorValue(), false, 50); err == nil && len(filtered) != 0 {
		t.Fatalf("outcome filter ignored: %+v", filtered)
	}
	if _, err = f.service.SLAObservations(ctx, f.other, onTime.ID, "", emptyCursorValue(), false, 50); crmErrorCode(err) != "sla_not_found" {
		t.Fatalf("cross-tenant observations: %v", err)
	}

	if _, err = f.service.SuspendSLA(ctx, f.tenant, onTime.ID, crmusecases.SLALifecycleInput{ReasonCode: "customer_request", IdempotencyKey: "sla-suspend-nov"}); err == nil {
		t.Fatal("suspend without expectedVersion must be refused")
	}
	if _, err = f.service.SuspendSLA(ctx, f.other, onTime.ID, crmusecases.SLALifecycleInput{ReasonCode: "x", ExpectedVersion: active.Version, IdempotencyKey: "sla-suspend-x"}); crmErrorCode(err) != "sla_not_found" {
		t.Fatalf("cross-tenant suspend: %v", err)
	}
	current, _ := f.service.GetSLA(ctx, f.tenant, onTime.ID)
	suspended, err := f.service.SuspendSLA(ctx, f.tenant, onTime.ID, crmusecases.SLALifecycleInput{ReasonCode: "customer_request", ExpectedVersion: current.Version, IdempotencyKey: "sla-suspend"})
	if err != nil || suspended.Status != crmmodels.SLASuspended {
		t.Fatalf("suspend SLA: %+v %v", suspended, err)
	}
	terminated, err := f.service.TerminateSLA(ctx, f.tenant, onTime.ID, crmusecases.SLALifecycleInput{ReasonCode: "contract_ended", ExpectedVersion: suspended.Version, IdempotencyKey: "sla-terminate"})
	if err != nil || terminated.Status != crmmodels.SLATerminated {
		t.Fatalf("terminate SLA: %+v %v", terminated, err)
	}
	// TERMINATED is final.
	if _, err = f.service.ActivateSLA(ctx, f.tenant, onTime.ID, crmusecases.SLALifecycleInput{ExpectedVersion: terminated.Version, IdempotencyKey: "sla-reactivate"}); err == nil {
		t.Fatal("a terminated SLA must not be reactivated")
	}
	history, err := f.service.SLAHistory(ctx, f.tenant, onTime.ID, emptyCursorValue(), false, 50)
	if err != nil || len(history) < 4 {
		t.Fatalf("SLA history (create, activate, suspend, terminate): %d %v", len(history), err)
	}
	if _, err = f.service.SLAHistory(ctx, f.other, onTime.ID, emptyCursorValue(), false, 50); crmErrorCode(err) != "sla_not_found" {
		t.Fatalf("cross-tenant SLA history: %v", err)
	}
}
