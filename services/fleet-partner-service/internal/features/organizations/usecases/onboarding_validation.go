package usecases

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	repos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func fieldError(field, message string) error {
	return apperrors.Validation(message, []apperrors.FieldViolation{{Field: field, Message: message}})
}
func normalizeRepresentative(rep *models.Representative) error {
	rep.FullName = strings.TrimSpace(rep.FullName)
	rep.RoleTitle = strings.TrimSpace(rep.RoleTitle)
	if rep.FullName == "" || len(rep.FullName) > 200 {
		return fieldError("fullName", "Enter a full name of up to 200 characters.")
	}
	if rep.RoleTitle == "" || len(rep.RoleTitle) > 200 {
		return fieldError("roleTitle", "Enter a role title of up to 200 characters.")
	}
	email, _, err := contact.NormalizeEmail(rep.Email)
	if err != nil {
		return fieldError("email", "Enter a valid representative email address.")
	}
	rep.Email = email
	phone, _, err := contact.NormalizePhone(rep.Phone, "")
	if err != nil {
		return fieldError("phone", "Enter a valid phone number including its country code.")
	}
	rep.Phone = phone
	if !contains(models.SupportedAuthorityTypes, rep.AuthorityType) {
		return fieldError("authorityType", "Choose a supported authority type.")
	}
	if rep.Status != "ACTIVE" && rep.Status != "ENDED" {
		return fieldError("status", "Representative status must be ACTIVE or ENDED.")
	}
	return nil
}

