package providerprofilemodels

import "time"

type Truck struct {
	ID                string
	ProviderID        string
	TruckType         string
	CapacityKg        int
	PlateNumber       string
	Year              *int
	Make              *string
	Model             *string
	Color             *string
	LicenseType       string
	NumberOfAxles     string
	YearsOfExperience string
	GoodsTypes        []string
	HasInsurance      bool
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PublicTruck struct {
	ID                string    `json:"id"`
	ProviderID        string    `json:"provider_id"`
	TruckType         string    `json:"truck_type"`
	CapacityKg        int       `json:"capacity_kg"`
	PlateNumber       string    `json:"plate_number"`
	Year              *int      `json:"year,omitempty"`
	Make              *string   `json:"make,omitempty"`
	Model             *string   `json:"model,omitempty"`
	Color             *string   `json:"color,omitempty"`
	LicenseType       string    `json:"license_type"`
	NumberOfAxles     string    `json:"number_of_axles"`
	YearsOfExperience string    `json:"years_of_experience"`
	GoodsTypes        []string  `json:"goods_types"`
	HasInsurance      bool      `json:"has_insurance"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

func (t Truck) Public() PublicTruck {
	goods := t.GoodsTypes
	if goods == nil {
		goods = []string{}
	}
	return PublicTruck{
		ID:                t.ID,
		ProviderID:        t.ProviderID,
		TruckType:         t.TruckType,
		CapacityKg:        t.CapacityKg,
		PlateNumber:       t.PlateNumber,
		Year:              t.Year,
		Make:              t.Make,
		Model:             t.Model,
		Color:             t.Color,
		LicenseType:       t.LicenseType,
		NumberOfAxles:     t.NumberOfAxles,
		YearsOfExperience: t.YearsOfExperience,
		GoodsTypes:        goods,
		HasInsurance:      t.HasInsurance,
		Status:            t.Status,
		CreatedAt:         t.CreatedAt,
	}
}

// ValidTruckTypes is a superset: the original seeded slugs plus the truck-type
// options shown in the provider Truck Information form (Figma "Select Truck Type").
var ValidTruckTypes = map[string]bool{
	"flatbed":      true,
	"container":    true,
	"tipper":       true,
	"van":          true,
	"refrigerated": true,
	"pickup":       true,
	"box":          true,
	"tanker":       true,
	"trailer":      true,
	"dump":         true,
	"lowbed":       true,
	"crane":        true,
	"other":        true,
}

var ValidTruckStatuses = map[string]bool{
	"active":   true,
	"inactive": true,
}
