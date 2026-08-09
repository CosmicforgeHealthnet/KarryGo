package serviceauth

import "strings"

// VersionedSecrets holds the raw HMAC secret bytes for every configured
// (serviceName, version) pair. This is the ONLY place a secret byte slice
// exists at runtime outside shared/go/serviceauth's own ephemeral Verifier
// instances -- never logged, never returned from any usecase, never stored
// in fleet_partner_service_credentials (see model.go's doc comment).
type VersionedSecrets map[string]map[int][]byte

// Get returns the secret for (serviceName, version), or nil if unconfigured.
func (v VersionedSecrets) Get(serviceName string, version int) []byte {
	byVersion, ok := v[serviceName]
	if !ok {
		return nil
	}
	return byVersion[version]
}

// ParseVersionedSecrets extends FLEET_PARTNER_SERVICE_SECRETS' existing
// "name=secret,name2=secret2" format (unchanged shared/go/
// serviceauth.ParseSecrets format, still used elsewhere in the platform) to
// additionally accept an optional ":vN" version suffix on the name:
// "payment-wallet-service:v2=secretB,payment-wallet-service=secretA" parses
// as {payment-wallet-service: {1: secretA, 2: secretB}}. A name with no
// ":vN" suffix is always version 1 -- so every existing deployment's
// FLEET_PARTNER_SERVICE_SECRETS value continues to parse identically to
// before this function existed; nothing needs to change for a caller that
// never rotates.
func ParseVersionedSecrets(raw string) VersionedSecrets {
	secrets := VersionedSecrets{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, secret, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		secret = strings.TrimSpace(secret)
		if name == "" || secret == "" {
			continue
		}

		serviceName, version := name, 1
		if base, versionSuffix, hasVersion := strings.Cut(name, ":v"); hasVersion {
			if parsed, ok := parsePositiveInt(versionSuffix); ok {
				serviceName, version = base, parsed
			}
		}

		if secrets[serviceName] == nil {
			secrets[serviceName] = map[int][]byte{}
		}
		secrets[serviceName][version] = []byte(secret)
	}
	return secrets
}

func parsePositiveInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}
