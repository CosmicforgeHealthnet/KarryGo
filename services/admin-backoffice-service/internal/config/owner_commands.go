package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
)

// OwnerCommand is one owner's private command channel. Every switch defaults
// off; with both off no key file is opened for that owner.
type OwnerCommand struct {
	Client                    *admincommand.Client
	WriteEnabled, ReadEnabled bool
}

// OwnerCommandClients loads only explicitly enabled owners. Writes require the
// independent readback channel, the owner's canonical configured origin and
// dedicated keys that differ from the owner's resource key and every HQ key.
func (c Config) OwnerCommandClients() ([]OwnerCommand, error) {
	var out []OwnerCommand
	for _, spec := range admincommand.Specs() {
		profile, ok := adminowner.ForOwner(spec.Owner)
		if !ok {
			return nil, errors.New("owner command profile missing")
		}
		prefix := "ADMIN_" + profile.Prefix + "_COMMAND_"
		write, read := os.Getenv(prefix+"WRITES_ENABLED") == "true", os.Getenv(prefix+"READBACK_ENABLED") == "true"
		if !write && !read {
			continue
		}
		if write && !read {
			return nil, errors.New("owner command writes require independent readback")
		}
		var resource *OwnerResources
		for i := range c.OwnerResources {
			if c.OwnerResources[i].Owner == spec.Owner {
				resource = &c.OwnerResources[i]
			}
		}
		if resource == nil {
			return nil, errors.New("owner commands require the configured canonical owner origin")
		}
		key := func(path string) ([]byte, error) {
			raw, err := os.ReadFile(path)
			if err != nil || len(raw) > 4096 {
				return nil, errors.New("owner command key file unavailable")
			}
			decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
			if err != nil || len(decoded) < 32 {
				return nil, errors.New("invalid owner command key")
			}
			return decoded, nil
		}
		resourceKey, err := key(resource.SecretFile)
		if err != nil {
			return nil, err
		}
		readKey, err := key(os.Getenv(prefix + "READBACK_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
		var writeKey []byte
		if write {
			if writeKey, err = key(os.Getenv(prefix + "WRITE_SECRET_FILE")); err != nil {
				return nil, err
			}
		}
		for _, secret := range [][]byte{writeKey, readKey} {
			if len(secret) == 0 {
				continue
			}
			if bytes.Equal(secret, resourceKey) {
				return nil, errors.New("owner command credential reused resource audience")
			}
			for _, encoded := range c.Trust.ServiceKeys {
				hq, _ := base64.RawURLEncoding.DecodeString(encoded)
				if bytes.Equal(secret, hq) {
					return nil, errors.New("owner command credential reused HQ audience")
				}
			}
		}
		client, err := admincommand.NewClient(spec.Owner, resource.Origin, writeKey, readKey)
		if err != nil {
			return nil, err
		}
		out = append(out, OwnerCommand{Client: client, WriteEnabled: write, ReadEnabled: read})
	}
	return out, nil
}
