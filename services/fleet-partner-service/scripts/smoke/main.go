// Command smoke exercises a running fleet-partner-service instance:
// /health, /ready, protected /metrics, the authenticated corporate meta
// endpoint and the service-signed internal meta endpoint.
//
// Usage (against a locally running instance):
//
//	SMOKE_BASE_URL=http://localhost:8113 \
//	SMOKE_ACCESS_TOKEN_SECRET=... \
//	SMOKE_SERVICE_NAME=admin-backoffice-service SMOKE_SERVICE_SECRET=... \
//	SMOKE_METRICS_TOKEN=... \
//	go run ./scripts/smoke
//
// Exits non-zero when any check fails. Never prints secrets.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
)

func main() {
	baseURL := getenv("SMOKE_BASE_URL", "http://localhost:8113")
	failures := 0

	check := func(name string, wantStatus int, build func() (*http.Request, error)) {
		req, err := build()
		if err != nil {
			fmt.Printf("FAIL %-28s build error: %v\n", name, err)
			failures++
			return
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("FAIL %-28s request error: %v\n", name, err)
			failures++
			return
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		if resp.StatusCode != wantStatus {
			fmt.Printf("FAIL %-28s status=%d want=%d body=%s\n", name, resp.StatusCode, wantStatus, strings.TrimSpace(string(body)))
			failures++
			return
		}
		fmt.Printf("PASS %-28s status=%d\n", name, resp.StatusCode)
	}

	check("GET /health", http.StatusOK, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, baseURL+"/health", nil)
	})
	check("GET /ready", http.StatusOK, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, baseURL+"/ready", nil)
	})
	check("GET /metrics unauthenticated", http.StatusUnauthorized, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, baseURL+"/metrics", nil)
	})
	if metricsToken := os.Getenv("SMOKE_METRICS_TOKEN"); metricsToken != "" {
		check("GET /metrics authorized", http.StatusOK, func() (*http.Request, error) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/metrics", nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+metricsToken)
			return req, nil
		})
	}

	check("GET corporate meta no auth", http.StatusUnauthorized, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, baseURL+"/fleet-partner/v1/meta", nil)
	})
	if accessSecret := os.Getenv("SMOKE_ACCESS_TOKEN_SECRET"); accessSecret != "" {
		check("GET corporate meta", http.StatusOK, func() (*http.Request, error) {
			signer := auth.NewTokenSigner([]byte(accessSecret))
			token, err := signer.Sign(auth.Claims{
				Subject:   "usr_smoke",
				Role:      "organization_user",
				Service:   config.ServiceName,
				SessionID: "ses_smoke",
				ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
			})
			if err != nil {
				return nil, err
			}
			req, err := http.NewRequest(http.MethodGet, baseURL+"/fleet-partner/v1/meta", nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return req, nil
		})
	}

	check("GET internal meta no auth", http.StatusUnauthorized, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, baseURL+"/internal/fleet-partner/v1/meta", nil)
	})
	serviceName := os.Getenv("SMOKE_SERVICE_NAME")
	if serviceSecret := os.Getenv("SMOKE_SERVICE_SECRET"); serviceSecret != "" && serviceName != "" {
		check("GET internal meta signed", http.StatusOK, func() (*http.Request, error) {
			req, err := http.NewRequest(http.MethodGet, baseURL+"/internal/fleet-partner/v1/meta", nil)
			if err != nil {
				return nil, err
			}
			if err := serviceauth.SignRequest(req, serviceName, []byte(serviceSecret), nil, time.Now()); err != nil {
				return nil, err
			}
			return req, nil
		})
	}

	if failures > 0 {
		fmt.Printf("smoke: %d check(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("smoke: all checks passed")
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
