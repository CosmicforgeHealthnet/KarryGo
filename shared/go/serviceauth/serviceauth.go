package serviceauth

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	HeaderServiceName = "X-Service-Name"
	HeaderTimestamp   = "X-Service-Timestamp"
	HeaderSignature   = "X-Service-Signature"
	HeaderNonce       = "X-Service-Nonce"
	HeaderVersion     = "X-Service-Signature-Version"
	SignatureVersion  = "2"
)

type Secrets map[string][]byte

func ParseSecrets(value string) Secrets {
	secrets := Secrets{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		name, secret, ok := strings.Cut(part, "=")
		if !ok {
			name, secret, ok = strings.Cut(part, ":")
		}
		if !ok {
			continue
		}

		name = strings.TrimSpace(name)
		secret = strings.TrimSpace(secret)
		if name != "" && secret != "" {
			secrets[name] = []byte(secret)
		}
	}

	return secrets
}

type Verifier struct {
	secrets Secrets
	now     func() time.Time
	maxSkew time.Duration
}

func NewVerifier(secrets Secrets, maxSkew time.Duration) *Verifier {
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}

	return &Verifier{
		secrets: secrets,
		now:     time.Now,
		maxSkew: maxSkew,
	}
}

func SignRequest(req *http.Request, serviceName string, secret []byte, body []byte, now time.Time) error {
	if serviceName == "" || len(secret) == 0 {
		return fmt.Errorf("service name and secret are required")
	}
	if now.IsZero() {
		now = time.Now()
	}

	timestamp := strconv.FormatInt(now.Unix(), 10)
	nonce := strings.TrimSpace(req.Header.Get(HeaderNonce))
	if nonce == "" {
		var value [16]byte
		if _, err := rand.Read(value[:]); err != nil {
			return fmt.Errorf("generate service nonce: %w", err)
		}
		nonce = hex.EncodeToString(value[:])
		req.Header.Set(HeaderNonce, nonce)
	}
	signature := signatureFor(SignatureVersion, serviceName, req.Method, req.URL.RequestURI(), timestamp, nonce, body, secret)
	req.Header.Set(HeaderServiceName, serviceName)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderVersion, SignatureVersion)
	req.Header.Set(HeaderSignature, signature)
	return nil
}

func (v *Verifier) VerifyRequest(req *http.Request) (string, error) {
	serviceName := req.Header.Get(HeaderServiceName)
	timestamp := req.Header.Get(HeaderTimestamp)
	signature := req.Header.Get(HeaderSignature)
	nonce := strings.TrimSpace(req.Header.Get(HeaderNonce))
	version := req.Header.Get(HeaderVersion)
	if serviceName == "" || timestamp == "" || signature == "" || nonce == "" || version != SignatureVersion {
		return "", apperrors.Unauthorized("Service authentication is required.", nil)
	}

	secret, ok := v.secrets[serviceName]
	if !ok || len(secret) == 0 {
		return "", apperrors.Forbidden("Service is not allowed to call this endpoint.", nil)
	}

	parsedTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "", apperrors.Unauthorized("Service authentication is invalid.", err)
	}
	signedAt := time.Unix(parsedTimestamp, 0)
	if signedAt.Before(v.now().Add(-v.maxSkew)) || signedAt.After(v.now().Add(v.maxSkew)) {
		return "", apperrors.Unauthorized("Service authentication has expired.", nil)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", apperrors.BadRequest("Request body is invalid.", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	expected := signatureFor(version, serviceName, req.Method, req.URL.RequestURI(), timestamp, nonce, body, secret)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return "", apperrors.Unauthorized("Service authentication is invalid.", nil)
	}

	return serviceName, nil
}

func Middleware(verifier *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceName, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			// Private authentication is a security boundary and must render its
			// own failure even when a service or test mounts it without the
			// application-wide error middleware.
			httpx.RespondError(c, err)
			c.Abort()
			return
		}

		c.Set("service_name", serviceName)
		c.Next()
	}
}

func signatureFor(version string, serviceName string, method string, requestURI string, timestamp string, nonce string, body []byte, secret []byte) string {
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		version,
		serviceName,
		strings.ToUpper(method),
		requestURI,
		timestamp,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}
