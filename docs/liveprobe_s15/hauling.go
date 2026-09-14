package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/custodychain"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

const (
	vendorSecret = "liveprobe-vendor-secret"
	fleetSecret  = "liveprobe-fleet-secret"
	adminSecret  = "liveprobe-admin-secret"
)

// certifyHauling boots the real Hauling service with Commerce and Fleet
// integration on, and drives the Section 15 surfaces over HTTP.
func certifyHauling(p *probe, dsn, redisAddr string) {
	fmt.Fprintln(p.out, "--- driver-hauling-service ---")

	svc, err := boot("hauling", "services/driver-hauling-service", []string{
		"APP_ENV=test", "HAULING_MIGRATION=true",
		"HAULING_DATABASE_URL=" + dsn,
		"HAULING_REDIS_ADDR=" + redisAddr,
		"HAULING_PROVIDER_TOKEN_SECRET=provider-token",
		"HAULING_PROVIDER_REFRESH_SECRET=provider-refresh",
		"HAULING_PROVIDER_OTP_SECRET=provider-otp",
		"HAULING_CUSTOMER_TOKEN_SECRET=customer-token",
		"HAULING_COMMERCE_INTEGRATION_ENABLED=true",
		"HAULING_MEDIA_URL=http://127.0.0.1:1",
		"HAULING_MEDIA_TOKEN=media-token",
		"HAULING_FLEET_INTEGRATION_ENABLED=true",
		"HAULING_FLEET_INTEGRATION_KILL_SWITCH=false",
		"HAULING_FLEET_ALLOWED_CALLERS=fleet-partner-service",
		"HAULING_SERVICE_SECRETS=fleet-partner-service=" + fleetSecret +
			",vendor-service=" + vendorSecret,
		"FLEET_PARTNER_SERVICE_URL=http://127.0.0.1:1",
		"FLEET_PARTNER_SERVICE_CALLER=driver-hauling-service",
		"FLEET_PARTNER_SERVICE_SECRET=hauling-to-fleet",
		"HANDOVER_CODE_SECRET=liveprobe-handover-secret",
		// Fleet integration refuses to start without its durable outbox and
		// its downstream dependencies configured. They are pointed at a
		// closed port on purpose: this probe certifies routing and
		// authorization, and a dependency that answers would make the
		// results depend on services this run does not own.
		"HAULING_OUTBOX_WORKER_ENABLED=true",
		"HAULING_FLEET_EVENT_URL=http://127.0.0.1:1/events",
		"HAULING_FLEET_EVENT_SECRET=fleet-event",
		"HAULING_PAYMENT_URL=http://127.0.0.1:1",
		"HAULING_PAYMENT_SECRET=wallet",
		"HAULING_NOTIFICATION_URL=http://127.0.0.1:1",
		"HAULING_NOTIFICATION_SECRET=notification",
	})
	if err != nil {
		// A boot failure is the single most important thing this probe can
		// catch, so it is reported loudly rather than skipped.
		p.fail("hauling boots with Commerce and Fleet integration enabled", err.Error())
		return
	}
	defer svc.stop()
	p.pass("hauling boots with Commerce and Fleet integration enabled", "GET /health -> 200")

	base := svc.baseURL + "/api/v1/hauling"

	// The boot defect this phase fixed: two handlers on one provider route.
	// A service that panics during registration never reaches this point, so
	// reaching it at all is the regression check. The routes themselves are
	// asserted below.
	p.pass("the provider corporate surfaces no longer collide",
		"route registration completed")

	// Section 15 capability truth, read the way Vendor reads it.
	capabilityURL := base + logisticscapability.Path
	status, body, err := call(http.MethodGet, capabilityURL,
		commercemission.SignatureCaller, []byte(vendorSecret), nil, "")
	if err != nil {
		p.fail("Vendor can read the Hauling capability", err.Error())
	} else if status != http.StatusOK {
		p.fail("Vendor can read the Hauling capability", fmt.Sprintf("-> %d %s", status, body))
	} else {
		var envelope struct {
			Data logisticscapability.Capability `json:"data"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			p.fail("Vendor can read the Hauling capability", "response is not the shared contract")
		} else {
			capability := envelope.Data
			p.pass("Vendor can read the Hauling capability",
				fmt.Sprintf("authority=%s version=%d", capability.Authority, capability.Version))

			if capability.MultiPickupSupported && capability.MaxPickups == 10 {
				p.pass("Hauling still advertises its real multi-pickup routing",
					fmt.Sprintf("maxPickups=%d", capability.MaxPickups))
			} else {
				p.fail("Hauling still advertises its real multi-pickup routing",
					fmt.Sprintf("multiPickup=%v maxPickups=%d",
						capability.MultiPickupSupported, capability.MaxPickups))
			}
			if capability.MaxSecurePickups == 1 && capability.SecurePickupLimit() == 1 {
				p.pass("Hauling declares one protected pickup per mission",
					"maxSecurePickups=1")
			} else {
				p.fail("Hauling declares one protected pickup per mission",
					fmt.Sprintf("maxSecurePickups=%d", capability.MaxSecurePickups))
			}

			// The truth that matters to Vendor: a protected multi-pickup
			// mission is refused before anyone is dispatched.
			ok, reason := logisticscapability.Match(capability, logisticscapability.Requirement{
				PickupCount: 3, DropoffCount: 1, VehicleClass: "TRUCK", SecurePickup: true,
			})
			if !ok && reason == logisticscapability.ReasonSecurePickupLimit {
				p.pass("a protected three-pickup haul is refused by the live declaration", reason)
			} else {
				p.fail("a protected three-pickup haul is refused by the live declaration",
					fmt.Sprintf("serviceable=%v reason=%s", ok, reason))
			}
		}
	}

	// Section 16: the capability contract is Vendor's, not everyone's.
	status, _, err = call(http.MethodGet, capabilityURL, "", nil, nil, "")
	if err != nil {
		p.fail("an unsigned caller cannot read the capability", err.Error())
	} else {
		p.expect("an unsigned caller cannot read the capability", http.StatusUnauthorized, status, "")
	}
	status, _, err = call(http.MethodGet, capabilityURL,
		"fleet-partner-service", []byte(fleetSecret), nil, "")
	if err != nil {
		p.fail("Fleet cannot read Vendor's commerce capability", err.Error())
	} else {
		p.expect("Fleet cannot read Vendor's commerce capability", http.StatusForbidden, status, "")
	}

	// CFC-005: the Commerce pickup handover is a Vendor-verified surface. An
	// unknown mission is a 404 from the handler, which proves the request
	// passed the boundary; a wrong caller never gets that far.
	handoverURL := base + "/internal/commerce/missions/mis_liveprobe/handover/PICKUP"
	status, _, err = call(http.MethodGet, handoverURL,
		commercemission.SignatureCaller, []byte(vendorSecret), nil, "")
	if err != nil {
		p.fail("Vendor reaches the Commerce pickup handover surface", err.Error())
	} else if status == http.StatusUnauthorized || status == http.StatusForbidden {
		p.fail("Vendor reaches the Commerce pickup handover surface",
			fmt.Sprintf("-> %d, Vendor was refused its own verification surface", status))
	} else {
		p.pass("Vendor reaches the Commerce pickup handover surface",
			fmt.Sprintf("-> %d (past the HMAC boundary)", status))
	}
	status, _, err = call(http.MethodGet, handoverURL,
		"fleet-partner-service", []byte(fleetSecret), nil, "")
	if err != nil {
		p.fail("no other service can verify a Commerce pickup", err.Error())
	} else if status == http.StatusUnauthorized || status == http.StatusForbidden {
		p.pass("no other service can verify a Commerce pickup", fmt.Sprintf("-> %d", status))
	} else {
		p.fail("no other service can verify a Commerce pickup",
			fmt.Sprintf("-> %d, a non-Vendor caller reached the handler", status))
	}

	// Section 16 / IAM-018: provider surfaces take bearer tokens, internal
	// surfaces take signatures, and neither accepts the other's credential.
	status, _, err = call(http.MethodGet, handoverURL, "", nil, nil, providerTokenShape)
	if err != nil {
		p.fail("a provider token cannot reach an internal route", err.Error())
	} else {
		p.expect("a provider token cannot reach an internal route", http.StatusUnauthorized, status, "")
	}

	// The corrected provider routes. Both families must exist, and an
	// unauthenticated read must be refused rather than answered.
	for name, path := range map[string]string{
		"the corporate company-assignment LIST is mounted": "/provider/company-assignments",
		"the private-assignment relay is mounted":          "/provider/private-assignments/pas_liveprobe",
	} {
		status, _, err = call(http.MethodGet, base+path, "", nil, nil, "")
		if err != nil {
			p.fail(name, err.Error())
			continue
		}
		if status == http.StatusNotFound {
			p.fail(name, "-> 404, the route is not registered")
			continue
		}
		p.expect(name, http.StatusUnauthorized, status, "(mounted, and closed to anonymous callers)")
	}

	// Section 19: the shared direction matrix is what the running service
	// will describe its credentials with.
	commerce, _ := custodychain.DescriptorFor(custodychain.PurposeCommercePickup)
	if commerce.Holder == custodychain.RoleProvider && commerce.Verifier == custodychain.RoleVendorSender {
		p.pass("the live build carries the Commerce credential direction",
			"holder=PROVIDER verifier=VENDOR_SENDER")
	} else {
		p.fail("the live build carries the Commerce credential direction",
			fmt.Sprintf("holder=%s verifier=%s", commerce.Holder, commerce.Verifier))
	}
}

// providerTokenShape is a well-formed JWT whose contents are irrelevant: the
// point is that an internal route never looks at one.
const providerTokenShape = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
	"eyJzdWIiOiJwcm92aWRlci1saXZlcHJvYmUifQ." +
	"c2lnbmF0dXJlLW5vLWludGVybmFsLXJvdXRlLWNoZWNrcw"
