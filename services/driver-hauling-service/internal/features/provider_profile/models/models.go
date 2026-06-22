package providerprofilemodels

import "time"

type Truck struct {
	ID          string
	ProviderID  string
	TruckType   string
	CapacityKg  int
	PlateNumber string
	Year        *int
	Make        *string
	Model       *string
	Color       *string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PublicTruck struct {
	ID          string  `json:"id"`
	ProviderID  string  `json:"provider_id"`
	TruckType   string  `json:"truck_type"`
	CapacityKg  int     `json:"capacity_kg"`
	PlateNumber string  `json:"plate_number"`
	Year        *int    `json:"year,omitempty"`
	Make        *string `json:"make,omitempty"`
	Model       *string `json:"model,omitempty"`
	Color       *string `json:"color,omitempty"`
	Status      string  `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (t Truck) Public() PublicTruck {
	return PublicTruck{
		ID:          t.ID,
		ProviderID:  t.ProviderID,
		TruckType:   t.TruckType,
		CapacityKg:  t.CapacityKg,
		PlateNumber: t.PlateNumber,
		Year:        t.Year,
		Make:        t.Make,
		Model:       t.Model,
		Color:       t.Color,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt,
	}
}

var ValidTruckTypes = map[string]bool{
	"flatbed":      true,
	"container":    true,
	"tipper":       true,
	"van":          true,
	"refrigerated": true,
}

var ValidTruckStatuses = map[string]bool{
	"active":   true,
	"inactive": true,
}
