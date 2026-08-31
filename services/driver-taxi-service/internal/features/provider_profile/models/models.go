package providerprofilemodels

import (
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/vehicleclass"
)

type Vehicle struct {
	ID           string
	ProviderID   string
	VehicleClass string
	PlateNumber  string
	Seats        int
	Year         *int
	Make         *string
	Model        *string
	Color        *string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type PublicVehicle struct {
	ID           string    `json:"id"`
	ProviderID   string    `json:"provider_id"`
	VehicleClass string    `json:"vehicle_class"`
	PlateNumber  string    `json:"plate_number"`
	Seats        int       `json:"seats"`
	Year         *int      `json:"year,omitempty"`
	Make         *string   `json:"make,omitempty"`
	Model        *string   `json:"model,omitempty"`
	Color        *string   `json:"color,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

func (v Vehicle) Public() PublicVehicle {
	return PublicVehicle{
		ID:           v.ID,
		ProviderID:   v.ProviderID,
		VehicleClass: v.VehicleClass,
		PlateNumber:  v.PlateNumber,
		Seats:        v.Seats,
		Year:         v.Year,
		Make:         v.Make,
		Model:        v.Model,
		Color:        v.Color,
		Status:       v.Status,
		CreatedAt:    v.CreatedAt,
	}
}

const (
	VehicleStatusActive   = "active"
	VehicleStatusInactive = "inactive"
)

var ValidVehicleStatuses = map[string]bool{
	VehicleStatusActive:   true,
	VehicleStatusInactive: true,
}

// ValidVehicleClass delegates to the shared vehicleclass package rather than
// keeping a second copy of the list. The matcher compares a booking's requested
// class against a vehicle's stored class by exact string equality, so the two
// sides must be validated against the same source.
func ValidVehicleClass(raw string) bool {
	return vehicleclass.Valid(raw)
}
