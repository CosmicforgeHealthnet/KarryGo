package http

import (
	"github.com/gin-gonic/gin"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
)

// providerMembership aliases the provider domain model for view helpers.
type providerMembership = providermodels.Membership

// providerViews renders provider memberships for corporate consumers.
//
// Provider-facing and organization-facing payloads never carry customer fare,
// company settlement, commission, margin or contract price — none of those
// values exist in this service at all.
func providerViews(memberships []providerMembership) []gin.H {
	views := make([]gin.H, 0, len(memberships))
	for _, membership := range memberships {
		views = append(views, gin.H{
			"membershipId":     membership.ID,
			"serviceCategory":  membership.ServiceCategory,
			"providerService":  membership.ProviderService,
			"providerId":       membership.ProviderID,
			"onboardingSource": membership.OnboardingSource,
			"relationshipType": membership.RelationshipType,
			"homeBranchId":     membership.HomeBranchID,
			"status":           membership.Status,
			"joinedAt":         membership.JoinedAt,
			"version":          membership.Version,
		})
	}
	return views
}
