package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// DefaultPhoneCountryCode is used when a caller supplies a national number
// without a country prefix.
const DefaultPhoneCountryCode = "+234"

// CreateContactInput opens a customer contact.
type CreateContactInput struct {
	FullName                string
	RoleTitle               string
	Email                   string
	Phone                   string
	PreferredChannel        string
	ContactPurpose          string
	IsPrimary               bool
	NotificationPreferences models.JSONMap
	ConsentStatus           string
	ConsentSource           string
	ExpectedVersion         int64
	IdempotencyKey          string
	CorrelationID           string
	CausationID             string
}

// CreateContact adds a named person at a customer.
//
// A contact is not a login identity: it holds no credential, grants no access
// and is never promoted into an account. It exists so a quotation can be sent
// to somebody and an escalation can reach somebody.
func (s *Service) CreateContact(ctx context.Context, t tenant.Context, customerID string,
	input CreateContactInput) (models.Contact, error) {
	if err := s.writeEnabled(s.opts.Flags.ContactWritesEnabled); err != nil {
		return models.Contact{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Contact{}, err
	}
	normalized, err := s.normalizeContactInput(&input)
	if err != nil {
		return models.Contact{}, err
	}

	var created models.Contact
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, customerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot receive new contacts.")
		}

		record := models.Contact{
			ID:                      ids.New(repositories.PrefixContact),
			OrganizationID:          t.OrganizationID,
			CustomerID:              customerID,
			FullName:                strings.TrimSpace(input.FullName),
			RoleTitle:               strings.TrimSpace(input.RoleTitle),
			NormalizedEmail:         normalized.EmailNormalized,
			NormalizedPhone:         normalized.PhoneE164,
			PreferredChannel:        input.PreferredChannel,
			ContactPurpose:          input.ContactPurpose,
			IsPrimary:               input.IsPrimary,
			NotificationPreferences: input.NotificationPreferences,
			ConsentStatus:           input.ConsentStatus,
			ConsentSource:           strings.TrimSpace(input.ConsentSource),
			Status:                  models.RecordStatusActive,
			CreatedBy:               t.OrganizationUserID,
		}
		if record.ConsentStatus != models.ConsentUnknown {
			now := s.now()
			record.ConsentRecordedAt = &now
		}
		if input.IsPrimary {
			// Demote first so the partial unique index has room. Two concurrent
			// creates both asking to be primary still resolve to one winner:
			// the index refuses the second.
			if txErr = s.opts.Repository.DemotePrimaryContacts(ctx, tx, t.OrganizationID,
				customerID, record.ContactPurpose, record.ID); txErr != nil {
				return txErr
			}
		}

		created, txErr = s.opts.Repository.CreateContact(ctx, tx, record)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return conflict(errorcodes.CustomerContactConflict,
					"Another contact already holds this role for the customer.")
			}
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if txErr = s.opts.Repository.AppendContactHistory(ctx, tx, models.ContactHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, ContactID: created.ID,
			ChangeType: models.ChangeCreated, NewStatus: created.Status,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contact.created", "crm_contact", created.ID,
			nil, contactAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContactCreated, "crm_contact", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID,
			contactEventPayload(created))
	})
	if err != nil {
		return models.Contact{}, err
	}
	return created, nil
}

