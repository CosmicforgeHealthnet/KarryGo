package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	AppEnv               string
	HTTPAddr             string
	DatabaseURL          string
	Migration            bool
	PrivateAPIEnabled    bool
	ServiceSecrets       string
	AllowedCallers       string
	VendorCallbackURL    string
	VendorCallbackSecret string

	// AssetCapabilityCertificationEnabled is Section 12's kill switch
	// (ASSET_CAPABILITY_CERTIFICATION_ENABLED). It gates WRITES only: opening a
	// case, recording a decision, registering a document. Reads stay open while
	// it is false, because an operational service asking "is this asset
	// certified?" during a rollout pause must keep getting the true stored
	// answer -- a disabled flag that hid existing certifications would make
	// certification-gated work look ungated, which is the one failure mode a
	// safety flag must never have.
	AssetCapabilityCertificationEnabled bool

	// Per-authority certification callback endpoints. Each is a bare origin;
	// the path is fixed per authority because the three services mount their
	// internal groups differently and deriving one path from a base URL would
	// have to guess which.
	TaxiCallbackURL        string
	TaxiCallbackSecret     string
	DispatchCallbackURL    string
	DispatchCallbackSecret string
	HaulingCallbackURL     string
	HaulingCallbackSecret  string

	// Media is where evidence bytes live. Without it, cited evidence cannot be
	// bound to the asset it claims to describe, so citations are refused rather
	// than trusted.
	MediaBaseURL string
	MediaToken   string

	// Section 18. AutomationEnabled (VERIFICATION_COMPLIANCE_AUTOMATION_ENABLED)
	// runs the leased pre-verification worker that records findings and an
	// advisory recommendation; it never changes a formal status.
	// CaseExpiryEnabled (VERIFICATION_COMPLIANCE_CASE_EXPIRY_ENABLED) moves
	// VERIFIED vendor cases past their expiry to EXPIRED. Both default false.
	AutomationEnabled bool
	CaseExpiryEnabled bool
	// ExternalAuthority names an external verification provider. No registry
	// or KYC provider is contracted, so the only accepted values are "" and
	// "disabled"; anything else is refused rather than silently ignored.
	ExternalAuthority string
}

func Load() Config {
	return Config{
		AppEnv:               getEnv("APP_ENV", "development"),
		HTTPAddr:             getEnv("HTTP_ADDR", ":8108"),
		DatabaseURL:          getEnv("VERIFICATION_COMPLIANCE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5440/verification_compliance_service?sslmode=disable"),
		Migration:            getEnv("VERIFICATION_COMPLIANCE_MIGRATION", "false") == "true",
		PrivateAPIEnabled:    getEnv("VERIFICATION_COMPLIANCE_PRIVATE_API_ENABLED", "false") == "true",
		ServiceSecrets:       os.Getenv("VERIFICATION_COMPLIANCE_SERVICE_SECRETS"),
		AllowedCallers:       getEnv("VERIFICATION_COMPLIANCE_ALLOWED_CALLERS", "fleet-partner-service,vendor-service,admin-backoffice-service"),
		VendorCallbackURL:    strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL")),
		VendorCallbackSecret: strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_SECRET")),

		AssetCapabilityCertificationEnabled: getEnv("ASSET_CAPABILITY_CERTIFICATION_ENABLED", "false") == "true",

		TaxiCallbackURL:        strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_TAXI_CALLBACK_URL")),
		TaxiCallbackSecret:     strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_TAXI_CALLBACK_SECRET")),
		DispatchCallbackURL:    strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_DISPATCH_CALLBACK_URL")),
		DispatchCallbackSecret: strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_DISPATCH_CALLBACK_SECRET")),
		HaulingCallbackURL:     strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_HAULING_CALLBACK_URL")),
		HaulingCallbackSecret:  strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_HAULING_CALLBACK_SECRET")),

		MediaBaseURL: strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_MEDIA_BASE_URL")),
		MediaToken:   strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_MEDIA_TOKEN")),

		AutomationEnabled: getEnv("VERIFICATION_COMPLIANCE_AUTOMATION_ENABLED", "false") == "true",
		CaseExpiryEnabled: getEnv("VERIFICATION_COMPLIANCE_CASE_EXPIRY_ENABLED", "false") == "true",
		ExternalAuthority: strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_EXTERNAL_AUTHORITY")),
	}
}

