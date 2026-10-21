package config

import (
	"cosmicforge/logistics/shared/go/adminowner"
	"errors"
	"os"
)

type OwnerResources struct{ Owner, Origin, SecretFile string }

func loadOwnerResources() ([]OwnerResources, error) {
	values := []OwnerResources{}
	for _, profile := range adminowner.Profiles() {
		prefix := "ADMIN_" + profile.Prefix
		if os.Getenv(prefix+"_RESOURCE_READ_ENABLED") != "true" {
			continue
		}
		item := OwnerResources{Owner: profile.Owner, Origin: os.Getenv(prefix + "_ORIGIN"), SecretFile: os.Getenv(prefix + "_RESOURCE_SECRET_FILE")}
		if item.Origin == "" || item.SecretFile == "" {
			return nil, errors.New("owner resource reads require fixed origin and dedicated secret file")
		}
		values = append(values, item)
	}
	return values, nil
}
