package section17acontract

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

type section17ASpec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

var ginParamPattern = regexp.MustCompile(`:([A-Za-z0-9_]+)`)

func normalizeGinPath(path string) string {
	return ginParamPattern.ReplaceAllString(path, `{$1}`)
}

func TestSection17AOpenAPIMatchesRegisteredRoutes(t *testing.T) {
	raw, err := os.ReadFile(
		"../../docs/openapi/section17a-customer-cart.openapi.json",
	)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(
		strings.ToLower(string(raw)),
		`"customer_id"`,
	) {
		t.Fatal("Section 17A OpenAPI exposes writable customer identity")
	}

	var spec section17ASpec

	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)

	router := gin.New()

	signer := sharedauth.NewTokenSigner(
		[]byte("section17a-contract-secret"),
	)

	group := router.Group(
		"/api/v1/commerce",
	)

	cart.RegisterCustomerRoutes(
		group,
		cart.NewService(
			cart.NewStore(nil),
			nil,
			nil,
			nil,
		),
		signer,
		nil,
	)

	actual := map[string]bool{}

	for _, route := range router.Routes() {
		key :=
			strings.ToUpper(route.Method) +
				" " +
				normalizeGinPath(route.Path)

		actual[key] = true
	}

	documented := map[string]bool{}

	for path, methods := range spec.Paths {
		for method, operation := range methods {
			method = strings.ToUpper(method)

			switch method {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				continue
			}

			fullPath := "/api/v1/commerce" + path
			key := method + " " + fullPath

			documented[key] = true

			if method != "GET" &&
				!strings.Contains(
					string(operation),
					"IdempotencyKey",
				) {
				t.Fatalf(
					"%s lacks Idempotency-Key documentation",
					key,
				)
			}

			if method != "GET" &&
				path != "/carts" &&
				!strings.Contains(
					string(operation),
					"IfMatch",
				) {
				t.Fatalf(
					"%s lacks If-Match documentation",
					key,
				)
			}
		}
	}

	if len(actual) != 12 {
		t.Fatalf(
			"registered Section 17A routes=%d want=12: %+v",
			len(actual),
			actual,
		)
	}

	if len(documented) != 12 {
		t.Fatalf(
			"documented Section 17A operations=%d want=12: %+v",
			len(documented),
			documented,
		)
	}

	for key := range actual {
		if !documented[key] {
			t.Fatalf(
				"registered route absent from OpenAPI: %s",
				key,
			)
		}
	}

	for key := range documented {
		if !actual[key] {
			t.Fatalf(
				"OpenAPI operation absent from router: %s",
				key,
			)
		}
	}

	for key := range actual {
		lower := strings.ToLower(key)

		for _, forbidden := range []string{
			"/checkout",
			"/orders",
			"/payments",
			"/logistics",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf(
					"out-of-scope Section 17A route: %s",
					key,
				)
			}
		}
	}
}
