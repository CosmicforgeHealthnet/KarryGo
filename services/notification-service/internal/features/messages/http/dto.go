package messagehttp

type deviceRequest struct {
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
	Token         string `json:"token"`
	Platform      string `json:"platform"`
	App           string `json:"app"`
}

type realtimeTokenRequest struct {
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
}