// normalizeContactInput validates and canonicalizes the contact details.
//
// Normalization happens once, here, so a customer entered as "Ops@Example.COM "
// and one entered as "ops@example.com" are the same contact for deduplication,
// masking and delivery. The platform normalizer is reused rather than
// reimplemented.
func (s *Service) normalizeContactInput(input *CreateContactInput) (contact.Pair, error) {
	if strings.TrimSpace(input.FullName) == "" {
		return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
			"fullName is required.")
	}
	if input.PreferredChannel == "" {
		input.PreferredChannel = models.ChannelEmail
	}
	if !models.ValidChannel(input.PreferredChannel) {
		return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
			"preferredChannel is not supported.")
	}
	if input.ContactPurpose == "" {
		input.ContactPurpose = models.PurposeGeneral
	}
	if !models.ValidContactPurpose(input.ContactPurpose) {
		return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
			"contactPurpose is not supported.")
	}
	if input.ConsentStatus == "" {
		input.ConsentStatus = models.ConsentUnknown
	}
	if !models.ValidConsentStatus(input.ConsentStatus) {
		return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
			"consentStatus is not supported.")
	}

	// Each half is normalized independently and only when supplied. The
	// platform's NormalizePair requires both, because an authentication
	// identity is an email *and* a phone; a CRM contact is a person at a
	// customer, and many of them are reachable by exactly one of the two.
	var pair contact.Pair
	if strings.TrimSpace(input.Email) != "" {
		normalized, display, err := contact.NormalizeEmail(input.Email)
		if err != nil {
			return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
				"The email address is not valid.")
		}
		pair.EmailNormalized, pair.EmailDisplay = normalized, display
	}
	if strings.TrimSpace(input.Phone) != "" {
		normalized, display, err := contact.NormalizePhone(input.Phone,
			DefaultPhoneCountryCode)
		if err != nil {
			return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
				"The phone number is not valid.")
		}
		pair.PhoneE164, pair.PhoneDisplay = normalized, display
	}
	if pair.EmailNormalized == "" && pair.PhoneE164 == "" && input.PreferredChannel != models.ChannelNone {
		return contact.Pair{}, badRequest(errorcodes.CustomerContactConflict,
			"A contact needs an email address or a phone number unless the preferred channel is NONE.")
	}
	return pair, nil
}

// GetContact reads one contact.
func (s *Service) GetContact(ctx context.Context, t tenant.Context,
	customerID, contactID string) (models.Contact, error) {
	if err := s.readEnabled(); err != nil {
		return models.Contact{}, err
	}
	if _, err := s.requireCustomer(ctx, nil, t, customerID); err != nil {
		return models.Contact{}, err
	}
	record, err := s.opts.Repository.GetContact(ctx, nil, t.OrganizationID, customerID, contactID)
	return record, translate(err, "Contact", errorcodes.CustomerContactNotFound,
		errorcodes.CustomerContactConflict)
}

// ListContacts returns a page of a customer's contacts.
func (s *Service) ListContacts(ctx context.Context, t tenant.Context, customerID string,
	filter repositories.ContactFilter) ([]models.Contact, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireCustomer(ctx, nil, t, customerID); err != nil {
		return nil, err
	}
	if filter.ContactPurpose != "" && !models.ValidContactPurpose(filter.ContactPurpose) {
		return nil, badRequest(errorcodes.CustomerContactConflict,
			"contactPurpose filter is not supported.")
	}
	return s.opts.Repository.ListContacts(ctx, t.OrganizationID, customerID, filter)
}

// UpdateContactInput carries a partial contact update.
type UpdateContactInput struct {
	FullName                *string
	RoleTitle               *string
	Email                   *string
	Phone                   *string
	PreferredChannel        *string
	ContactPurpose          *string
	NotificationPreferences models.JSONMap
	ConsentStatus           *string
	ConsentSource           *string
	Status                  *string
	ExpectedVersion         int64
	CorrelationID           string
	CausationID             string
}

