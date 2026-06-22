// Command devtoken mints a short-lived access token for local testing of the
// payment-wallet service. It signs the token with the same TokenSigner the
// service verifies against, so the resulting bearer token is accepted by the
// customer and provider endpoints without standing up customer-service or the
// provider apps.
//
// This is a development convenience only. The default secrets are the documented
// local dev defaults; never use them outside local development.
//
// Examples:
//
//	go run ./docs/devtoken -kind=customer
//	go run ./docs/devtoken -kind=provider -service=hauling
//	go run ./docs/devtoken -kind=provider -service=taxi -sub=my-provider-id -ttl=2h
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	sharedauth "cosmicforge/logistics/shared/go/auth"

	"github.com/google/uuid"
)

// Documented local dev defaults. These mirror services/payment-wallet-service/.env.example.
const (
	defaultCustomerSecret = "development-customer-access-token-secret"
	defaultServiceSecrets = "taxi=development-taxi-access-token-secret," +
		"dispatch=development-dispatch-access-token-secret," +
		"hauling=development-hauling-provider-token-secret"
)

func main() {
	kind := flag.String("kind", "customer", "token kind: customer | provider")
	service := flag.String("service", "", "service binding; defaults to 'customer' for customer kind, required for provider (taxi|dispatch|hauling)")
	role := flag.String("role", "", "role claim; defaults to 'customer' for customer kind, 'truck_provider' for provider kind")
	sub := flag.String("sub", "", "subject (owner id); defaults to a random UUID")
	secret := flag.String("secret", "", "override signing secret; defaults to the matching dev secret from the env or documented dev default")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime, e.g. 30m, 1h, 24h")
	flag.Parse()

	subject := *sub
	if subject == "" {
		subject = uuid.NewString()
	}

	var (
		tokenService string
		tokenRole    string
		signingKey   string
	)

	switch strings.ToLower(*kind) {
	case "customer":
		tokenService = "customer"
		tokenRole = firstNonEmpty(*role, "customer")
		signingKey = firstNonEmpty(*secret, os.Getenv("PAYMENT_WALLET_CUSTOMER_ACCESS_TOKEN_SECRET"), defaultCustomerSecret)
	case "provider":
		tokenService = firstNonEmpty(*service, "")
		if tokenService == "" {
			fail("provider tokens require -service=taxi|dispatch|hauling")
		}
		tokenRole = firstNonEmpty(*role, "truck_provider")
		signingKey = firstNonEmpty(*secret, providerSecret(tokenService))
		if signingKey == "" {
			fail(fmt.Sprintf("no signing secret for service %q; pass -secret or set PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS", tokenService))
		}
	default:
		fail(fmt.Sprintf("unknown -kind %q (want customer|provider)", *kind))
	}

	now := time.Now()
	signer := sharedauth.NewTokenSigner([]byte(signingKey))
	token, err := signer.Sign(sharedauth.Claims{
		Subject:   subject,
		Role:      tokenRole,
		Service:   tokenService,
		SessionID: uuid.NewString(),
		Type:      sharedauth.TokenTypeAccess,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(*ttl).Unix(),
	})
	if err != nil {
		fail("failed to sign token: " + err.Error())
	}

	// Human-readable summary on stderr, raw token on stdout so it can be piped.
	fmt.Fprintf(os.Stderr, "kind=%s service=%s role=%s sub=%s expires=%s\n",
		strings.ToLower(*kind), tokenService, tokenRole, subject, now.Add(*ttl).Format(time.RFC3339))
	fmt.Println(token)
}

// providerSecret resolves the dev signing secret for a provider service from the
// PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS env var, falling back to the
// documented dev defaults. The value format is "svc=secret,svc=secret".
func providerSecret(service string) string {
	source := os.Getenv("PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS")
	if source == "" {
		source = defaultServiceSecrets
	}
	for _, part := range strings.Split(source, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(name) == service {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "devtoken: "+message)
	os.Exit(1)
}
