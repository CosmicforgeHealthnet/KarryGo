package usecases

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ErrRetentionVersionConflict is what a PrivacyRetention store returns when a
// class changed after it was read: the whole patch is refused.
var ErrRetentionVersionConflict = errors.New("governance: retention policy changed concurrently")

// PrivacyRetention is the slice of the retention-policy store privacy
// settings read and write (spec 59.10). The composition root adapts the one
// authoritative retention store; governance never stores retention itself.
type PrivacyRetention interface {
	// Read returns each requested class's stored policy (platform minimum and
	// version 0 when the organization never overrode it). q is nil outside a
	// transaction.
	Read(ctx context.Context, q pgx.Tx, organizationID string, classes []string) (map[string]models.RetentionValue, error)
	// Write stores one class's days, refusing with ErrRetentionVersionConflict
	// unless its version still equals expectedVersion.
	Write(ctx context.Context, tx pgx.Tx, organizationID, class string, days int, expectedVersion int64) error
	// Lock serializes privacy-settings writes for one organization.
	Lock(ctx context.Context, tx pgx.Tx, organizationID string) error
	// ExecutionAvailable reports whether a retention executor is registered
	// for the class in this deployment.
	ExecutionAvailable(class string) bool
}

// WithPrivacyRetention attaches the retention store. It is set by the
// composition root once the retention executors exist.
func (s *Service) WithPrivacyRetention(p PrivacyRetention) *Service {
	s.o.PrivacyRetention = p
	return s
}

func (s *Service) privacyReady() error {
	if s.o.PrivacyRetention == nil {
		return apperrors.Unavailable("Privacy settings are unavailable.", nil)
	}
	return nil
}

// GetPrivacySettings returns the organization's privacy settings.
func (s *Service) GetPrivacySettings(ctx context.Context, t tenant.Context) (models.PrivacySettings, error) {
	if err := s.gate(s.o.Flags.PrivacySettings, false); err != nil {
		return models.PrivacySettings{}, err
	}
	if err := s.privacyReady(); err != nil {
		return models.PrivacySettings{}, err
	}
	return s.readPrivacySettings(ctx, nil, t.OrganizationID)
}

func (s *Service) readPrivacySettings(ctx context.Context, tx pgx.Tx, organizationID string) (models.PrivacySettings, error) {
	classes := append(append([]string{}, models.ConfigurableRetentionClasses...), models.ProtectedRetentionClasses...)
	values, err := s.o.PrivacyRetention.Read(ctx, tx, organizationID, classes)
	if err != nil {
		return models.PrivacySettings{}, err
	}
	out := models.PrivacySettings{OrganizationID: organizationID, LegalHoldsOverrideRetention: true,
		Retention: map[string]models.RetentionSetting{}, ProtectedRetention: map[string]models.RetentionSetting{}}
	for _, class := range models.ConfigurableRetentionClasses {
		v := values[class]
		out.Version += v.Version
		out.Retention[class] = models.RetentionSetting{RetentionDays: v.RetentionDays, PlatformMinimumDays: v.PlatformMinimumDays,
			Configurable: true, ExecutionAvailable: s.o.PrivacyRetention.ExecutionAvailable(class)}
	}
	for _, class := range models.ProtectedRetentionClasses {
		v := values[class]
		out.ProtectedRetention[class] = models.RetentionSetting{RetentionDays: v.RetentionDays, PlatformMinimumDays: v.PlatformMinimumDays,
			ExecutionAvailable: s.o.PrivacyRetention.ExecutionAvailable(class)}
	}
	return out, nil
}

func isConfigurableClass(class string) bool {
	for _, c := range models.ConfigurableRetentionClasses {
		if c == class {
			return true
		}
	}
	return false
}

func isProtectedClass(class string) bool {
	for _, c := range models.ProtectedRetentionClasses {
		if c == class {
			return true
		}
	}
	return false
}

// maxRetentionDays bounds a configured value (100 years), so an obviously
// mistyped duration is refused rather than stored.
const maxRetentionDays = 36500