// UpdateContact applies a partial update to a non-archived contact.
func (s *Service) UpdateContact(ctx context.Context, t tenant.Context,
	customerID, contactID string, input UpdateContactInput) (models.Contact, error) {
	if err := s.writeEnabled(s.opts.Flags.ContactWritesEnabled); err != nil {
		return models.Contact{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Contact{}, err
	}
	update := repositories.ContactUpdate{
		FullName: input.FullName, RoleTitle: input.RoleTitle,
		NotificationPreferences: input.NotificationPreferences,
		ConsentSource:           input.ConsentSource,
		UpdatedBy:               t.OrganizationUserID,
	}
	if input.PreferredChannel != nil {
		if !models.ValidChannel(*input.PreferredChannel) {
			return models.Contact{}, badRequest(errorcodes.CustomerContactConflict,
				"preferredChannel is not supported.")
		}
		update.PreferredChannel = input.PreferredChannel
	}
	if input.ContactPurpose != nil {
		if !models.ValidContactPurpose(*input.ContactPurpose) {
			return models.Contact{}, badRequest(errorcodes.CustomerContactConflict,
				"contactPurpose is not supported.")
		}
		update.ContactPurpose = input.ContactPurpose
	}
	if input.ConsentStatus != nil {
		if !models.ValidConsentStatus(*input.ConsentStatus) {
			return models.Contact{}, badRequest(errorcodes.CustomerContactConflict,
				"consentStatus is not supported.")
		}
		update.ConsentStatus = input.ConsentStatus
	}
	if input.Status != nil {
		if *input.Status != models.RecordStatusActive &&
			*input.Status != models.RecordStatusInactive {
			return models.Contact{}, badRequest(errorcodes.CustomerContactConflict,
				"status must be ACTIVE or INACTIVE; use the archive endpoint to archive.")
		}
		update.Status = input.Status
	}
	if input.Email != nil {
		normalized, _, err := contact.NormalizeEmail(*input.Email)
		if err != nil {
			return models.Contact{}, badRequest(errorcodes.CustomerContactConflict,
				"The email address is not valid.")
		}
		update.NormalizedEmail = &normalized
	}
	if input.Phone != nil {
		normalized, _, err := contact.NormalizePhone(*input.Phone, DefaultPhoneCountryCode)
		if err != nil {
			return models.Contact{}, badRequest(errorcodes.CustomerContactConflict,
				"The phone number is not valid.")
		}
		update.NormalizedPhone = &normalized
	}

	var updated models.Contact
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, txErr := s.requireCustomer(ctx, tx, t, customerID); txErr != nil {
			return txErr
		}
		existing, txErr := s.opts.Repository.LockContact(ctx, tx, t.OrganizationID,
			customerID, contactID)
		if txErr != nil {
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if existing.IsArchived() {
			return conflict(errorcodes.CustomerContactConflict,
				"An archived contact cannot be modified.")
		}
		updated, txErr = s.opts.Repository.UpdateContact(ctx, tx, t.OrganizationID,
			customerID, contactID, input.ExpectedVersion, update)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return conflict(errorcodes.CustomerContactConflict,
					"Another contact already holds this role for the customer.")
			}
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if txErr = s.opts.Repository.AppendContactHistory(ctx, tx, models.ContactHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, ContactID: contactID,
			ChangeType: models.ChangeUpdated, PreviousStatus: existing.Status,
			NewStatus: updated.Status, ChangedFields: changedContactFields(input),
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contact.updated", "crm_contact", contactID,
			contactAuditValues(existing), contactAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContactUpdated, "crm_contact", contactID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			contactEventPayload(updated))
	})
	if err != nil {
		return models.Contact{}, err
	}
	return updated, nil
}

func changedContactFields(input UpdateContactInput) []string {
	changed := make([]string, 0, 9)
	for name, present := range map[string]bool{
		"fullName":                input.FullName != nil,
		"roleTitle":               input.RoleTitle != nil,
		"email":                   input.Email != nil,
		"phone":                   input.Phone != nil,
		"preferredChannel":        input.PreferredChannel != nil,
		"contactPurpose":          input.ContactPurpose != nil,
		"notificationPreferences": input.NotificationPreferences != nil,
		"consentStatus":           input.ConsentStatus != nil,
		"status":                  input.Status != nil,
	} {
		if present {
			changed = append(changed, name)
		}
	}
	sortStrings(changed)
	return changed
}

// ArchiveContact closes a contact.
//
// Archived contacts stay readable: an issued quotation version snapshotted this
// contact, and a snapshot that pointed at a deleted row would be a record with
// a hole in it.
func (s *Service) ArchiveContact(ctx context.Context, t tenant.Context,
	customerID, contactID string, expectedVersion int64,
	reason, idempotencyKey, correlationID string) (models.Contact, error) {
	if err := s.writeEnabled(s.opts.Flags.ContactWritesEnabled); err != nil {
		return models.Contact{}, err
	}
	if err := requireExpectedVersion(expectedVersion); err != nil {
		return models.Contact{}, err
	}
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return models.Contact{}, err
	}

	var archived models.Contact
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, txErr := s.requireCustomer(ctx, tx, t, customerID); txErr != nil {
			return txErr
		}
		existing, txErr := s.opts.Repository.LockContact(ctx, tx, t.OrganizationID,
			customerID, contactID)
		if txErr != nil {
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if existing.IsArchived() {
			archived = existing
			return nil
		}
		archived, txErr = s.opts.Repository.ArchiveContact(ctx, tx, t.OrganizationID,
			customerID, contactID, expectedVersion, t.OrganizationUserID)
		if txErr != nil {
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if txErr = s.opts.Repository.AppendContactHistory(ctx, tx, models.ContactHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, ContactID: contactID,
			ChangeType: models.ChangeArchived, PreviousStatus: existing.Status,
			NewStatus: archived.Status, ReasonCode: reason, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: correlationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contact.archived", "crm_contact", contactID,
			contactAuditValues(existing), contactAuditValues(archived), reason); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContactArchived, "crm_contact", contactID,
			archived.Version, t.OrganizationID, correlationID, contactEventPayload(archived))
	})
	if err != nil {
		return models.Contact{}, err
	}
	return archived, nil
}

