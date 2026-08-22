package inventory

import "errors"

var (
	ErrUnsupportedItemType = errors.New("inventory item type is not enabled")
	ErrSourceNotFound      = errors.New("inventory source not found")
	ErrSourceInactive      = errors.New("inventory source is inactive")
	ErrDuplicateItem       = errors.New("inventory item already exists")
	ErrInvalidMeasure      = errors.New("inventory stock measure is invalid")
	ErrMeasurementNotExact = errors.New("inventory stock measure is not exact")
	ErrItemNotFound        = errors.New("inventory item not found")
	ErrVersionConflict     = errors.New("inventory item version conflict")
)
