package fleetprovider

import (
	"errors"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/fleetproviderwork"
	"cosmicforge/logistics/shared/go/providercompat"
)

// FleetError is the shared classified Fleet failure.
//
// Taxi, Hauling and Dispatch all need identical behaviour here, so the
// implementation lives in shared/go/fleetproviderwork and this is an alias.
// Two copies would drift, and the one that drifted would be the one that
// quietly downgraded a managed provider to independent.
type FleetError = fleetproviderwork.FleetError

// ErrFleetUnavailable is retained for callers that only need to know Fleet
// could not be reached. New code should classify with FleetError instead.
var ErrFleetUnavailable = errors.New("fleet provider work is unavailable")

// dependencyError builds a retryable dependency failure.
func dependencyError(err error) *FleetError { return fleetproviderwork.DependencyError(err) }

// classifyResponse turns a non-2xx Fleet response into a typed failure.
func classifyResponse(status int, body []byte) *FleetError {
	return fleetproviderwork.ClassifyResponse(status, body)
}

// AsAPIError maps any Fleet or managed-authorization failure onto the
// provider-facing API error.
func AsAPIError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrFleetUnavailable) {
		return apperrors.Unavailable("Fleet provider work is temporarily unavailable.", err)
	}
	return fleetproviderwork.AsAPIError(err)
}

// messageForKind is the provider-safe message for one classification.
func messageForKind(kind providercompat.FailureKind) string {
	return fleetproviderwork.MessageForKind(kind)
}

var _ = messageForKind
