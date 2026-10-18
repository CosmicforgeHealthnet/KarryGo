// Package config loads the bridge's mandatory, locally approved trust policy.
package config

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const HQPrincipal = "cosmicforge-hq-karryam-admin"
const Product = "karryam"
const Caller = "admin-backoffice-service"

// Trust is provisioned by the KarryGo operator, never fetched from assertion URLs.
// No operational or financial approval thresholds are inferred by this service.
type Trust struct {
	Issuer                   string            `json:"issuer"`
	Audience                 string            `json:"audience"`
	WorkloadURI              string            `json:"workload_uri"`
	PolicyVersion            string            `json:"policy_version"`
	AssertionLifetimeSeconds int64             `json:"assertion_lifetime_seconds"`
	SessionLifetimeSeconds   int64             `json:"session_lifetime_seconds"`
	AllowedACR               []string          `json:"allowed_acr"`
	StrongACR                []string          `json:"strong_acr"`
	RequestsPerMinute        int               `json:"requests_per_minute"`
	StaffKeys                map[string]string `json:"staff_keys"`
	ServiceKeys              map[string]string `json:"service_keys"`
	StaffKeyExpires          map[string]int64  `json:"staff_key_expires"`
	ServiceKeyExpires        map[string]int64  `json:"service_key_expires"`
}

func (t Trust) Validate() error {
	if t.Issuer == "" || t.Audience == "" || !strings.HasPrefix(t.WorkloadURI, "spiffe://") || t.PolicyVersion == "" || t.AssertionLifetimeSeconds < 1 || t.AssertionLifetimeSeconds > 300 || t.SessionLifetimeSeconds < 1 || t.SessionLifetimeSeconds > 86400 || len(t.AllowedACR) == 0 || t.RequestsPerMinute < 1 || t.RequestsPerMinute > 10000 || len(t.StaffKeys) == 0 || len(t.ServiceKeys) == 0 {
		return errors.New("invalid approved trust policy")
	}
	for _, acr := range t.StrongACR {
		if !Contains(t.AllowedACR, acr) {
			return errors.New("strong ACR must also be allowed")
		}
	}
	for kid, key := range t.StaffKeys {
		b, err := base64.RawURLEncoding.DecodeString(key)
		if kid == "" || len(kid) > 128 || err != nil || len(b) != ed25519.PublicKeySize || t.StaffKeyExpires[kid] <= time.Now().Unix() {
			return errors.New("invalid staff public key")
		}
	}
	for kid, key := range t.ServiceKeys {
		b, err := base64.RawURLEncoding.DecodeString(key)
		if kid == "" || len(kid) > 128 || err != nil || len(b) < 32 || t.ServiceKeyExpires[kid] <= time.Now().Unix() {
			return errors.New("invalid workload signing key")
		}
	}
	return nil
}
func Contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

type Config struct {
	Address, DatabaseURL, CertFile, KeyFile, CAFile string
	Trust                                           Trust
	RemoteEnabled                                   bool
}

func Load() (Config, error) {
	c := Config{Address: os.Getenv("ADMIN_TLS_ADDR"), DatabaseURL: os.Getenv("ADMIN_BACKOFFICE_DATABASE_URL"), CertFile: os.Getenv("ADMIN_TLS_CERT_FILE"), KeyFile: os.Getenv("ADMIN_TLS_KEY_FILE"), CAFile: os.Getenv("ADMIN_TLS_CLIENT_CA_FILE"), RemoteEnabled: os.Getenv("ADMIN_REMOTE_ENABLED") == "true"}
	if c.Address == "" {
		c.Address = ":8110"
	}
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return c, errors.New("invalid ADMIN_TLS_ADDR")
	}
	if c.DatabaseURL == "" || c.CertFile == "" || c.KeyFile == "" || c.CAFile == "" {
		return c, errors.New("database and TLS files are required")
	}
	f, err := os.Open(os.Getenv("ADMIN_TRUST_FILE"))
	if err != nil {
		return c, errors.New("approved ADMIN_TRUST_FILE is required")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if err = d.Decode(&c.Trust); err != nil {
		return c, errors.New("invalid trust file")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("trailing trust data")
	}
	return c, c.Trust.Validate()
}

func (c Config) TLS() (*tls.Config, error) {
	pair, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, errors.New("invalid server certificate")
	}
	pem, err := os.ReadFile(c.CAFile)
	if err != nil {
		return nil, errors.New("client CA required")
	}
	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid client CA")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || time.Now().After(leaf.NotAfter) {
		return nil, errors.New("expired server certificate")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, ClientCAs: ca, ClientAuth: tls.RequireAndVerifyClientCert}, nil
}
