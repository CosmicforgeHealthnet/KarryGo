package messageclients

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const firebaseScope = "https://www.googleapis.com/auth/firebase.messaging"

type FirebasePushSender struct {
	projectID       string
	credentialsFile string
	httpClient      *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func NewFirebasePushSender(projectID string, credentialsFile string) *FirebasePushSender {
	return &FirebasePushSender{
		projectID:       projectID,
		credentialsFile: credentialsFile,
		httpClient:      http.DefaultClient,
	}
}

func (s *FirebasePushSender) SendPush(ctx context.Context, message PushMessage) (ProviderResult, error) {
	if s.projectID == "" || s.credentialsFile == "" {
		return ProviderResult{Provider: "firebase", Retryable: false}, fmt.Errorf("firebase push is not configured")
	}
	if len(message.Tokens) == 0 {
		return ProviderResult{Provider: "firebase", Retryable: false}, fmt.Errorf("no firebase tokens are available")
	}

	accessToken, err := s.token(ctx)
	if err != nil {
		return ProviderResult{Provider: "firebase", Retryable: true}, err
	}

	var invalidTokens []string
	var firstProviderID *string
	for _, token := range message.Tokens {
		providerID, invalid, err := s.sendOne(ctx, accessToken, token, message)
		if invalid {
			invalidTokens = append(invalidTokens, token)
			continue
		}
		if err != nil {
			return ProviderResult{Provider: "firebase", ProviderMessageID: firstProviderID, InvalidTokens: invalidTokens, Retryable: true}, err
		}
		if firstProviderID == nil {
			firstProviderID = &providerID
		}
	}

	return ProviderResult{Provider: "firebase", ProviderMessageID: firstProviderID, InvalidTokens: invalidTokens}, nil
}

func (s *FirebasePushSender) sendOne(ctx context.Context, accessToken string, token string, message PushMessage) (string, bool, error) {
	payload := map[string]interface{}{
		"message": map[string]interface{}{
			"token": token,
			"notification": map[string]string{
				"title": message.Title,
				"body":  message.Body,
			},
			"data": message.Data,
			"android": map[string]interface{}{
				"priority": androidPriority(message.Priority),
			},
			"apns": map[string]interface{}{
				"headers": map[string]string{"apns-priority": apnsPriority(message.Priority)},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		var parsed struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(responseBody, &parsed)
		return parsed.Name, false, nil
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusNotFound {
		bodyText := string(responseBody)
		if strings.Contains(bodyText, "UNREGISTERED") || strings.Contains(bodyText, "INVALID_ARGUMENT") {
			return "", true, nil
		}
	}

	return "", false, fmt.Errorf("firebase send failed: status=%d body=%s", response.StatusCode, string(responseBody))
}

func (s *FirebasePushSender) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry.Add(-time.Minute)) {
		return s.accessToken, nil
	}

	credentials, err := readFirebaseCredentials(s.credentialsFile)
	if err != nil {
		return "", err
	}
	assertion, err := credentials.assertion(time.Now())
	if err != nil {
		return "", err
	}

	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + assertion
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, credentials.TokenURI, strings.NewReader(form))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("firebase token request failed: status=%d body=%s", response.StatusCode, string(body))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", err
	}
	s.accessToken = parsed.AccessToken
	s.tokenExpiry = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	return s.accessToken, nil
}

type firebaseCredentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func readFirebaseCredentials(path string) (firebaseCredentials, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return firebaseCredentials{}, err
	}
	var credentials firebaseCredentials
	if err := json.Unmarshal(content, &credentials); err != nil {
		return firebaseCredentials{}, err
	}
	if credentials.TokenURI == "" {
		credentials.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return credentials, nil
}

func (c firebaseCredentials) assertion(now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(c.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("firebase private key is invalid")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	privateKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("firebase private key is not RSA")
	}

	header := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]string{"alg": "RS256", "typ": "JWT"}))
	claims := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]interface{}{
		"iss":   c.ClientEmail,
		"scope": firebaseScope,
		"aud":   c.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}))
	unsigned := header + "." + claims
	hashed := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func mustJSON(value interface{}) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func androidPriority(priority string) string {
	if priority == "high" {
		return "HIGH"
	}
	return "NORMAL"
}

func apnsPriority(priority string) string {
	if priority == "high" {
		return "10"
	}
	return "5"
}
