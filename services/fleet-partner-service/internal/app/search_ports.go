package app

import (
	"context"

	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	searchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
)

// ordersSearchAdapter lets the orders feature index into Section 25 search
// without importing the search feature's usecases/models packages directly.
//
// This adapter lives at the composition root because it is the only place
// allowed to know both features. Its job is pure translation: orders'
// SearchIndexInput carries no dependency on search's own types, and this is
// where the two vocabularies meet.
type ordersSearchAdapter struct {
	search *searchusecases.Service
}

// Index implements ordersusecases.SearchIndexer.
func (a ordersSearchAdapter) Index(ctx context.Context, organizationID string,
	in ordersusecases.SearchIndexInput) error {
	_, err := a.search.Index(ctx, organizationID, searchusecases.IndexInput{
		BranchID: in.BranchID, SubjectType: in.SubjectType, SubjectReference: in.SubjectReference,
		Title: in.Title, SearchableText: in.SearchableText,
		ExactReferenceFields: searchmodels.JSONMap(in.ExactReferenceFields),
		Status:               in.Status, RequiredPermission: in.RequiredPermission,
		SourceVersion: in.SourceVersion,
	})
	return err
}

// RemoveFromIndex implements ordersusecases.SearchIndexer.
func (a ordersSearchAdapter) RemoveFromIndex(ctx context.Context, organizationID,
	subjectType, subjectReference string) error {
	return a.search.RemoveFromIndex(ctx, organizationID, subjectType, subjectReference)
}

// NewOrdersSearchAdapter builds the orders search-indexing port.
//
// A nil search service (the search domain is not enabled for this
// deployment) produces a nil SearchIndexer, which orders/usecases already
// treats as "indexing disabled" rather than an error.
func NewOrdersSearchAdapter(search *searchusecases.Service) ordersusecases.SearchIndexer {
	if search == nil {
		return nil
	}
	return ordersSearchAdapter{search: search}
}

// Compile-time proof that the adapter satisfies the port.
var _ ordersusecases.SearchIndexer = ordersSearchAdapter{}
