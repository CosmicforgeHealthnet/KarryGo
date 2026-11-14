package models

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/money"
)

// WithMoney adds the standard Money object (spec 6.1) beside every flat
// money pair of a finance record: for each "<x>_minor" amount a sibling
// "<x>" carries {"amountMinor","currency"}. The flat fields stay unchanged for
// compatibility. A record without its own currency (an invoice line) uses its
// parent's. A null amount gets no Money object. It handles the record shapes
// the finance-operations routes return: a map, a list of maps and a provider
// statement.
func WithMoney(v any) any {
	switch x := v.(type) {
	case map[string]any:
		attachMoney(x, "")
		return x
	case []map[string]any:
		for _, item := range x {
			attachMoney(item, "")
		}
		return x
	case Statement:
		for i, raw := range x.Periods {
			x.Periods[i] = withMoneyRaw(raw)
		}
		return x
	}
	return v
}

func withMoneyRaw(raw json.RawMessage) json.RawMessage {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return raw
	}
	attachMoney(value, "")
	out, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return out
}

func attachMoney(m map[string]any, inherited string) {
	currency, _ := m["currency"].(string)
	if currency == "" {
		currency = inherited
	}
	for key, value := range m {
		switch nested := value.(type) {
		case map[string]any:
			attachMoney(nested, currency)
			continue
		case []any:
			for _, item := range nested {
				if child, ok := item.(map[string]any); ok {
					attachMoney(child, currency)
				}
			}
			continue
		}
		if currency == "" || !strings.HasSuffix(key, "_minor") {
			continue
		}
		amount, ok := minorUnits(value)
		if !ok {
			continue
		}
		name := strings.TrimSuffix(key, "_minor")
		if _, taken := m[name]; taken {
			name = name + "_money"
			if key == "amount_minor" {
				name = "money"
			}
		}
		m[name] = money.Money{AmountMinor: amount, Currency: currency}
	}
}

func minorUnits(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) || math.Abs(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}
