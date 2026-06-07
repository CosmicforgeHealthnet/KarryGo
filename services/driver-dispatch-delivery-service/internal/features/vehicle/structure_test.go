package vehicle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVehicleFeatureFolderHasRequiredFiles(t *testing.T) {
	required := []string{
		"handler.go",
		"service.go",
		"repository.go",
		"model.go",
		"events.go",
	}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("required vehicle file %s is missing: %v", name, err)
			}
		})
	}
}

func TestVehicleModelConstants(t *testing.T) {
	if BikeMotorcycle != "motorcycle" {
		t.Fatalf("BikeMotorcycle = %q, want motorcycle", BikeMotorcycle)
	}
	if BikeDispatch != "dispatch_bike" {
		t.Fatalf("BikeDispatch = %q, want dispatch_bike", BikeDispatch)
	}
	if VehicleUnverified != "unverified" {
		t.Fatalf("VehicleUnverified = %q, want unverified", VehicleUnverified)
	}
	if VehiclePending != "pending" {
		t.Fatalf("VehiclePending = %q, want pending", VehiclePending)
	}
	if VehicleVerified != "verified" {
		t.Fatalf("VehicleVerified = %q, want verified", VehicleVerified)
	}
	if VehicleRejected != "rejected" {
		t.Fatalf("VehicleRejected = %q, want rejected", VehicleRejected)
	}
	if VehicleSuspended != "suspended" {
		t.Fatalf("VehicleSuspended = %q, want suspended", VehicleSuspended)
	}
	if DocRegistration != "registration" {
		t.Fatalf("DocRegistration = %q, want registration", DocRegistration)
	}
	if DocInsurance != "insurance" {
		t.Fatalf("DocInsurance = %q, want insurance", DocInsurance)
	}
}

func TestIsValidBikeType(t *testing.T) {
	if !IsValidBikeType(BikeMotorcycle) {
		t.Fatal("motorcycle must be valid")
	}
	if !IsValidBikeType(BikeDispatch) {
		t.Fatal("dispatch_bike must be valid")
	}
	if IsValidBikeType("truck") {
		t.Fatal("truck must not be valid")
	}
	if IsValidBikeType("") {
		t.Fatal("empty must not be valid")
	}
}
