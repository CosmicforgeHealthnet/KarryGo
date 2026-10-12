package promotions

import "cosmicforge/logistics/services/vendor-service/internal/features/production"

func canReadCampaign(scope production.Scope, c Campaign) bool {
	if scope.Global {
		return true
	}
	for _, target := range c.Scopes {
		if target.Type == "BRANCH" && scope.Covers(target.Ref) {
			return true
		}
	}
	return false
}

func canManageCampaign(scope production.Scope, c Campaign) bool {
	if scope.Global {
		return true
	}
	hasBranch := false
	for _, target := range c.Scopes {
		if target.Type == "BRANCH" {
			if !scope.Covers(target.Ref) {
				return false
			}
			hasBranch = true
		}
	}
	return hasBranch
}
