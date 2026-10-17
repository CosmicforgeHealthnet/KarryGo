package walletclients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type SupportedBank struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Currency string `json:"currency"`
}

type BankDirectoryClient interface {
	ListBanks(context.Context) ([]SupportedBank, error)
}

func (c HTTPPaystackClient) ListBanks(ctx context.Context) ([]SupportedBank, error) {
	if strings.TrimSpace(c.SecretKey) == "" {
		return nil, fmt.Errorf("bank directory credentials are not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	banks := map[string]SupportedBank{}
	cursors := map[string]bool{}
	next := ""
	for page := 0; page < 20; page++ {
		q := url.Values{"country": {"nigeria"}, "currency": {"NGN"}, "type": {"nuban"}, "perPage": {"100"}, "use_cursor": {"true"}}
		if next != "" {
			q.Set("next", next)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/bank?"+q.Encode()), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+c.SecretKey)
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if len(raw) > 1024*1024 {
			return nil, fmt.Errorf("bank directory response exceeds limit")
		}
		var envelope struct {
			Status bool `json:"status"`
			Data   []struct {
				Name     string `json:"name"`
				Code     string `json:"code"`
				Currency string `json:"currency"`
				Type     string `json:"type"`
				Active   bool   `json:"active"`
				Deleted  bool   `json:"is_deleted"`
			} `json:"data"`
			Meta struct {
				Next *string `json:"next"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Status || envelope.Data == nil {
			return nil, fmt.Errorf("bank directory unavailable")
		}
		for _, bank := range envelope.Data {
			name, code := strings.TrimSpace(bank.Name), strings.TrimSpace(bank.Code)
			if bank.Active && !bank.Deleted && bank.Type == "nuban" && bank.Currency == "NGN" && name != "" && code != "" {
				banks[code] = SupportedBank{Name: name, Code: code, Currency: bank.Currency}
			}
		}
		if envelope.Meta.Next == nil || strings.TrimSpace(*envelope.Meta.Next) == "" {
			result := make([]SupportedBank, 0, len(banks))
			for _, bank := range banks {
				result = append(result, bank)
			}
			sort.Slice(result, func(i, j int) bool {
				if result[i].Name == result[j].Name {
					return result[i].Code < result[j].Code
				}
				return result[i].Name < result[j].Name
			})
			return result, nil
		}
		next = *envelope.Meta.Next
		if cursors[next] {
			return nil, fmt.Errorf("bank directory pagination did not advance")
		}
		cursors[next] = true
	}
	return nil, fmt.Errorf("bank directory pagination limit exceeded")
}
