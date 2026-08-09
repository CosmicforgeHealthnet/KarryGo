package usecases

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// CreateLocationInput opens a customer site.
type CreateLocationInput struct {
	LocationType       string
	Label              string
	Address            models.Address
	Latitude           *float64
	Longitude          *float64
	GeofenceReference  string
	OperatingHours     models.JSONMap
	AccessInstructions string
	SafetyInstructions string
	ContactReference   string
	Timezone           string
	IdempotencyKey     string
	CorrelationID      string
	CausationID        string
}

// CreateLocation records a customer site.
func (s *Service) CreateLocation(ctx context.Context, t tenant.Context, customerID string,
	input CreateLocationInput) (models.Location, error) {
	if err := s.writeEnabled(s.opts.Flags.LocationWritesEnabled); err != nil {
		return models.Location{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Location{}, err
	}
	if err := validateLocationInput(&input); err != nil {
		return models.Location{}, err
	}

	var created models.Location
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, customerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot receive new locations.")
		}
		if input.ContactReference != "" {
			// The contact must belong to this customer. Allowing a location to
			// point at another customer's contact would leak one customer's
			// staff into another's operational instructions.
			if _, txErr = s.opts.Repository.GetContact(ctx, tx, t.OrganizationID, customerID,
				input.ContactReference); txErr != nil {
				return errorcodes.BadRequest(errorcodes.CustomerContactNotFound,
					"contactReference does not belong to this customer.")
			}
		}

		record := models.Location{
			ID:                 ids.New(repositories.PrefixLocation),
			OrganizationID:     t.OrganizationID,
			CustomerID:         customerID,
			LocationType:       input.LocationType,
			Label:              strings.TrimSpace(input.Label),
			Address:            input.Address,
			Latitude:           input.Latitude,
			Longitude:          input.Longitude,
			GeofenceReference:  input.GeofenceReference,
			OperatingHours:     input.OperatingHours,
			AccessInstructions: strings.TrimSpace(input.AccessInstructions),
			SafetyInstructions: strings.TrimSpace(input.SafetyInstructions),
			ContactReference:   input.ContactReference,
			Timezone:           input.Timezone,
			Status:             models.RecordStatusActive,
			CreatedBy:          t.OrganizationUserID,
		}
		created, txErr = s.opts.Repository.CreateLocation(ctx, tx, record)
		if txErr != nil {
			return translate(txErr, "Location", errorcodes.CustomerLocationNotFound,
				errorcodes.CustomerLocationInvalidCoord)
		}
		if txErr = s.opts.Repository.AppendLocationHistory(ctx, tx, models.LocationHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, LocationID: created.ID,
			ChangeType: models.ChangeCreated, NewStatus: created.Status,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.location.created", "crm_location", created.ID,
			nil, locationAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventLocationCreated, "crm_location", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID,
			locationEventPayload(created))
	})
	if err != nil {
		return models.Location{}, err
	}
	return created, nil
}

func validateLocationInput(input *CreateLocationInput) error {
	if !models.ValidLocationType(input.LocationType) {
		return badRequest(errorcodes.CustomerLocationNotFound, "locationType is not supported.")
	}
	if strings.TrimSpace(input.Label) == "" {
		return badRequest(errorcodes.CustomerLocationNotFound, "label is required.")
	}
	if err := input.Address.Validate(); err != nil {
		return badRequest(errorcodes.CustomerLocationNotFound, err.Error())
	}
	if err := models.ValidateTimezone(input.Timezone); err != nil {
		return badRequest(errorcodes.CustomerLocationInvalidTZ,
			"timezone must be a valid IANA timezone name, such as Africa/Lagos.")
	}
	if err := models.ValidateCoordinates(input.Latitude, input.Longitude); err != nil {
		return badRequest(errorcodes.CustomerLocationInvalidCoord, err.Error())
	}
	return nil
}

// GetLocation reads one location.
func (s *Service) GetLocation(ctx context.Context, t tenant.Context,
	customerID, locationID string) (models.Location, error) {
	if err := s.readEnabled(); err != nil {
		return models.Location{}, err
	}
	if _, err := s.requireCustomer(ctx, nil, t, customerID); err != nil {
		return models.Location{}, err
	}
	record, err := s.opts.Repository.GetLocation(ctx, nil, t.OrganizationID, customerID,
		locationID)
	return record, translate(err, "Location", errorcodes.CustomerLocationNotFound,
		errorcodes.CustomerLocationInvalidCoord)
}

