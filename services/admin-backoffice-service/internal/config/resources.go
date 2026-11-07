package config

import (
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"errors"
	"os"
	"time"
)

type OwnerResources struct {
	Owner, Origin, SecretFile              string
	AccessPolicyFile, AccessPolicyKeysFile string
}

func loadOwnerResources() ([]OwnerResources, error) {
	values := []OwnerResources{}
	for _, profile := range adminowner.Profiles() {
		prefix := "ADMIN_" + profile.Prefix
		if os.Getenv(prefix+"_RESOURCE_READ_ENABLED") != "true" {
			continue
		}
		item := OwnerResources{Owner: profile.Owner, Origin: os.Getenv(prefix + "_ORIGIN"), SecretFile: os.Getenv(prefix + "_RESOURCE_SECRET_FILE"), AccessPolicyFile: os.Getenv(prefix + "_ACCESS_POLICY_FILE"), AccessPolicyKeysFile: os.Getenv(prefix + "_ACCESS_POLICY_KEYS_FILE")}
		if item.Origin == "" || item.SecretFile == "" {
			return nil, errors.New("owner resource reads require fixed origin and dedicated secret file")
		}
		if (item.AccessPolicyFile == "") != (item.AccessPolicyKeysFile == "") {
			return nil, errors.New("owner resource policy requires both document and governance keys")
		}
		values = append(values, item)
	}
	return values, nil
}

// LoadPolicy preserves the legacy shared policy only when no owner-specific
// pair is configured. An invalid or blocked explicit policy never falls back
// to another owner's policy. Each source retains its own signed revision and
// the existing per-document rule limit.
func (o OwnerResources) LoadPolicy(defaultFile, defaultKeys, environment string, now time.Time) (*adminpolicy.Engine, error) {
	if (o.AccessPolicyFile == "") != (o.AccessPolicyKeysFile == "") {
		return nil, adminpolicy.ErrInvalid
	}
	if o.AccessPolicyFile == "" {
		return adminpolicy.LoadFiles(defaultFile, defaultKeys, environment, now)
	}
	policy, err := adminpolicy.LoadFiles(o.AccessPolicyFile, o.AccessPolicyKeysFile, environment, now)
	if err != nil {
		return nil, err
	}
	profile, ok := adminowner.ForOwner(o.Owner)
	if !ok {
		return nil, adminpolicy.ErrInvalid
	}
	for _, definition := range profile.Definitions {
		rule, err := policy.Rule(definition.Capability, now)
		if err == nil && rule.Owner == o.Owner && rule.SourceContract == definition.SourceContract {
			return policy, nil
		}
	}
	return nil, adminpolicy.ErrInvalid
}
