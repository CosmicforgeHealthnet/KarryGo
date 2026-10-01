package goods

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/handover"
)

func TestPricingPolicyIsDistinctAndDeterministic(t *testing.T) {
	policy, err := ParsePricingPolicy(`{"base_minor":1000,"per_kilometre_minor":200,"per_minute_minor":10,"handling_minor":50,"minimum_minor":1500,"currency":"ngn","version":"goods-v1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Currency != "NGN" {
		t.Fatalf("currency = %q, want NGN", policy.Currency)
	}
	total, distance := policy.Price(2.5, 10, 2)
	if distance != 500 || total != 1700 {
		t.Fatalf("price = %d (distance %d), want 1700 (500)", total, distance)
	}
}

func TestCargoProfileValidation(t *testing.T) {
	valid := CargoProfile{VehicleID: "vehicle-1", MaxWeightGrams: 100_000, MaxVolumeML: 300_000, MaxPackageCount: 20, LongestDimensionMM: 1200, HandlingCapabilities: []string{commercequote.CapabilityFragileCarry}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid cargo profile rejected: %v", err)
	}
	for name, mutate := range map[string]func(*CargoProfile){
		"weight over":      func(p *CargoProfile) { p.MaxWeightGrams = 1_000_001 },
		"volume over":      func(p *CargoProfile) { p.MaxVolumeML = 10_000_001 },
		"packages over":    func(p *CargoProfile) { p.MaxPackageCount = 101 },
		"dimension over":   func(p *CargoProfile) { p.LongestDimensionMM = 5001 },
		"unknown handling": func(p *CargoProfile) { p.HandlingCapabilities = []string{"MAGIC"} },
		"duplicate handling": func(p *CargoProfile) {
			p.HandlingCapabilities = []string{commercequote.CapabilityFragileCarry, commercequote.CapabilityFragileCarry}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid cargo profile accepted")
			}
		})
	}
}

func TestProviderMissionProjectionExcludesCommerceFinancialAndIdentityData(t *testing.T) {
	view := providerMission(Mission{
		MissionRef: "mission-1", ParentOrderRef: "parent-1", CustomerRef: "customer-1",
		VendorOrderRefs: []string{"vendor-order-1"}, AgreedFeeMinor: 12500,
		Currency: "NGN", SettlementStatus: "APPLIED",
	})
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(body)
	for _, forbidden := range []string{"accepted_fee_minor", "currency", "settlement_status", "parent_order_ref", "customer_ref", "vendor_order_refs"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("provider projection leaked %q: %s", forbidden, serialized)
		}
	}
}

func TestMissionQuoteBindingCoversRouteLoadAndHandling(t *testing.T) {
	quoted := commercequote.Request{
		QuoteReference: "quote-1", VehicleClass: commercequote.VehicleCar,
		Pickup:   commercequote.Point{Latitude: 6.45, Longitude: 3.40},
		Dropoff:  commercequote.Point{Latitude: 6.50, Longitude: 3.45},
		Load:     commercequote.Load{PackageCount: 1, TotalWeightGrams: 1000, TotalVolumeMl: 6, LongestDimensionMm: 30, Capabilities: []string{commercequote.CapabilityFragileCarry}},
		Currency: "NGN",
	}
	mission := commercemission.Request{
		VehicleClass: commercemission.VehicleCar, Currency: "NGN",
		Pickup:  commercemission.Point{Latitude: 6.45, Longitude: 3.40},
		Dropoff: commercemission.Point{Latitude: 6.50, Longitude: 3.45},
		Parcels: []commercemission.Parcel{{ParcelRef: "parcel-1", VendorOrderRef: "vendor-order-1", ManifestRef: "manifest-1", ManifestVersion: 1, WeightGrams: 1000, LengthMm: 10, WidthMm: 20, HeightMm: 30, PackageCount: 1, Handling: commercemission.Handling{Fragile: true}}},
	}
	if !missionMatchesQuote(mission, quoted) {
		t.Fatal("matching mission was not bound to its quote")
	}
	mission.Dropoff.Latitude = 6.51
	if missionMatchesQuote(mission, quoted) {
		t.Fatal("changed route still matched quote")
	}
}

func TestMissionParcelsMustBelongToDeclaredVendorOrders(t *testing.T) {
	request := commercemission.Request{VendorOrderRefs: []string{"vendor-order-1"}, Parcels: []commercemission.Parcel{{ParcelRef: "parcel-1", VendorOrderRef: "vendor-order-1", ManifestRef: "manifest-1", ManifestVersion: 1}}}
	if !validMissionParcels(request) {
		t.Fatal("valid parcel ownership rejected")
	}
	request.Parcels[0].VendorOrderRef = "other-vendor-order"
	if validMissionParcels(request) {
		t.Fatal("cross-vendor parcel accepted")
	}
}

func TestSecureGoodsCredentialUsesIndependentOpaqueQRAndKeyedHashes(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	record, err := handover.NewSecureCodeRecord([]byte("01234567890123456789012345678901"), "taxi-goods", handover.StepDelivery, "challenge-1", 1, "mission-1", "provider-1", "receiver-1", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if record.Code == record.QRToken || strings.Contains(record.QRToken, record.Code) {
		t.Fatal("QR bearer value is derived from or contains the numeric OTP")
	}
	if record.Hash == record.Code || record.QRHash == record.QRToken || record.Hash == record.QRHash {
		t.Fatal("durable credential values are not independent keyed hashes")
	}
	if !handover.VerifySecureCredential([]byte("01234567890123456789012345678901"), "OTP", record, record.Code, record.Hash) ||
		!handover.VerifySecureCredential([]byte("01234567890123456789012345678901"), "QR", record, record.QRToken, record.QRHash) {
		t.Fatal("issued credentials do not verify against their keyed hashes")
	}
}

func TestGoodsMigrationsStoreNoPlaintextCredentialsAndShareExecutionAuthority(t *testing.T) {
	root := filepath.Join("..", "..", "..", "migrations")
	secure, err := os.ReadFile(filepath.Join(root, "008_taxi_goods_secure_handover.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(secure))
	for _, forbidden := range []string{"otp_plaintext", "qr_token text", "pin text"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("secure migration persists forbidden credential material %q", forbidden)
		}
	}
	for _, required := range []string{"otp_hash bytea", "qr_hash bytea", "where status = 'active'", "provider_reassigned", "receiver_changed"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("secure migration missing %q", required)
		}
	}
	foundation, err := os.ReadFile(filepath.Join(root, "007_taxi_goods_foundation.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(foundation)), "provider_one_execution_ref") {
		t.Fatal("Goods migration does not share the passenger operational-state authority")
	}
}