// Every customer mutation locks the tenant root. Submission takes the same lock,
// so a concurrent save cannot change a submission's prerequisite data underneath it.
func (s *Service) lockEditable(ctx context.Context, tx pgx.Tx, organizationID string) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM fleet_partner_organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&status); err != nil {
		return err
	}
	if !AcceptsOnboardingEdits(status) {
		return apperrors.Conflict("This company is under review and cannot be edited. Return to the review page for its current status.", nil)
	}
	return nil
}
func (s *Service) transactionService(tx pgx.Tx) *Service {
	clone := *s
	clone.opts = s.opts
	clone.opts.Organizations = repos.NewOrganizationRepository(tx)
	clone.opts.Onboarding = repos.NewOnboardingRepository(tx)
	return &clone
}
func (s *Service) representativeByID(ctx context.Context, orgID, id string) (*models.Representative, error) {
	reps, err := s.opts.Onboarding.ListRepresentatives(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range reps {
		if reps[i].ID == id {
			return &reps[i], nil
		}
	}
	return nil, apperrors.NotFound("Representative not found.", nil)
}

// Identity linkage is verified against the organization's actual owner membership.
// A different person's contacts never become the owner's login credentials.
func linkRepresentative(ctx context.Context, tx pgx.Tx, orgID string, rep *models.Representative) error {
	if !rep.IsOnboardingOwner {
		rep.IdentityReference = nil
		return nil
	}
	if rep.IdentityReference == nil {
		return fieldError("isOnboardingOwner", "Choose yourself using your authenticated account, or select someone else.")
	}
	var owner bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_users WHERE organization_id=$1 AND identity_id=$2 AND is_owner AND status='ACTIVE')`, orgID, *rep.IdentityReference).Scan(&owner); err != nil {
		return err
	}
	if !owner {
		return apperrors.Forbidden("Only the company owner can select themselves as its onboarding owner representative.", nil)
	}
	return nil
}

func addressMissing(address map[string]any, prefix string) []string {
	missing := []string{}
	for _, key := range []string{"street", "city", "state", "country"} {
		// Read the prior address contract as well; no saved address is erased by renaming a key.
		value, _ := address[key].(string)
		if key == "street" && value == "" {
			value, _ = address["streetAddress"].(string)
			if value == "" {
				value, _ = address["addressLine1"].(string)
			}
		}
		if key == "country" && value == "" {
			value, _ = address["countryCode"].(string)
		}
		if strings.TrimSpace(value) == "" {
			missing = append(missing, prefix+"."+key)
		}
	}
	return missing
}
func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func profileMissing(profile *models.Profile) []string {
	if profile == nil {
		return []string{"registeredAddress", "operatingAddress", "businessEmail", "businessPhone"}
	}
	missing := append(addressMissing(profile.RegisteredAddress, "registeredAddress"), addressMissing(profile.OperatingAddress, "operatingAddress")...)
	if _, _, err := contact.NormalizeEmail(pointerValue(profile.BusinessEmail)); err != nil {
		missing = append(missing, "businessEmail")
	}
	if _, _, err := contact.NormalizePhone(pointerValue(profile.BusinessPhone), ""); err != nil {
		missing = append(missing, "businessPhone")
	}
	return missing
}
func validateProfile(profile *models.Profile) error {
	if profile.BusinessEmail != nil && *profile.BusinessEmail != "" {
		v, _, err := contact.NormalizeEmail(*profile.BusinessEmail)
		if err != nil {
			return fieldError("businessEmail", "Enter a valid business email address.")
		}
		profile.BusinessEmail = &v
	}
	if profile.BusinessPhone != nil && *profile.BusinessPhone != "" {
		v, _, err := contact.NormalizePhone(*profile.BusinessPhone, "")
		if err != nil {
			return fieldError("businessPhone", "Enter a business phone number including its country code.")
		}
		profile.BusinessPhone = &v
	}
	if profile.Website != nil && *profile.Website != "" {
		v, err := url.Parse(*profile.Website)
		if err != nil || (v.Scheme != "https" && v.Scheme != "http") || v.Host == "" {
			return fieldError("website", "Enter a complete website address starting with https://.")
		}
	}
	return nil
}

// assessStages derives customer completion from persisted records, not client flags.
// Internal verification, wallet and activation stages remain owned by their services.
func (s *Service) assessStages(ctx context.Context, org *models.Organization, stages []models.OnboardingStage) ([]models.OnboardingStage, error) {
	profile, err := s.opts.Onboarding.GetProfile(ctx, org.ID)
	if err != nil && !errors.Is(err, repos.ErrNotFound) {
		return nil, err
	}
	reps, err := s.opts.Onboarding.ListRepresentatives(ctx, org.ID)
	if err != nil {
		return nil, err
	}
	caps, err := s.opts.Onboarding.ListCapabilities(ctx, org.ID)
	if err != nil {
		return nil, err
	}
	docs, err := s.opts.Onboarding.ListDocuments(ctx, org.ID)
	if err != nil {
		return nil, err
	}
	missing := map[string][]string{models.StageBusinessProfile: {}, models.StageHeadquarters: profileMissing(profile), models.StageAuthorizedRepresentative: {"authorizedRepresentative"}, models.StageOperatingCapabilities: {}, models.StageBusinessDocuments: {}}
	for key, value := range map[string]string{"legalName": org.LegalName, "registrationNumber": org.RegistrationNumber, "organizationType": org.OrganizationType, "countryCode": org.CountryCode, "timezone": org.Timezone, "defaultCurrency": org.DefaultCurrency} {
		if strings.TrimSpace(value) == "" {
			missing[models.StageBusinessProfile] = append(missing[models.StageBusinessProfile], key)
		}
	}
	for _, rep := range reps {
		if rep.Status == "ACTIVE" && (rep.EffectiveTo == nil || rep.EffectiveTo.After(s.opts.Now())) && normalizeRepresentative(&rep) == nil {
			missing[models.StageAuthorizedRepresentative] = []string{}
			break
		}
	}
	if len(caps) == 0 {
		missing[models.StageOperatingCapabilities] = []string{"capabilities"}
	}
	for _, kind := range models.RequiredDocumentTypes {
		found := false
		for _, doc := range docs {
			if doc.DocumentType == kind && doc.Status == models.DocumentStatusReady && (doc.ExpiresAt == nil || doc.ExpiresAt.After(s.opts.Now())) {
				found = true
				break
			}
		}
		if !found {
			missing[models.StageBusinessDocuments] = append(missing[models.StageBusinessDocuments], "documents."+kind)
		}
	}
	for i := range stages {
		if fields, ok := missing[stages[i].Stage]; ok {
			stages[i].MissingFields = fields
			stages[i].Status = models.StageStatusInProgress
			if len(fields) == 0 {
				stages[i].Status = models.StageStatusComplete
			} else {
				stages[i].CompletedAt = nil
			}
		}
	}
	return stages, nil
}

// RemoveDocument retains the file and document record, soft-removing only this
// tenant's attachment. Media projection events cannot resurrect removed records.
func (s *Service) RemoveDocument(ctx context.Context, t tenant.Context, id string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.lockEditable(ctx, tx, t.OrganizationID); err != nil {
			return err
		}
		if err := s.opts.Onboarding.RemoveDocument(ctx, tx, t.OrganizationID, id); errors.Is(err, repos.ErrNotFound) {
			return apperrors.NotFound("Document not found.", nil)
		} else if err != nil {
			return err
		}
		if err := s.opts.Onboarding.UpsertStage(ctx, tx, t.OrganizationID, models.StageBusinessDocuments, models.StageStatusInProgress, nil, nil); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(t, "organization.document_removed", "organization_document", id)
		return s.opts.Audit.Record(ctx, tx, entry)
	})
}

func mediaUploadError(err error) error {
	var remote *platformclients.Error
	if errors.As(err, &remote) {
		switch remote.Outcome {
		case platformclients.OutcomeValidation:
			return fieldError("file", "The file did not pass Media Service validation. Check its type, size and contents.")
		case platformclients.OutcomeConflict:
			return apperrors.Conflict("The upload authorization is no longer usable. Select the file again to request a new authorization.", nil)
		}
	}
	return errorcodes.Unavailable(errorcodes.OrganizationOnboardingIncomplete, "The document service is unavailable. Your saved documents are unchanged; try again shortly.")
}
