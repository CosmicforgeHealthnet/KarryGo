package app

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	retentionpolicyusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
)

// retentionDaysAdapter implements retentionexec.EffectiveRetentionDaysReader
// over retentionpolicy's own service, so internal/platform/retentionexec
// (a platform package) never imports a feature usecases package directly --
// same port/adapter convention every other cross-feature dependency in this
// composition root already uses (stepUpAdapter, documents.MediaPort, …).
type retentionDaysAdapter struct {
	policy *retentionpolicyusecases.Service
}

// EffectiveRetentionDays implements retentionexec.EffectiveRetentionDaysReader.
func (a retentionDaysAdapter) EffectiveRetentionDays(ctx context.Context, organizationID, class string) (int, error) {
	policy, err := a.policy.Get(ctx, organizationID, class)
	if err != nil {
		return 0, err
	}
	return policy.EffectiveDays(), nil
}

// retentionSupportAuthorityAdapter implements
// retentionexec.SupportCaseAuthorityReader over the SAME real
// engagement.SupportPort (supportCaseAdapter wrapping the real
// support-dispute-service HTTP client, section22_23_adapters.go) that
// engagementService and registerSection26Authorities already use for
// production -- no second Support Service client, no test double.
type retentionSupportAuthorityAdapter struct {
	port engagement.SupportPort
}

// CaseStatus implements retentionexec.SupportCaseAuthorityReader.
func (a retentionSupportAuthorityAdapter) CaseStatus(ctx context.Context, organizationID, supportServiceCaseID string) (string, error) {
	projection, err := a.port.Read(ctx, organizationID, supportServiceCaseID)
	if err != nil {
		return "", err
	}
	return projection.Status, nil
}

// retentionSynchronizationAuthorityAdapter implements
// retentionexec.ProjectionAuthorityReader over the SAME real
// *synchronization.Service (and its real, registerSection26Authorities-
// populated authority registry) the synchronization feature's own routes
// already use -- no second synchronization service, no test double.
// sourceService is accepted (matching the port's signature, which the
// retentionexec package deliberately keeps general) but not part of the
// lookup key: RegisterAuthority's real registry is keyed by event/
// projection type alone (see section26_adapters.go), not by source
// service, so this adapter mirrors that reality rather than pretending a
// finer-grained lookup exists.
type retentionSynchronizationAuthorityAdapter struct {
	service *synchronization.Service
}

// CurrentSourceVersion implements retentionexec.ProjectionAuthorityReader.
func (a retentionSynchronizationAuthorityAdapter) CurrentSourceVersion(ctx context.Context,
	organizationID, sourceService, projectionType, subjectReference string) (int64, error) {
	return a.service.AuthoritySourceVersion(ctx, projectionType, organizationID, subjectReference)
}
