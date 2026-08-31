package inventory

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type section14HBranchResolver struct {
	values map[string]string
}

func (r section14HBranchResolver) BusinessForBranch(
	_ context.Context,
	branchID string,
) (
	string,
	error,
) {
	value, ok := r.values[branchID]

	if !ok {
		return "", errors.New("branch not found")
	}

	return value, nil
}

type section14HNonceStore struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (s *section14HNonceStore) Consume(
	_ context.Context,
	caller string,
	nonce string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := caller + ":" + nonce

	if s.seen[key] {
		return errors.New("nonce replay")
	}

	s.seen[key] = true

	return nil
}

type section14HAuthority struct {
	itemBranch map[string]string
	itemOwner  map[string]string

	reservationBranch map[string]string
	reservationOwner  map[string]string

	lastActor Actor
	lastScope BranchScope
	lastInput CreateReservationInput
}

func (s *section14HAuthority) ItemBranch(
	_ context.Context,
	businessID string,
	itemID string,
) (
	string,
	error,
) {
	if s.itemOwner[itemID] != businessID {
		return "", errors.New("item not found")
	}

	branch, ok := s.itemBranch[itemID]

	if !ok {
		return "", errors.New("item not found")
	}

	return branch, nil
}

func (s *section14HAuthority) Availability(
	_ context.Context,
	actor Actor,
	scope BranchScope,
	_ string,
) (
	Balance,
	error,
) {
	s.lastActor = actor
	s.lastScope = scope

	return Balance{}, nil
}

func (s *section14HAuthority) CreateReservation(
	_ context.Context,
	actor Actor,
	scope BranchScope,
	input CreateReservationInput,
) (
	Reservation,
	error,
) {
	s.lastActor = actor
	s.lastScope = scope
	s.lastInput = input

	return Reservation{
		ID:         "reservation-created",
		BusinessID: actor.BusinessID,
		BranchID:   input.BranchID,
		Status:     ReservationHeld,
	}, nil
}

func (s *section14HAuthority) ReservationBranch(
	_ context.Context,
	businessID string,
	reservationID string,
) (
	string,
	error,
) {
	if s.reservationOwner[reservationID] != businessID {
		return "", errors.New("reservation not found")
	}

	branch, ok := s.reservationBranch[reservationID]

	if !ok {
		return "", errors.New("reservation not found")
	}

	return branch, nil
}

func (s *section14HAuthority) GetReservation(
	_ context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	s.lastActor = actor
	s.lastScope = scope

	branchID := ""

	if scope.BranchID != nil {
		branchID = *scope.BranchID
	}

	return Reservation{
		ID:         reservationID,
		BusinessID: actor.BusinessID,
		BranchID:   branchID,
		Status:     ReservationHeld,
	}, nil
}

func (s *section14HAuthority) ConfirmReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out, err := s.GetReservation(
		ctx,
		actor,
		scope,
		reservationID,
	)

	out.Status = ReservationConfirmed

	return out, err
}

func (s *section14HAuthority) CommitReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out, err := s.GetReservation(
		ctx,
		actor,
		scope,
		reservationID,
	)

	out.Status = ReservationCommitted

	return out, err
}

func (s *section14HAuthority) ReleaseReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out, err := s.GetReservation(
		ctx,
		actor,
		scope,
		reservationID,
	)

	out.Status = ReservationReleased

	return out, err
}

func (s *section14HAuthority) Revision(
	_ context.Context,
	actor Actor,
	scope BranchScope,
	_ string,
) (
	int64,
	time.Time,
	error,
) {
	s.lastActor = actor
	s.lastScope = scope

	return 7, time.Unix(1, 0).UTC(), nil
}

