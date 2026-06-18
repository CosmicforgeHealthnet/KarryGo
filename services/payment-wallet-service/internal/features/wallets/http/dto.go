package wallethttp

type topUpRequest struct {
	AmountKobo     int64  `json:"amount_kobo"`
	Currency       string `json:"currency"`
	CustomerEmail  string `json:"customer_email"`
	IdempotencyKey string `json:"idempotency_key"`
}

type createPaymentIntentRequest struct {
	SourceService   string                 `json:"source_service"`
	SourceReference string                 `json:"source_reference"`
	CustomerID      string                 `json:"customer_id"`
	CustomerEmail   string                 `json:"customer_email"`
	ProviderID      string                 `json:"provider_id"`
	ProviderType    string                 `json:"provider_type"`
	AmountKobo      int64                  `json:"amount_kobo"`
	Currency        string                 `json:"currency"`
	PaymentMethod   string                 `json:"payment_method"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type walletPayRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type resolveBankAccountRequest struct {
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
}

type registerBankAccountRequest struct {
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	Currency      string `json:"currency"`
}

type withdrawalRequest struct {
	BankAccountID  string `json:"bank_account_id"`
	AmountKobo     int64  `json:"amount_kobo"`
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"idempotency_key"`
}

type refundRequest struct {
	PaymentReference string `json:"payment_reference"`
	AmountKobo       int64  `json:"amount_kobo"`
	Currency         string `json:"currency"`
	Reason           string `json:"reason"`
	IdempotencyKey   string `json:"idempotency_key"`
}
