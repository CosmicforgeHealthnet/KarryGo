package usecases

import (
	"fmt"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

func newID(prefix string) string { return ids.New(prefix) }

func itoa(v int64) string { return fmt.Sprint(v) }

func conflict(code apperrors.Code, message string) error {
	return errorcodes.Conflict(code, message)
}

func unavailable(code apperrors.Code, message string) error {
	return errorcodes.Unavailable(code, message)
}

func badRequest(message string) error { return apperrors.BadRequest(message, nil) }

func forbiddenTaxi() error {
	return errorcodes.Forbidden("taxi_execution_disabled", "Passenger route publication is disabled.")
}