func section14HSignedRequest(
	t *testing.T,
	method string,
	path string,
	caller string,
	secret []byte,
	body []byte,
	nonce string,
) *http.Request {
	t.Helper()

	request := httptest.NewRequest(
		method,
		path,
		bytes.NewReader(body),
	)

	request.Header.Set(
		serviceauth.HeaderNonce,
		nonce,
	)

	if len(body) > 0 {
		request.Header.Set(
			"Content-Type",
			"application/json",
		)
	}

	if err := serviceauth.SignRequest(
		request,
		caller,
		secret,
		body,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	return request
}

func TestSection14HInternalInventorySecurity(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	const (
		businessA = "business-a"
		businessB = "business-b"

		branchA1 = "branch-a1"
		branchA2 = "branch-a2"
		branchB1 = "branch-b1"

		itemA = "item-a"
		itemB = "item-b"

		reservationA  = "res-a"
		reservationA2 = "res-a2"
		reservationB  = "res-b"
	)

	authority := &section14HAuthority{
		itemBranch: map[string]string{
			itemA: branchA1,
			itemB: branchB1,
		},
		itemOwner: map[string]string{
			itemA: businessA,
			itemB: businessB,
		},
		reservationBranch: map[string]string{
			reservationA:  branchA1,
			reservationA2: branchA2,
			reservationB:  branchB1,
		},
		reservationOwner: map[string]string{
			reservationA:  businessA,
			reservationA2: businessA,
			reservationB:  businessB,
		},
	}

	branches := section14HBranchResolver{
		values: map[string]string{
			branchA1: businessA,
			branchA2: businessA,
			branchB1: businessB,
		},
	}

	nonces := &section14HNonceStore{
		seen: map[string]bool{},
	}

	orderSecret := []byte(
		"section14h-order-secret",
	)

	logisticsSecret := []byte(
		"section14h-logistics-secret",
	)

	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{
			"order-service": orderSecret,

			"driver-dispatch-delivery-service": logisticsSecret,
		},
		5*time.Minute,
	)

	handler := newInternalInventoryHandlerWithDependencies(
		authority,
		branches,
		nonces,
		[]string{
			"order-service",
		},
	)

	router := gin.New()

	RegisterInternalRoutes(
		router,
		handler,
		verifier,
		Flags{
			Enabled:       true,
			WritesEnabled: true,
		},
	)

	availabilityPath :=
		"/internal/vendor/v1/inventory/branches/" +
			branchA1 +
			"/items/" +
			itemA +
			"/availability"

	t.Run(
		"unsigned request is refused",
		func(t *testing.T) {
			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				httptest.NewRequest(
					http.MethodGet,
					availabilityPath,
					nil,
				),
			)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusUnauthorized,
				)
			}
		},
	)

	t.Run(
		"bad signature is refused",
		func(t *testing.T) {
			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodGet,
					availabilityPath,
					"order-service",
					[]byte("wrong-secret"),
					nil,
					"14h-bad-signature",
				),
			)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusUnauthorized,
				)
			}
		},
	)

	t.Run(
		"logistics caller has no Inventory mutation authority",
		func(t *testing.T) {
			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodGet,
					availabilityPath,
					"driver-dispatch-delivery-service",
					logisticsSecret,
					nil,
					"14h-logistics-denied",
				),
			)

			if response.Code != http.StatusForbidden {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusForbidden,
				)
			}
		},
	)

	t.Run(
		"valid caller receives branch-bound availability",
		func(t *testing.T) {
			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodGet,
					availabilityPath,
					"order-service",
					orderSecret,
					nil,
					"14h-valid-availability",
				),
			)

			if response.Code != http.StatusOK {
				t.Fatalf(
					"status=%d body=%s",
					response.Code,
					response.Body.String(),
				)
			}

			if authority.lastActor.BusinessID != businessA {
				t.Fatalf(
					"derived business=%s want=%s",
					authority.lastActor.BusinessID,
					businessA,
				)
			}

			if authority.lastActor.ActorRef !=
				"service:order-service" {

				t.Fatalf(
					"actor ref=%s",
					authority.lastActor.ActorRef,
				)
			}

			if authority.lastScope.BranchID == nil ||
				*authority.lastScope.BranchID != branchA1 {

				t.Fatalf(
					"scope=%+v",
					authority.lastScope,
				)
			}
		},
	)

	t.Run(
		"nonce replay is refused",
		func(t *testing.T) {
			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodGet,
					availabilityPath,
					"order-service",
					orderSecret,
					nil,
					"14h-valid-availability",
				),
			)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusUnauthorized,
				)
			}
		},
	)

	t.Run(
		"cross-business Inventory item is indistinguishable from not found",
		func(t *testing.T) {
			path :=
				"/internal/vendor/v1/inventory/branches/" +
					branchA1 +
					"/items/" +
					itemB +
					"/availability"

			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodGet,
					path,
					"order-service",
					orderSecret,
					nil,
					"14h-cross-business-item",
				),
			)

			if response.Code != http.StatusNotFound {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusNotFound,
				)
			}
		},
	)

	t.Run(
		"cross-business reservation mutation is refused",
		func(t *testing.T) {
			path :=
				"/internal/vendor/v1/inventory/branches/" +
					branchA1 +
					"/reservations/" +
					reservationB +
					"/commit"

			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodPost,
					path,
					"order-service",
					orderSecret,
					nil,
					"14h-cross-business-reservation",
				),
			)

			if response.Code != http.StatusNotFound {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusNotFound,
				)
			}
		},
	)

	t.Run(
		"reservation cannot escape its branch",
		func(t *testing.T) {
			path :=
				"/internal/vendor/v1/inventory/branches/" +
					branchA1 +
					"/reservations/" +
					reservationA2

			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodGet,
					path,
					"order-service",
					orderSecret,
					nil,
					"14h-reservation-idor",
				),
			)

			if response.Code != http.StatusNotFound {
				t.Fatalf(
					"status=%d want=%d",
					response.Code,
					http.StatusNotFound,
				)
			}
		},
	)

	t.Run(
		"request body cannot spoof Vendor identity",
		func(t *testing.T) {
			body :=
				[]byte(
					`{
                        "business_id":"business-b",
                        "branch_id":"branch-a1",
                        "source_type":"ORDER",
                        "source_ref":"spoofed-order",
                        "lines":[
                            {
                                "inventory_item_id":"item-a",
                                "quantity":1
                            }
                        ]
                    }`,
				)

			path :=
				"/internal/vendor/v1/inventory/branches/" +
					branchA1 +
					"/reservations"

			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodPost,
					path,
					"order-service",
					orderSecret,
					body,
					"14h-vendor-spoof",
				),
			)

			if response.Code != http.StatusBadRequest {
				t.Fatalf(
					"status=%d want=%d body=%s",
					response.Code,
					http.StatusBadRequest,
					response.Body.String(),
				)
			}
		},
	)

	t.Run(
		"reservation branch in request is path authoritative",
		func(t *testing.T) {
			body :=
				[]byte(
					`{
                        "branch_id":"branch-a2",
                        "source_type":"ORDER",
                        "source_ref":"wrong-branch",
                        "lines":[
                            {
                                "inventory_item_id":"item-a",
                                "quantity":1
                            }
                        ]
                    }`,
				)

			path :=
				"/internal/vendor/v1/inventory/branches/" +
					branchA1 +
					"/reservations"

			response :=
				httptest.NewRecorder()

			router.ServeHTTP(
				response,
				section14HSignedRequest(
					t,
					http.MethodPost,
					path,
					"order-service",
					orderSecret,
					body,
					"14h-body-branch-spoof",
				),
			)

			if response.Code != http.StatusNotFound {
				t.Fatalf(
					"status=%d want=%d body=%s",
					response.Code,
					http.StatusNotFound,
					response.Body.String(),
				)
			}
		},
	)
}

func TestSection14HBranchScopeCannotEscape(
	t *testing.T,
) {
	scope, err :=
		ExactBranchScope(
			"business-a",
			"branch-a1",
		)

	if err != nil {
		t.Fatal(err)
	}

	if !scope.Covers(
		"business-a",
		"branch-a1",
	) {
		t.Fatal(
			"exact branch scope refused its own branch",
		)
	}

	if scope.Covers(
		"business-a",
		"branch-a2",
	) {
		t.Fatal(
			"branch scope escaped to another branch",
		)
	}

	if scope.Covers(
		"business-b",
		"branch-a1",
	) {
		t.Fatal(
			"branch scope escaped to another Vendor",
		)
	}
}