// ListLocations returns a page of a customer's locations.
func (s *Service) ListLocations(ctx context.Context, t tenant.Context, customerID string,
	filter repositories.LocationFilter) ([]models.Location, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireCustomer(ctx, nil, t, customerID); err != nil {
		return nil, err
	}
	if filter.LocationType != "" && !models.ValidLocationType(filter.LocationType) {
		return nil, badRequest(errorcodes.CustomerLocationNotFound,
			"locationType filter is not supported.")
	}
	return s.opts.Repository.ListLocations(ctx, t.OrganizationID, customerID, filter)
}

// UpdateLocationInput carries a partial location update.
type UpdateLocationInput struct {
	LocationType       *string
	Label              *string
	Address            *models.Address
	Latitude           *float64
	Longitude          *float64
	ClearCoordinates   bool
	GeofenceReference  *string
	OperatingHours     models.JSONMap
	AccessInstructions *string
	SafetyInstructions *string
	ContactReference   *string
	Timezone           *string
	Status             *string
	ExpectedVersion    int64
	CorrelationID      string
	CausationID        string
}

// UpdateLocation applies a partial update to a non-archived location.
func (s *Service) UpdateLocation(ctx context.Context, t tenant.Context,
	customerID, locationID string, input UpdateLocationInput) (models.Location, error) {
	if err := s.writeEnabled(s.opts.Flags.LocationWritesEnabled); err != nil {
		return models.Location{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Location{}, err
	}
	if input.LocationType != nil && !models.ValidLocationType(*input.LocationType) {
		return models.Location{}, badRequest(errorcodes.CustomerLocationNotFound,
			"locationType is not supported.")
	}
	if input.Address != nil {
		if err := input.Address.Validate(); err != nil {
			return models.Location{}, badRequest(errorcodes.CustomerLocationNotFound,
				err.Error())
		}
	}
	if input.Timezone != nil {
		if err := models.ValidateTimezone(*input.Timezone); err != nil {
			return models.Location{}, badRequest(errorcodes.CustomerLocationInvalidTZ,
				"timezone must be a valid IANA timezone name, such as Africa/Lagos.")
		}
	}
	if !input.ClearCoordinates {
		if err := models.ValidateCoordinates(input.Latitude, input.Longitude); err != nil {
			return models.Location{}, badRequest(errorcodes.CustomerLocationInvalidCoord,
				err.Error())
		}
	}
	if input.Status != nil && *input.Status != models.RecordStatusActive &&
		*input.Status != models.RecordStatusInactive {
		return models.Location{}, badRequest(errorcodes.CustomerLocationNotFound,
			"status must be ACTIVE or INACTIVE; use the archive endpoint to archive.")
	}

	var updated models.Location
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, txErr := s.requireCustomer(ctx, tx, t, customerID); txErr != nil {
			return txErr
		}
		existing, txErr := s.opts.Repository.LockLocation(ctx, tx, t.OrganizationID,
			customerID, locationID)
		if txErr != nil {
			return translate(txErr, "Location", errorcodes.CustomerLocationNotFound,
				errorcodes.CustomerLocationInvalidCoord)
		}
		if existing.IsArchived() {
			return conflict(errorcodes.CustomerLocationNotFound,
				"An archived location cannot be modified.")
		}
		if input.ContactReference != nil && *input.ContactReference != "" {
			if _, txErr = s.opts.Repository.GetContact(ctx, tx, t.OrganizationID, customerID,
				*input.ContactReference); txErr != nil {
				return errorcodes.BadRequest(errorcodes.CustomerContactNotFound,
					"contactReference does not belong to this customer.")
			}
		}

		updated, txErr = s.opts.Repository.UpdateLocation(ctx, tx, t.OrganizationID,
			customerID, locationID, input.ExpectedVersion, repositories.LocationUpdate{
				LocationType: input.LocationType, Label: input.Label, Address: input.Address,
				Latitude: input.Latitude, Longitude: input.Longitude,
				ClearCoordinates:   input.ClearCoordinates,
				GeofenceReference:  input.GeofenceReference,
				OperatingHours:     input.OperatingHours,
				AccessInstructions: input.AccessInstructions,
				SafetyInstructions: input.SafetyInstructions,
				ContactReference:   input.ContactReference, Timezone: input.Timezone,
				Status: input.Status, UpdatedBy: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Location", errorcodes.CustomerLocationNotFound,
				errorcodes.CustomerLocationInvalidCoord)
		}
		if txErr = s.opts.Repository.AppendLocationHistory(ctx, tx, models.LocationHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, LocationID: locationID,
			ChangeType: models.ChangeUpdated, PreviousStatus: existing.Status,
			NewStatus: updated.Status, ChangedFields: changedLocationFields(input),
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.location.updated", "crm_location", locationID,
			locationAuditValues(existing), locationAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventLocationUpdated, "crm_location", locationID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			locationEventPayload(updated))
	})
	if err != nil {
		return models.Location{}, err
	}
	return updated, nil
}

