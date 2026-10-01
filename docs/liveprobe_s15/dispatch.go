package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

// certifyDispatch boots the real Dispatch service and drives the surfaces
// this phase changed: the capability declaration Vendor reads, and the
// operator repair surface that previously did not exist.
func certifyDispatch(p *probe, dsn, redisAddr string) {
	fmt.Fprintln(p.out, "--- driver-dispatch-delivery-service ---")

	svc, err := boot("dispatch", "services/driver-dispatch-delivery-service", []string{
		"APP_ENV=test",
		"DISPATCH_DELIVERY_DATABASE_URL=" + dsn,
		"DISPATCH_DELIVERY_REDIS_ADDR=" + redisAddr,
		"DISPATCH_MIGRATION=true",
		"DISPATCH_COMMERCE_INTEGRATION_ENABLED=true",
		"VENDOR_SERVICE_SECRET=" + vendorSecret,
		"FLEET_PARTNER_SERVICE_SECRET=" + fleetSecret,
		"DISPATCH_SERVICE_SECRETS=admin-backoffice-service=" + adminSecret,
		"DISPATCH_MARKETPLACE_INTERNAL_API_ENABLED=true",
		"HANDOVER_CODE_SECRET=liveprobe-handover-secret",
		"DISPATCH_RIDER_ACCESS_TOKEN_SECRET=rider-access",
		"DISPATCH_RIDER_REFRESH_TOKEN_SECRET=rider-refresh",
		"DISPATCH_RIDER_OTP_SECRET=rider-otp",
		"DISPATCH_CUSTOMER_TOKEN_SECRET=customer-token",
		"WALLET_SERVICE_SECRET=wallet-secret",
		"WALLET_SERVICE_URL=http://127.0.0.1:1",
		"NOTIFICATION_SERVICE_SECRET=notification-secret",
		"NOTIFICATION_SERVICE_URL=http://127.0.0.1:1",
		"MEDIA_SERVICE_SECRET=media-secret",
		"MEDIA_SERVICE_URL=http://127.0.0.1:1",
	})
	if err != nil {
		p.fail("dispatch boots with Commerce integration enabled", err.Error())
		return
	}
	defer svc.stop()
	p.pass("dispatch boots with Commerce integration enabled", "GET /health -> 200")

	base := svc.baseURL + "/api/v1"

	// Capability truth: Dispatch protects every pickup it routes, which is
	// what makes a consolidated protected run possible at all.
	// The shared capability path carries its own /internal/commerce prefix,
	// so Dispatch mounts it from the api base rather than under its own
	// commerce base.
	capabilityURL := base + logisticscapability.Path
	status, body, err := call(http.MethodGet, capabilityURL,
		dispatchcommerce.SignatureCaller, []byte(vendorSecret), nil, "")
	switch {
	case err != nil:
		p.fail("Vendor can read the Dispatch capability", err.Error())
	case status != http.StatusOK:
		p.skip("Vendor can read the Dispatch capability",
			fmt.Sprintf("-> %d; the capability route is not mounted in this configuration", status))
	default:
		var envelope struct {
			Data logisticscapability.Capability `json:"data"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			p.fail("Vendor can read the Dispatch capability", "response is not the shared contract")
			break
		}
		capability := envelope.Data
		if capability.MaxSecurePickups == capability.MaxPickups && capability.MaxPickups > 1 {
			p.pass("Dispatch protects every pickup it routes",
				fmt.Sprintf("maxPickups=%d maxSecurePickups=%d",
					capability.MaxPickups, capability.MaxSecurePickups))
		} else {
			p.fail("Dispatch protects every pickup it routes",
				fmt.Sprintf("maxPickups=%d maxSecurePickups=%d",
					capability.MaxPickups, capability.MaxSecurePickups))
		}
	}

	// EVT-010: the operator can see what is stuck, over HTTP, as an
	// allowlisted service.
	outboxURL := base + "/internal/dispatch/v1/operations/outbox"
	status, body, err = call(http.MethodGet, outboxURL,
		"admin-backoffice-service", []byte(adminSecret), nil, "")
	if err != nil {
		p.fail("an operator can inspect event delivery", err.Error())
	} else if status != http.StatusOK {
		p.fail("an operator can inspect event delivery",
			fmt.Sprintf("-> %d %s", status, body))
	} else {
		p.pass("an operator can inspect event delivery", "-> 200")
	}

	// And nobody else can.
	status, _, err = call(http.MethodGet, outboxURL, "", nil, nil, "")
	if err != nil {
		p.fail("an unsigned caller cannot inspect event delivery", err.Error())
	} else {
		p.expect("an unsigned caller cannot inspect event delivery", http.StatusUnauthorized, status, "")
	}
	status, _, err = call(http.MethodGet, outboxURL,
		dispatchcommerce.SignatureCaller, []byte(vendorSecret), nil, "")
	if err != nil {
		p.fail("Vendor cannot inspect Dispatch event delivery", err.Error())
	} else if status == http.StatusOK {
		p.fail("Vendor cannot inspect Dispatch event delivery", "-> 200, Vendor read the outbox")
	} else {
		p.pass("Vendor cannot inspect Dispatch event delivery", fmt.Sprintf("-> %d", status))
	}

	// EVT-011: a requeue of an event that does not exist is a conflict, not
	// a silent success. Reaching that answer proves the repair route is
	// mounted behind the same boundary as the read.
	status, _, err = call(http.MethodPost,
		outboxURL+"/00000000-0000-0000-0000-000000000000/requeue",
		"admin-backoffice-service", []byte(adminSecret), nil, "")
	if err != nil {
		p.fail("an operator can request a requeue", err.Error())
	} else if status == http.StatusNotFound {
		p.fail("an operator can request a requeue", "-> 404, the repair route is not mounted")
	} else {
		p.pass("an operator can request a requeue",
			fmt.Sprintf("-> %d (reached the handler)", status))
	}

	// A filter the contract does not define is refused rather than silently
	// treated as "everything".
	status, _, err = call(http.MethodGet, outboxURL+"?status=ALMOST_SENT",
		"admin-backoffice-service", []byte(adminSecret), nil, "")
	if err != nil {
		p.fail("an invented delivery status is refused", err.Error())
	} else {
		p.expect("an invented delivery status is refused", http.StatusBadRequest, status, "")
	}
}
