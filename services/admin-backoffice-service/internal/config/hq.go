package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"
)

// HQ holds Sections 22-30 rollout configuration. Every switch defaults off;
// with a switch off no credential file for it is opened.
type HQ struct {
	Events, SupportQueue, Approvals, Campaigns, CampaignWrites, Finance, Privacy, Exports, MediaMapping bool
	IngressEnabled                                                                                      bool
	IngressAddr                                                                                         string
	SourceSecretsFile                                                                                   string
	Snapshots                                                                                           map[string]SnapshotSource
	BackfillEnabled                                                                                     bool
	ReconcileInterval                                                                                   time.Duration
	NotificationOrigin                                                                                  string
	CampaignSecret                                                                                      []byte
	FinanceOrigin                                                                                       string
	FinanceSecret                                                                                       []byte
	ExportKey                                                                                           []byte
	ExportKeyID                                                                                         string
}

type SnapshotSource struct {
	Origin string
	Secret []byte
}

var snapshotPrefixes = map[string]string{"support-dispute-service": "SUPPORT_DISPUTE", "verification-compliance-service": "VERIFICATION_COMPLIANCE", "notification-service": "NOTIFICATION"}

func readKey(path string, min int) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("HQ integration key file unavailable")
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) < min {
		return nil, errors.New("invalid HQ integration key")
	}
	return key, nil
}

// LoadHQ reads only enabled features. Every loaded key must be distinct from
// every HQ workload key and from each other (per-audience compartmentalization).
func (c Config) LoadHQ() (HQ, error) {
	on := func(k string) bool { return os.Getenv(k) == "true" }
	h := HQ{Events: on("ADMIN_HQ_EVENTS_ENABLED"), SupportQueue: on("ADMIN_HQ_SUPPORT_QUEUE_ENABLED"), Approvals: on("ADMIN_HQ_APPROVALS_ENABLED"), Campaigns: on("ADMIN_HQ_CAMPAIGNS_ENABLED"), CampaignWrites: on("ADMIN_HQ_CAMPAIGN_WRITES_ENABLED"), Finance: on("ADMIN_HQ_FINANCE_ENABLED"), Privacy: on("ADMIN_HQ_PRIVACY_ENABLED"), Exports: on("ADMIN_HQ_EXPORTS_ENABLED"), MediaMapping: on("ADMIN_MEDIA_LEGACY_MAPPING_ENABLED"), IngressEnabled: on("ADMIN_EVENT_INGRESS_ENABLED"), BackfillEnabled: on("ADMIN_EVENT_BACKFILL_ENABLED"), Snapshots: map[string]SnapshotSource{}}
	var keys [][]byte
	var err error
	if h.CampaignWrites && !h.Campaigns {
		return h, errors.New("campaign writes require campaign read-back")
	}
	if h.Campaigns {
		h.NotificationOrigin = os.Getenv("ADMIN_NOTIFICATION_CAMPAIGN_ORIGIN")
		if h.NotificationOrigin == "" {
			return h, errors.New("campaigns require a fixed notification-service origin")
		}
		if h.CampaignSecret, err = readKey(os.Getenv("ADMIN_NOTIFICATION_CAMPAIGN_SECRET_FILE"), 32); err != nil {
			return h, err
		}
		keys = append(keys, h.CampaignSecret)
	}
	if h.Finance {
		h.FinanceOrigin = os.Getenv("ADMIN_FINANCE_REPORT_ORIGIN")
		if h.FinanceOrigin == "" {
			return h, errors.New("finance reports require a fixed payment-wallet origin")
		}
		if h.FinanceSecret, err = readKey(os.Getenv("ADMIN_FINANCE_REPORT_SECRET_FILE"), 32); err != nil {
			return h, err
		}
		keys = append(keys, h.FinanceSecret)
	}
	if h.Exports {
		if h.ExportKey, err = readKey(os.Getenv("ADMIN_EXPORT_KEY_FILE"), 32); err != nil || len(h.ExportKey) != 32 {
			return h, errors.New("exports require a 32-byte encryption key file")
		}
		h.ExportKeyID = os.Getenv("ADMIN_EXPORT_KEY_ID")
		if h.ExportKeyID == "" || len(h.ExportKeyID) > 64 {
			return h, errors.New("exports require an export key id")
		}
		keys = append(keys, h.ExportKey)
	}
	if h.IngressEnabled {
		h.IngressAddr = os.Getenv("ADMIN_EVENT_INGRESS_ADDR")
		h.SourceSecretsFile = os.Getenv("ADMIN_EVENT_SOURCE_SECRETS_FILE")
		if h.IngressAddr == "" || h.SourceSecretsFile == "" {
			return h, errors.New("event ingress requires a private address and source secrets file")
		}
	}
	if h.BackfillEnabled {
		for owner, prefix := range snapshotPrefixes {
			origin := os.Getenv("ADMIN_" + prefix + "_EVENT_SNAPSHOT_ORIGIN")
			if origin == "" {
				continue
			}
			secret, e := readKey(os.Getenv("ADMIN_"+prefix+"_EVENT_SNAPSHOT_SECRET_FILE"), 32)
			if e != nil {
				return h, e
			}
			h.Snapshots[owner] = SnapshotSource{Origin: origin, Secret: secret}
			keys = append(keys, secret)
		}
		h.ReconcileInterval = 24 * time.Hour
		if v := os.Getenv("ADMIN_EVENT_RECONCILE_INTERVAL"); v != "" {
			d, e := time.ParseDuration(v)
			if e != nil || d < 5*time.Minute || d > 7*24*time.Hour {
				return h, errors.New("invalid ADMIN_EVENT_RECONCILE_INTERVAL")
			}
			h.ReconcileInterval = d
		}
	}
	for i, k := range keys {
		for _, encoded := range c.Trust.ServiceKeys {
			hq, _ := base64.RawURLEncoding.DecodeString(encoded)
			if bytes.Equal(k, hq) {
				return h, errors.New("HQ integration key reused an HQ workload key")
			}
		}
		for _, other := range keys[i+1:] {
			if bytes.Equal(k, other) {
				return h, errors.New("HQ integration keys must be distinct per audience")
			}
		}
	}
	return h, nil
}