func (c Config) Validate() error {
	var p []string
	if c.DatabaseURL == "" {
		p = append(p, "VERIFICATION_COMPLIANCE_DATABASE_URL is required")
	}
	if c.PrivateAPIEnabled {
		if strings.TrimSpace(c.ServiceSecrets) == "" {
			p = append(p, "VERIFICATION_COMPLIANCE_SERVICE_SECRETS is required")
		}
		if strings.TrimSpace(c.AllowedCallers) == "" {
			p = append(p, "VERIFICATION_COMPLIANCE_ALLOWED_CALLERS is required")
		}
	}
	if (c.VendorCallbackURL == "") != (c.VendorCallbackSecret == "") {
		p = append(p, "VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL and VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_SECRET must be configured together")
	}
	if c.VendorCallbackURL != "" {
		parsed, err := url.ParseRequestURI(c.VendorCallbackURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			p = append(p, "VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL must be an absolute http(s) URL")
		}
	}
	p = append(p, c.validateAssetCapability()...)
	if c.ExternalAuthority != "" && c.ExternalAuthority != "disabled" {
		p = append(p, "VERIFICATION_COMPLIANCE_EXTERNAL_AUTHORITY: no external verification provider is contracted; only \"disabled\" is accepted")
	}
	if c.AutomationEnabled && (c.MediaBaseURL == "") != (c.MediaToken == "") {
		p = append(p, "VERIFICATION_COMPLIANCE_AUTOMATION_ENABLED with Media requires both VERIFICATION_COMPLIANCE_MEDIA_BASE_URL and VERIFICATION_COMPLIANCE_MEDIA_TOKEN")
	}
	if len(p) > 0 {
		return errors.New("configuration invalid: " + strings.Join(p, "; "))
	}
	return nil
}

// assetAuthorityCallback is one authority's certification callback pair.
type assetAuthorityCallback struct {
	name      string
	urlEnv    string
	secretEnv string
	url       string
	secret    string
}

// AssetAuthorityCallbacks returns the configured per-authority callbacks.
func (c Config) AssetAuthorityCallbacks() []assetAuthorityCallback {
	return []assetAuthorityCallback{
		{"TAXI", "VERIFICATION_COMPLIANCE_TAXI_CALLBACK_URL", "VERIFICATION_COMPLIANCE_TAXI_CALLBACK_SECRET", c.TaxiCallbackURL, c.TaxiCallbackSecret},
		{"DISPATCH", "VERIFICATION_COMPLIANCE_DISPATCH_CALLBACK_URL", "VERIFICATION_COMPLIANCE_DISPATCH_CALLBACK_SECRET", c.DispatchCallbackURL, c.DispatchCallbackSecret},
		{"HAULING", "VERIFICATION_COMPLIANCE_HAULING_CALLBACK_URL", "VERIFICATION_COMPLIANCE_HAULING_CALLBACK_SECRET", c.HaulingCallbackURL, c.HaulingCallbackSecret},
	}
}

// validateAssetCapability rejects configurations that cannot work.
//
// Each rule exists because the alternative failure is silent. A certification
// feature with no reachable authority records decisions nobody ever receives,
// so the asset is suspended here and eligible where work is dispatched. A
// feature with no Media integration can never register a document, so the whole
// evidence path is dead while looking enabled. Neither shows up as an error at
// runtime; both show up as an asset doing work it should not.
func (c Config) validateAssetCapability() []string {
	var p []string
	for _, callback := range c.AssetAuthorityCallbacks() {
		if (callback.url == "") != (callback.secret == "") {
			p = append(p, callback.urlEnv+" and "+callback.secretEnv+" must be configured together")
			continue
		}
		if callback.url == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(callback.url)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			p = append(p, callback.urlEnv+" must be an absolute http(s) origin")
		}
	}
	if !c.AssetCapabilityCertificationEnabled {
		return p
	}
	if (c.MediaBaseURL == "") != (c.MediaToken == "") {
		p = append(p, "VERIFICATION_COMPLIANCE_MEDIA_BASE_URL and VERIFICATION_COMPLIANCE_MEDIA_TOKEN must be configured together")
	}
	if c.MediaBaseURL == "" {
		p = append(p, "ASSET_CAPABILITY_CERTIFICATION_ENABLED requires VERIFICATION_COMPLIANCE_MEDIA_BASE_URL: evidence cannot be bound to an asset without Media")
	}
	reachable := 0
	for _, callback := range c.AssetAuthorityCallbacks() {
		if callback.url != "" && callback.secret != "" {
			reachable++
		}
	}
	if reachable == 0 {
		p = append(p, "ASSET_CAPABILITY_CERTIFICATION_ENABLED requires at least one authority callback: decisions would be recorded and never delivered")
	}
	return p
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
