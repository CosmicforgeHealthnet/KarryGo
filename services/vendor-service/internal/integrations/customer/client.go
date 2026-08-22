package customer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxAddressResponse = 64 << 10

// Client resolves authoritative customer-owned data from Customer Service.
//
// Section 17A uses it only for saved delivery-address snapshots. Customer
// Service remains the canonical owner; Vendor Service stores only the snapshot
// selected for a cart.
type Client struct {
	baseURL string
	secret  []byte
	http    *http.Client
	now     func() time.Time
}

func NewClient(
	baseURL string,
	secret []byte,
	timeout time.Duration,
) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		secret:  secret,
		http: &http.Client{
			Timeout: timeout,
		},
		now: time.Now,
	}
}

type savedLocationEnvelope struct {
	Success bool                  `json:"success"`
	Data    savedLocationResponse `json:"data"`
}

type savedLocationResponse struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Address     string  `json:"address"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Description string  `json:"description"`
}

// ResolveSavedAddress implements cart.CustomerAddressAuthority.
//
// customerID is never obtained from the request body. The caller supplies the
// verified Customer Service bearer-token subject.
func (c *Client) ResolveSavedAddress(
	ctx context.Context,
	customerID string,
	addressRef string,
) (cart.AddressSnapshot, error) {
	customerID = strings.TrimSpace(customerID)
	addressRef = strings.TrimSpace(addressRef)

	if c == nil ||
		c.baseURL == "" ||
		len(c.secret) == 0 {
		return cart.AddressSnapshot{}, cart.ErrDependencyMissing
	}

	if customerID == "" || addressRef == "" {
		return cart.AddressSnapshot{}, cart.ErrInvalidAddress
	}

	path := "/api/v1/customer/internal/customers/" +
		urlpkg.PathEscape(customerID) +
		"/saved-locations/" +
		urlpkg.PathEscape(addressRef)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+path,
		nil,
	)
	if err != nil {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: customer saved-address request", cart.ErrDependencyMissing)
	}

	if err = serviceauth.SignRequest(
		req,
		"vendor-service",
		c.secret,
		nil,
		c.now().UTC(),
	); err != nil {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: sign customer saved-address request", cart.ErrDependencyMissing)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: customer saved-address transport", cart.ErrDependencyMissing)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(
		io.LimitReader(resp.Body, maxAddressResponse+1),
	)
	if err != nil {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: read customer saved-address response", cart.ErrDependencyMissing)
	}

	if len(raw) > maxAddressResponse {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: oversized customer saved-address response", cart.ErrDependencyMissing)
	}

	if resp.StatusCode == http.StatusNotFound {
		return cart.AddressSnapshot{}, cart.ErrInvalidAddress
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return cart.AddressSnapshot{},
			fmt.Errorf(
				"%w: customer saved-address authority returned http_%d",
				cart.ErrDependencyMissing,
				resp.StatusCode,
			)
	}

	var envelope savedLocationEnvelope

	if err = json.Unmarshal(raw, &envelope); err != nil {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: decode customer saved-address response", cart.ErrDependencyMissing)
	}

	if !envelope.Success {
		return cart.AddressSnapshot{},
			fmt.Errorf("%w: unsuccessful customer saved-address response", cart.ErrDependencyMissing)
	}

	if strings.TrimSpace(envelope.Data.ID) != addressRef {
		return cart.AddressSnapshot{}, cart.ErrInvalidAddress
	}

	snapshot := cart.AddressSnapshot{
		SourceAddressRef: envelope.Data.ID,
		Title:            envelope.Data.Title,
		Address:          envelope.Data.Address,
		Latitude:         envelope.Data.Latitude,
		Longitude:        envelope.Data.Longitude,
		Description:      envelope.Data.Description,
		SnapshottedAt:    c.now().UTC(),
	}

	validated, err := cart.ValidateAddressSnapshot(snapshot)
	if err != nil {
		if errors.Is(err, cart.ErrInvalidAddress) {
			return cart.AddressSnapshot{}, cart.ErrInvalidAddress
		}

		return cart.AddressSnapshot{}, err
	}

	return validated, nil
}
