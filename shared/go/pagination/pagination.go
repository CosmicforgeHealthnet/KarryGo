package pagination

type Page struct {
	Number int `json:"page"`
	Limit  int `json:"limit"`
	Total  int `json:"total,omitempty"`
}

func Normalize(page int, limit int) Page {
	if page < 1 {
		page = 1
	}

	if limit < 1 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	return Page{Number: page, Limit: limit}
}
