package models

import (
	"encoding/json"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/money"
)

func TestWithMoneyAddsObjectBesideEveryFlatPair(t *testing.T) {
	invoice := map[string]any{
		"id": "inv_1", "currency": "NGN", "total_minor": float64(3800), "subtotal_minor": float64(3600),
		"lines": []any{map[string]any{"id": "inl_1", "unit_amount_minor": float64(1200), "line_total_minor": float64(3600)}},
	}
	WithMoney(invoice)
	if got := invoice["total"]; got != (money.Money{AmountMinor: 3800, Currency: "NGN"}) {
		t.Fatalf("total = %#v", got)
	}
	if invoice["total_minor"] != float64(3800) || invoice["currency"] != "NGN" {
		t.Fatal("flat fields must stay unchanged")
	}
	line := invoice["lines"].([]any)[0].(map[string]any)
	if got := line["line_total"]; got != (money.Money{AmountMinor: 3600, Currency: "NGN"}) {
		t.Fatalf("an invoice line inherits the invoice currency: %#v", got)
	}

	claim := map[string]any{"currency": "NGN", "approved_amount_minor": nil}
	WithMoney(claim)
	if _, present := claim["approved_amount"]; present {
		t.Fatal("a null amount has no Money object")
	}
	noCurrency := map[string]any{"amount_minor": float64(5)}
	WithMoney(noCurrency)
	if _, present := noCurrency["amount"]; present {
		t.Fatal("an amount without a currency has no Money object")
	}
}

func TestWithMoneyHandlesListsAndStatements(t *testing.T) {
	list := []map[string]any{{"currency": "USD", "amount_minor": float64(7)}}
	WithMoney(list)
	if list[0]["amount"] != (money.Money{AmountMinor: 7, Currency: "USD"}) {
		t.Fatalf("list item = %#v", list[0])
	}
	statement := Statement{MembershipID: "mem_1", Periods: []json.RawMessage{
		json.RawMessage(`{"period":{"currency":"NGN"},"items":[{"currency":"NGN","amount_minor":9007199254740993}]}`),
	}}
	out := WithMoney(statement).(Statement)
	var period struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(out.Periods[0], &period); err != nil {
		t.Fatal(err)
	}
	if string(period.Items[0]["amount"]) != `{"amountMinor":9007199254740993,"currency":"NGN"}` {
		t.Fatalf("statement item amount = %s", period.Items[0]["amount"])
	}
	if string(period.Items[0]["amount_minor"]) != `9007199254740993` {
		t.Fatalf("statement flat amount changed: %s", period.Items[0]["amount_minor"])
	}
}