// MakeContactPrimary promotes a contact to primary for its purpose.
//
// Two concurrent promotions cannot both win: the demote and the promote happen
// inside one transaction, and the partial unique index on
// (organization, customer, purpose) WHERE is_primary is what actually decides
// the race.
func (s *Service) MakeContactPrimary(ctx context.Context, t tenant.Context,
	customerID, contactID string, expectedVersion int64,
	idempotencyKey, correlationID string) (models.Contact, error) {
	if err := s.writeEnabled(s.opts.Flags.ContactWritesEnabled); err != nil {
		return models.Contact{}, err
	}
	if err := requireExpectedVersion(expectedVersion); err != nil {
		return models.Contact{}, err
	}
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return models.Contact{}, err
	}

	var promoted models.Contact
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, txErr := s.requireCustomer(ctx, tx, t, customerID); txErr != nil {
			return txErr
		}
		existing, txErr := s.opts.Repository.LockContact(ctx, tx, t.OrganizationID,
			customerID, contactID)
		if txErr != nil {
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if existing.IsArchived() || existing.Status != models.RecordStatusActive {
			return conflict(errorcodes.CustomerContactConflict,
				"Only an active contact can be made primary.")
		}
		if existing.IsPrimary {
			promoted = existing
			return nil
		}
		if txErr = s.opts.Repository.DemotePrimaryContacts(ctx, tx, t.OrganizationID,
			customerID, existing.ContactPurpose, contactID); txErr != nil {
			return txErr
		}
		promoted, txErr = s.opts.Repository.PromoteContact(ctx, tx, t.OrganizationID,
			customerID, contactID, expectedVersion, t.OrganizationUserID)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return conflict(errorcodes.CustomerContactConflict,
					"Another contact was made primary for this purpose at the same time.")
			}
			return translate(txErr, "Contact", errorcodes.CustomerContactNotFound,
				errorcodes.CustomerContactConflict)
		}
		if txErr = s.opts.Repository.AppendContactHistory(ctx, tx, models.ContactHistory{
			OrganizationID: t.OrganizationID, CustomerID: customerID, ContactID: contactID,
			ChangeType: models.ChangePrimaryChanged, PreviousStatus: existing.Status,
			NewStatus: promoted.Status, ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: correlationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.contact.made_primary", "crm_contact", contactID,
			map[string]any{"isPrimary": existing.IsPrimary},
			map[string]any{"isPrimary": promoted.IsPrimary}, ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventContactUpdated, "crm_contact", contactID,
			promoted.Version, t.OrganizationID, correlationID, contactEventPayload(promoted))
	})
	if err != nil {
		return models.Contact{}, err
	}
	return promoted, nil
}

// contactAuditValues builds the audit projection.
//
// No email and no phone number: an audit trail that copies contact details is a
// second, less guarded, copy of the PII it exists to account for.
func contactAuditValues(record models.Contact) map[string]any {
	return map[string]any{
		"contactPurpose":   record.ContactPurpose,
		"preferredChannel": record.PreferredChannel,
		"isPrimary":        record.IsPrimary,
		"status":           record.Status,
		"consentStatus":    record.ConsentStatus,
		"hasEmail":         record.NormalizedEmail != "",
		"hasPhone":         record.NormalizedPhone != "",
	}
}

// primaryContactFor resolves the contact a CRM notification should reach.
//
// It prefers the primary contact for the requested purpose and falls back to
// the general primary. A customer with no contactable primary yields an empty
// contact, and the caller records no notification intent rather than queueing
// one that can only ever dead-letter.
func (s *Service) primaryContactFor(ctx context.Context, q database.Querier,
	organizationID, customerID, purpose string) models.Contact {
	for _, candidate := range []string{purpose, models.PurposeGeneral} {
		if candidate == "" {
			continue
		}
		found, err := s.opts.Repository.PrimaryContact(ctx, q, organizationID, customerID,
			candidate)
		if err == nil && found.Contactable() {
			return found
		}
	}
	return models.Contact{}
}