func changedLocationFields(input UpdateLocationInput) []string {
	changed := make([]string, 0, 12)
	for name, present := range map[string]bool{
		"locationType":       input.LocationType != nil,
		"label":              input.Label != nil,
		"address":            input.Address != nil,
		"coordinates":        input.Latitude != nil || input.ClearCoordinates,
		"geofenceReference":  input.GeofenceReference != nil,
		"operatingHours":     input.OperatingHours != nil,
		"accessInstructions": input.AccessInstructions != nil,
		"safetyInstructions": input.SafetyInstructions != nil,
		"contactReference":   input.ContactReference != nil,
		"timezone":           input.Timezone != nil,
		"status":             input.Status != nil,
	} {
		if present {
			changed = append(changed, name)
		}
	}
	sortStrings(changed)
	return changed
}

// ArchiveLocation closes a location.
func (s *Service) ArchiveLocation(ctx context.Context, t tenant.Context,
	customerID, locationID string, expectedVersion int64,
	reason, idempotencyKey, correlationID string) (models.Location, error) {
	if err := s.writeEnabled(s.opts.Flags.LocationWritesEnabled); err != nil {
		return models.Location{}, err
	}
	if err := requireExpectedVersion(expectedVersion); err != nil {
		return models.Location{}, err
	}
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return models.Location{}, err
	}

	var archived models.Location
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, txErr := s.requireCustomer(ctx, tx, t, customerID); txErr != nil {
			return txErr
		}
		existing, txErr := s.opts.Repository.LockLocation(ctx, tx, t.OrganizationID,
			customerID, locationID)
		if txErr != nil {
			return translate(txErr, "Location", errorcodes.CustomerLocationNotFound,
				errorcodes.CustomerLocationInvalidCoord)
		}
		if existing.IsArchived() {
			archived = existing
			return nil
		}
		archived, txErr = s.opts.Repository.ArchiveLocation(ctx, tx, t.OrganizationID,
			customerID, locationID, expectedVersion, t.OrganizationUserID)
		if txErr != nil {
			return translate(txErr, "Location", errorcodes.CustomerLocationNotFound,
				errorcodes.CustomerLocationInvalidCoord)
		}
		if txErr = s.opts.Repository.AppendLocationHistory(ctx, tx, models.LocationHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, LocationID: locationID,
			ChangeType: models.ChangeArchived, PreviousStatus: existing.Status,
			NewStatus: archived.Status, ReasonCode: reason, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: correlationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.location.archived", "crm_location", locationID,
			locationAuditValues(existing), locationAuditValues(archived),
			reason); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventLocationArchived, "crm_location", locationID,
			archived.Version, t.OrganizationID, correlationID, locationEventPayload(archived))
	})
	if err != nil {
		return models.Location{}, err
	}
	return archived, nil
}

// locationAuditValues builds the audit projection.
//
// It records that a site exists, of what kind and where in the world, and
// deliberately omits the street address, the exact coordinates and the access
// and safety instructions. Those are the fields the sensitive-read permission
// exists to protect, and an audit trail must not be a way around it.
func locationAuditValues(location models.Location) map[string]any {
	return map[string]any{
		"locationType":          location.LocationType,
		"label":                 location.Label,
		"countryCode":           location.Address.CountryCode,
		"timezone":              location.Timezone,
		"status":                location.Status,
		"hasCoordinates":        location.Latitude != nil,
		"hasAccessInstructions": location.AccessInstructions != "",
		"hasSafetyInstructions": location.SafetyInstructions != "",
	}
}