// UpdatePrivacySettings applies a merge patch to the retention settings. The
// whole patch applies or none of it does; If-Match must carry the current
// version.
func (s *Service) UpdatePrivacySettings(ctx context.Context, t tenant.Context, in models.PrivacySettingsPatch) (models.PrivacySettings, error) {
	if err := s.gate(s.o.Flags.PrivacySettings, true); err != nil {
		return models.PrivacySettings{}, err
	}
	if err := s.privacyReady(); err != nil {
		return models.PrivacySettings{}, err
	}
	if len(t.PermittedBranchIDs) > 0 {
		return models.PrivacySettings{}, apperrors.Forbidden("Organization-wide access is required to change privacy settings.", nil)
	}
	if in.ExpectedVersion < 0 {
		// The handler passes -1 when If-Match is absent: version 0 is a real
		// version (nothing overridden yet), so absence must be distinct.
		return models.PrivacySettings{}, apperrors.BadRequest("If-Match must carry the current privacy-settings version.", nil)
	}
	if len(in.Retention) == 0 {
		return models.PrivacySettings{}, apperrors.Validation("Nothing to change: provide retention settings.", nil)
	}
	var fields []apperrors.FieldViolation
	classes := make([]string, 0, len(in.Retention))
	for class, p := range in.Retention {
		switch {
		case isProtectedClass(class):
			fields = append(fields, apperrors.FieldViolation{Field: "retention." + class, Message: "This retention class is protected and cannot be changed."})
		case !isConfigurableClass(class):
			fields = append(fields, apperrors.FieldViolation{Field: "retention." + class, Message: "Unknown retention class."})
		case p.RetentionDays == nil:
			fields = append(fields, apperrors.FieldViolation{Field: "retention." + class + ".retentionDays", Message: "retentionDays is required."})
		case *p.RetentionDays > maxRetentionDays:
			fields = append(fields, apperrors.FieldViolation{Field: "retention." + class + ".retentionDays", Message: fmt.Sprintf("retentionDays cannot exceed %d.", maxRetentionDays)})
		default:
			classes = append(classes, class)
		}
	}
	if len(fields) > 0 {
		sort.Slice(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
		return models.PrivacySettings{}, apperrors.Validation("Check the privacy settings.", fields)
	}
	sort.Strings(classes)

	var result models.PrivacySettings
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.PrivacyRetention.Lock(ctx, tx, t.OrganizationID); err != nil {
			return err
		}
		current, err := s.readPrivacySettings(ctx, tx, t.OrganizationID)
		if err != nil {
			return err
		}
		if in.ExpectedVersion != current.Version {
			return apperrors.Conflict("Privacy settings changed since they were read; reload and retry.", nil)
		}
		stored, err := s.o.PrivacyRetention.Read(ctx, tx, t.OrganizationID, classes)
		if err != nil {
			return err
		}
		changed := map[string]any{}
		previous := map[string]any{}
		for _, class := range classes {
			days := *in.Retention[class].RetentionDays
			v := stored[class]
			if days < v.PlatformMinimumDays {
				return apperrors.Validation("Check the privacy settings.", []apperrors.FieldViolation{{Field: "retention." + class + ".retentionDays",
					Message: fmt.Sprintf("retentionDays cannot be below the platform minimum of %d.", v.PlatformMinimumDays)}})
			}
			if days == v.RetentionDays {
				continue
			}
			if err := s.o.PrivacyRetention.Write(ctx, tx, t.OrganizationID, class, days, v.Version); err != nil {
				if errors.Is(err, ErrRetentionVersionConflict) {
					return apperrors.Conflict("Privacy settings changed since they were read; reload and retry.", err)
				}
				return err
			}
			previous[class] = v.RetentionDays
			changed[class] = days
		}
		result, err = s.readPrivacySettings(ctx, tx, t.OrganizationID)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			return nil
		}
		return s.recordKind(ctx, tx, t, "privacy_settings.updated.v1", "privacy_settings", t.OrganizationID, result.Version,
			map[string]any{"organizationId": t.OrganizationID, "retention": changed, "previousRetention": previous, "version": result.Version,
				"actorId": strings.TrimSpace(t.ActorID)})
	})
	if err != nil {
		return models.PrivacySettings{}, err
	}
	return result, nil
}
