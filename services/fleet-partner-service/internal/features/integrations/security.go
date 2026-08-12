package integrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnsafeDestination = errors.New("integrations: webhook destination is not permitted")
	ErrSignatureInvalid  = errors.New("integrations: webhook signature is invalid")
)

func randomToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("integrations: generate credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func generateClientID() (string, error) {
	token, err := randomToken(18)
	if err != nil {
		return "", err
	}
	return "kgo_" + token, nil
}

func hashSecret(key []byte, secret string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(secret))
	return mac.Sum(nil)
}

func secretMatches(key []byte, secret string, expected []byte) bool {
	actual := hashSecret(key, secret)
	return len(expected) == len(actual) && subtle.ConstantTimeCompare(actual, expected) == 1
}

func encryptSecret(key []byte, plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("integrations: configure webhook encryption: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("integrations: configure webhook encryption: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("integrations: encrypt webhook secret: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), []byte("karrygo:webhook:v1")), nil
}

func decryptSecret(key, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("integrations: configure webhook decryption: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("integrations: configure webhook decryption: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("integrations: webhook ciphertext is invalid")
	}
	plain, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], []byte("karrygo:webhook:v1"))
	if err != nil {
		return "", errors.New("integrations: webhook ciphertext could not be decrypted")
	}
	return string(plain), nil
}

type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type DestinationValidator struct {
	Resolver          IPResolver
	Production        bool
	AllowHTTP         bool
	AllowedPrivate    []*net.IPNet
	AllowTestLoopback bool
}

func NewDestinationValidator(production, allowHTTP bool, privateCIDRs []string) (*DestinationValidator, error) {
	v := &DestinationValidator{Resolver: net.DefaultResolver, Production: production, AllowHTTP: allowHTTP}
	for _, raw := range privateCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("integrations: invalid private destination CIDR %q", raw)
		}
		v.AllowedPrivate = append(v.AllowedPrivate, network)
	}
	return v, nil
}

func (v *DestinationValidator) Validate(ctx context.Context, raw string) (*url.URL, []net.IP, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, nil, ErrUnsafeDestination
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && v.AllowHTTP && !v.Production) {
		return nil, nil, ErrUnsafeDestination
	}
	if _, err = parsePort(u); err != nil {
		return nil, nil, ErrUnsafeDestination
	}
	host := u.Hostname()
	var ips []net.IP
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	} else {
		resolver := v.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		resolved, lookupErr := resolver.LookupIPAddr(ctx, host)
		if lookupErr != nil || len(resolved) == 0 {
			return nil, nil, ErrUnsafeDestination
		}
		for _, item := range resolved {
			ips = append(ips, item.IP)
		}
	}
	if err = v.validateIPs(ips); err != nil {
		return nil, nil, err
	}
	return u, ips, nil
}

func parsePort(u *url.URL) (string, error) {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			return "443", nil
		}
		return "80", nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", ErrUnsafeDestination
	}
	return port, nil
}

func (v *DestinationValidator) validateIPs(ips []net.IP) error {
	for _, ip := range ips {
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			return ErrUnsafeDestination
		}
		if ip.IsLoopback() {
			if !v.AllowTestLoopback || v.Production {
				return ErrUnsafeDestination
			}
			continue
		}
		if isMetadataIP(ip) {
			return ErrUnsafeDestination
		}
		if ip.IsPrivate() && !v.privateAllowed(ip) {
			return ErrUnsafeDestination
		}
	}
	return nil
}

func (v *DestinationValidator) privateAllowed(ip net.IP) bool {
	for _, network := range v.AllowedPrivate {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func isMetadataIP(ip net.IP) bool {
	return ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("fd00:ec2::254"))
}

func signatureCanonical(subscriptionID, deliveryID, eventID string, timestamp int64, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("v1\n%s\n%s\n%s\n%d\n%s", subscriptionID, deliveryID, eventID, timestamp, hex.EncodeToString(sum[:]))
}

type signingSecret struct {
	Version int
	Secret  string
}

func signWebhook(subscriptionID, deliveryID, eventID string, timestamp int64, body []byte, secrets []signingSecret) string {
	canonical := signatureCanonical(subscriptionID, deliveryID, eventID, timestamp, body)
	parts := make([]string, 0, len(secrets))
	for _, item := range secrets {
		mac := hmac.New(sha256.New, []byte(item.Secret))
		_, _ = mac.Write([]byte(canonical))
		parts = append(parts, fmt.Sprintf("v1.k%d=%s", item.Version, hex.EncodeToString(mac.Sum(nil))))
	}
	return strings.Join(parts, ",")
}

// VerifyWebhookSignature is the documented consumer-side verification helper.
// It performs a constant-time MAC comparison and rejects timestamps outside
// the configured replay window before accepting the body.
func VerifyWebhookSignature(secret string, secretVersion int, subscriptionID, deliveryID, eventID string,
	timestamp int64, body []byte, header string, now time.Time, replayWindow time.Duration) error {
	if replayWindow <= 0 || now.Sub(time.Unix(timestamp, 0)) > replayWindow || time.Unix(timestamp, 0).Sub(now) > replayWindow {
		return ErrSignatureInvalid
	}
	want := signWebhook(subscriptionID, deliveryID, eventID, timestamp, body,
		[]signingSecret{{Version: secretVersion, Secret: secret}})
	wantValue := strings.SplitN(want, "=", 2)
	for _, candidate := range strings.Split(header, ",") {
		parts := strings.SplitN(strings.TrimSpace(candidate), "=", 2)
		if len(parts) != 2 || parts[0] != wantValue[0] {
			continue
		}
		actual, errA := hex.DecodeString(parts[1])
		expected, errE := hex.DecodeString(wantValue[1])
		if errA == nil && errE == nil && len(actual) == len(expected) && subtle.ConstantTimeCompare(actual, expected) == 1 {
			return nil
		}
	}
	return ErrSignatureInvalid
}

var prohibitedPayloadKeys = map[string]bool{
	"password": true, "accesstoken": true, "refreshtoken": true, "secret": true,
	"authorization": true, "credential": true, "bankdetails": true, "identitydocument": true,
	"signedurl": true, "internalexception": true, "debug": true, "auditmetadata": true,
}

func validateWebhookPayload(body []byte) error {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("integrations: webhook payload is invalid: %w", err)
	}
	return walkPayload(value)
}

func walkPayload(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if prohibitedPayloadKeys[normalized] {
				return fmt.Errorf("integrations: webhook payload contains prohibited field %q", key)
			}
			if err := walkPayload(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := walkPayload(child); err != nil {
				return err
			}
		}
	}
	return nil
}
