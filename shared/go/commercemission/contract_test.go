package commercemission

import (
	"errors"
	"testing"
	"time"
)

func validRequest() Request {
	return Request{
		MissionRef:      "vfm_1",
		ParentOrderRef:  "cpo_1",
		VendorOrderRefs: []string{"cvo_a"},
		CustomerRef:     "customer_1",
		Service:         ServiceDispatch,
		VehicleClass:    VehicleMotorcycle,
		Pickup: Point{
			Address: "1 Market Street", Latitude: 6.455, Longitude: 3.41,
		},
		Dropoff: Point{
			Address: "2 Customer Road", Latitude: 6.43, Longitude: 3.45,
		},
		Load:           Load{WeightGrams: 4000, PackageCount: 2},
		Currency:       "NGN",
		AgreedFeeMinor: 50000,
		RequestedAt:    time.Unix(0, 0).UTC(),
	}
}

func TestRequestValidate_AcceptsAWellFormedMission(t *testing.T) {
	if reason, ok := validRequest().Validate(); !ok {
		t.Fatalf("valid request refused: %s", reason)
	}
}

func TestRequestValidate_RefusesIncompleteOrImpossibleMissions(t *testing.T) {
	cases := map[string]func(*Request){
		"no mission ref":     func(r *Request) { r.MissionRef = " " },
		"no parent order":    func(r *Request) { r.ParentOrderRef = "" },
		"no vendor orders":   func(r *Request) { r.VendorOrderRefs = nil },
		"no customer":        func(r *Request) { r.CustomerRef = "" },
		"unknown service":    func(r *Request) { r.Service = "TAXI" },
		"unknown vehicle":    func(r *Request) { r.VehicleClass = "HOVERCRAFT" },
		"null island pickup": func(r *Request) { r.Pickup.Latitude, r.Pickup.Longitude = 0, 0 },
		"out of range drop":  func(r *Request) { r.Dropoff.Latitude = 200 },
		"no pickup address":  func(r *Request) { r.Pickup.Address = "" },
		"bad currency":       func(r *Request) { r.Currency = "NAIRA" },
		"negative fee":       func(r *Request) { r.AgreedFeeMinor = -1 },
		"negative load":      func(r *Request) { r.Load.WeightGrams = -1 },
	}

	for name, mutate := range cases {
		request := validRequest()
		mutate(&request)
		if _, ok := request.Validate(); ok {
			t.Errorf("%s was accepted", name)
		}
	}
}

// Taxi is deliberately not a commerce execution authority. A mission must
// never be created against an integration that is not ready.
func TestRequestValidate_TaxiIsNotAnExecutionAuthority(t *testing.T) {
	request := validRequest()
	request.Service = "TAXI"
	if _, ok := request.Validate(); ok {
		t.Fatal("a taxi mission was accepted")
	}
	if Service("TAXI").Valid() {
		t.Fatal("TAXI is a valid commerce execution service")
	}
}

// The whole point of the contract: the executor does not re-price.
func TestVerifyMission_RepricedAnswerIsRefused(t *testing.T) {
	request := validRequest()

	mission := Mission{
		MissionRef:       request.MissionRef,
		ExternalRef:      "trip_1",
		Status:           StatusAccepted,
		AcceptedFeeMinor: request.AgreedFeeMinor + 1,
	}
	if err := VerifyMission(request, mission); !errors.Is(err, ErrRepriced) {
		t.Fatalf("err=%v, want ErrRepriced", err)
	}

	// A cheaper answer is refused too. The customer's charge is fixed by an
	// approved quote, and any difference is settled through the Wallet
	// authority rather than by quietly changing the mission's fee.
	mission.AcceptedFeeMinor = request.AgreedFeeMinor - 1
	if err := VerifyMission(request, mission); !errors.Is(err, ErrRepriced) {
		t.Fatalf("cheaper answer: err=%v, want ErrRepriced", err)
	}

	mission.AcceptedFeeMinor = request.AgreedFeeMinor
	if err := VerifyMission(request, mission); err != nil {
		t.Fatalf("matching fee refused: %v", err)
	}
}

func TestVerifyMission_AnswerMustBelongToTheRequest(t *testing.T) {
	request := validRequest()
	mission := Mission{
		MissionRef:       "vfm_other",
		ExternalRef:      "trip_1",
		Status:           StatusAccepted,
		AcceptedFeeMinor: request.AgreedFeeMinor,
	}
	if err := VerifyMission(request, mission); !errors.Is(err, ErrMissionMismatch) {
		t.Fatalf("err=%v, want ErrMissionMismatch", err)
	}
}

// A refusal is a real answer commerce must handle, not a failure to retry.
func TestVerifyMission_RefusalMustNameItsReason(t *testing.T) {
	request := validRequest()

	silent := Mission{MissionRef: request.MissionRef, Status: StatusRefused}
	if err := VerifyMission(request, silent); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("a reasonless refusal was accepted: %v", err)
	}

	explained := Mission{
		MissionRef:  request.MissionRef,
		Status:      StatusRefused,
		RefusalCode: "NO_RIDER_AVAILABLE",
	}
	if err := VerifyMission(request, explained); err != nil {
		t.Fatalf("an explained refusal was rejected: %v", err)
	}
}

func TestVerifyMission_AcceptedMissionMustNameItsOwnIdentity(t *testing.T) {
	request := validRequest()
	mission := Mission{
		MissionRef:       request.MissionRef,
		Status:           StatusAccepted,
		AcceptedFeeMinor: request.AgreedFeeMinor,
	}
	if err := VerifyMission(request, mission); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("an accepted mission with no external reference was allowed")
	}
}

func TestVerifyMission_UnknownStatusIsRefused(t *testing.T) {
	request := validRequest()
	mission := Mission{
		MissionRef:       request.MissionRef,
		ExternalRef:      "trip_1",
		Status:           "MAYBE",
		AcceptedFeeMinor: request.AgreedFeeMinor,
	}
	if err := VerifyMission(request, mission); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err=%v, want ErrInvalidRequest", err)
	}
}
