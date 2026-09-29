package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// ValidateMediaUploadSessions refuses to start with the N12 upload capability
// enabled but its Media authority, public redemption origin, Support source
// authority or signing secret missing. Names only, never values.
func (c Config) ValidateMediaUploadSessions() error {
	if !c.MediaUploadSessionEnabled {
		return nil
	}
	var problems []string
	for name, value := range map[string]string{"CUSTOMER_MEDIA_BASE_URL": c.MediaBaseURL, "CUSTOMER_MEDIA_PUBLIC_BASE_URL": c.MediaPublicBaseURL, "CUSTOMER_SUPPORT_BASE_URL": c.SupportBaseURL} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			problems = append(problems, name+" must be an HTTP(S) origin")
		}
	}
	if len(c.MediaPrivateSecret) == 0 {
		problems = append(problems, "CUSTOMER_MEDIA_PRIVATE_SECRET is required")
	}
	if len(problems) > 0 {
		return fmt.Errorf("media upload session configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ValidateJourneySharing rejects enabled but incomplete/malformed deployment
// boundaries. Errors contain setting names only, never credential values.
func (c Config) ValidateJourneySharing() error {
	if !c.ShareJourneyEnabled {
		return nil
	}
	var problems []string
	for name, value := range map[string]string{"CUSTOMER_SHARE_TAXI_URL": c.ShareTaxiURL, "CUSTOMER_SHARE_DISPATCH_URL": c.ShareDispatchURL, "CUSTOMER_SHARE_HAULING_URL": c.ShareHaulingURL, "CUSTOMER_SHARE_VENDOR_URL": c.ShareVendorURL, "CUSTOMER_SHARE_VIEWER_ORIGIN": c.ShareViewerOrigin} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			problems = append(problems, name+" must be a bare HTTP(S) origin")
		}
	}
	for name, secret := range map[string][]byte{"CUSTOMER_SHARE_TAXI_SECRET": c.ShareTaxiSecret, "CUSTOMER_SHARE_DISPATCH_SECRET": c.ShareDispatchSecret, "CUSTOMER_SHARE_HAULING_SECRET": c.ShareHaulingSecret, "CUSTOMER_SHARE_VENDOR_SECRET": c.ShareVendorSecret} {
		if len(secret) == 0 {
			problems = append(problems, name+" is required")
		}
	}
	if c.ShareJourneyTTL <= 0 || c.ShareJourneyTTL > 30*24*time.Hour {
		problems = append(problems, "CUSTOMER_SHARE_JOURNEY_TTL_SECONDS must be positive and at most 30 days")
	}
	if c.ShareCompletedRetention <= 0 || c.ShareCompletedRetention > c.ShareJourneyTTL {
		problems = append(problems, "CUSTOMER_SHARE_COMPLETED_RETENTION_SECONDS must be positive and no longer than TTL")
	}
	for _, cidr := range c.ShareTrustedGatewayCIDRs {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			problems = append(problems, "CUSTOMER_SHARE_TRUSTED_GATEWAY_CIDRS contains an invalid prefix")
			break
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("journey sharing configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}
