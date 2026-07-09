package partnerauth

import "context"

// APIKeyRepository looks up partner API keys by their public prefix and records
// usage. A concrete Postgres implementation is intentionally deferred until the
// api-gateway DB bootstrap exists (GW later); GW-C only needs this interface so
// the middleware can be tested with a fake repository.
type APIKeyRepository interface {
	// FindByPrefix returns the API key record matching the given key prefix.
	// It returns (nil, nil) when no key with that prefix exists.
	FindByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	// UpdateLastUsed records that the key was used. It is called asynchronously
	// and its error is intentionally ignored by callers.
	UpdateLastUsed(ctx context.Context, keyID string) error
}
