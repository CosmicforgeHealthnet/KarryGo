package savedlocationmodels

import "time"

type SavedLocation struct {
	ID          string
	CustomerID  string
	Title       string
	Address     string
	Latitude    float64
	Longitude   float64
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PublicSavedLocation struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Address     string    `json:"address"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

func (l SavedLocation) Public() PublicSavedLocation {
	return PublicSavedLocation{
		ID:          l.ID,
		Title:       l.Title,
		Address:     l.Address,
		Latitude:    l.Latitude,
		Longitude:   l.Longitude,
		Description: l.Description,
		CreatedAt:   l.CreatedAt,
	}
}
