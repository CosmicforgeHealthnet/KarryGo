package http

import (
	"bytes"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

// No files are loaded by the caller unless private case reads are explicitly on.
// Missing enrollment leaves all source-dependent issuance/read paths denied.
func ownerCaseAuthority(service *usecases.Service, policy *adminpolicy.Engine, caseKey, downloadKey []byte, legacy serviceauth.Secrets) (*usecases.OwnerCaseAuthority, error) {
	path := os.Getenv("MEDIA_ADMIN_CASE_SOURCES_FILE")
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 16384 {
		return nil, errors.New("case source enrollment unavailable")
	}
	var config struct {
		Sources []struct {
			Owner      string `json:"owner"`
			Origin     string `json:"origin"`
			SecretFile string `json:"secret_file"`
		} `json:"sources"`
	}
	// The recognized owners are exactly those with a compiled case kind.
	owners := map[string]bool{}
	for _, spec := range admincase.Specs() {
		owners[spec.Owner] = true
	}
	if admincase.Strict(raw, &config) != nil || len(config.Sources) == 0 || len(config.Sources) > len(owners) {
		return nil, errors.New("invalid case source enrollment")
	}
	a := &usecases.OwnerCaseAuthority{Media: service, Policy: policy, Sources: map[string]usecases.CaseSource{}}
	keys := [][]byte{caseKey, downloadKey}
	for _, key := range legacy {
		keys = append(keys, key)
	}
	for _, entry := range config.Sources {
		if !owners[entry.Owner] {
			return nil, errors.New("unrecognized case source owner")
		}
		if a.Sources[entry.Owner] != nil {
			return nil, errors.New("duplicate source owner")
		}
		encoded, err := os.ReadFile(entry.SecretFile)
		if err != nil || len(encoded) > 4096 {
			return nil, errors.New("case source key unavailable")
		}
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil || len(key) < 32 {
			return nil, errors.New("invalid case source key")
		}
		for _, other := range keys {
			if bytes.Equal(key, other) {
				return nil, errors.New("case source audience reused")
			}
		}
		keys = append(keys, key)
		client, err := admincase.NewClient(entry.Owner, entry.Origin, key)
		if err != nil {
			return nil, err
		}
		a.Sources[entry.Owner] = client
	}
	// Keep all origin/key configuration operator-owned; no request selects it.
	return a, nil
}
