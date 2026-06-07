package verification

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

var (
	ErrSmileIdentityNotConfigured = errors.New("smile identity client is not configured")
	ErrSmileIdentityUnavailable   = errors.New("smile identity provider is unavailable")
)

type SmileIdentityClient struct {
	APIKey    string
	PartnerID string
	BaseURL   string
	Threshold float64
	FakeMode  string
}

type FaceMatchResult struct {
	MatchScore  float64
	Passed      bool
	RawResponse string
}

type FaceMatcher interface {
	MatchFace(ctx context.Context, selfieURL string, idDocURL string) (FaceMatchResult, error)
}

func NewSmileIdentityClientFromEnv(appEnv string) *SmileIdentityClient {
	threshold := parseFloatEnv("SMILE_IDENTITY_MATCH_THRESHOLD", 70.00)
	fakeMode := strings.ToLower(strings.TrimSpace(os.Getenv("SMILE_IDENTITY_FAKE_MODE")))
	if appEnv == "production" {
		fakeMode = ""
	}
	return &SmileIdentityClient{
		APIKey:    strings.TrimSpace(os.Getenv("SMILE_IDENTITY_API_KEY")),
		PartnerID: strings.TrimSpace(os.Getenv("SMILE_IDENTITY_PARTNER_ID")),
		BaseURL:   strings.TrimSpace(os.Getenv("SMILE_IDENTITY_BASE_URL")),
		Threshold: threshold,
		FakeMode:  fakeMode,
	}
}

func NewStubSmileIdentityClient() *SmileIdentityClient {
	return &SmileIdentityClient{Threshold: 70.00}
}

func (c *SmileIdentityClient) MatchFace(ctx context.Context, selfieURL string, idDocURL string) (FaceMatchResult, error) {
	select {
	case <-ctx.Done():
		return FaceMatchResult{}, ctx.Err()
	default:
	}

	threshold := c.Threshold
	if threshold <= 0 {
		threshold = 70.00
	}

	switch c.FakeMode {
	case "pass":
		score := threshold + 20
		if score > 99.99 {
			score = 99.99
		}
		return FaceMatchResult{MatchScore: score, Passed: true, RawResponse: `{"mode":"fake","result":"pass"}`}, nil
	case "fail":
		score := threshold - 20
		if score < 0 {
			score = 0
		}
		return FaceMatchResult{MatchScore: score, Passed: false, RawResponse: `{"mode":"fake","result":"fail"}`}, nil
	case "unavailable":
		return FaceMatchResult{}, ErrSmileIdentityUnavailable
	}

	if c.APIKey == "" || c.PartnerID == "" || c.BaseURL == "" {
		return FaceMatchResult{}, ErrSmileIdentityNotConfigured
	}

	return FaceMatchResult{}, ErrSmileIdentityUnavailable
}

func parseFloatEnv(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}
