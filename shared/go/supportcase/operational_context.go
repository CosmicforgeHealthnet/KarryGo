package supportcase

// Minimized operational context: what an owning service tells Support about a
// job, and nothing more.
//
// This is the reverse of the commerce case contract above. That one carries a
// case INTO Support; this one lets Support ask an owner "what is the state of
// the job this complaint is about?" -- and bounds the answer tightly.
//
// The bound is the point. Support needs to know that a mission is in recovery
// and which provider is on it now. It does not need, and must not receive,
// where the vehicle has been, the customer's address, what anybody was paid,
// or the operational notes a dispatcher typed. Every one of those would
// become support-agent-visible the moment it crossed this boundary, and a
// support tool is the widest-read surface in the platform.
//
// So OperationalContext has no field that could carry them. The guard in this
// package's tests walks the struct and fails on any location-, contact- or
// money-shaped field, so a later "just add the address" cannot pass review by
// accident.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// OperationalContextSchemaVersion is the wire version of the context read.
const OperationalContextSchemaVersion = 1

// ContextCaller is the only service allowed to read operational context.
const ContextCaller = "support-dispute-service"

// Context types. Closed, so Support cannot ask an owner about a kind of thing
// that owner has never heard of.
const (
	ContextTaxiBooking       = "TAXI_BOOKING"
	ContextTaxiGoodsMission  = "TAXI_GOODS_MISSION"
	ContextDispatchMission   = "DISPATCH_MISSION"
	ContextHaulageBooking    = "HAULAGE_BOOKING"
	ContextVendorOrder       = "VENDOR_ORDER"
	ContextFleetPrivateOrder = "FLEET_PRIVATE_ORDER"
)

// ValidContextType reports whether a context type is one this contract knows.
func ValidContextType(value string) bool {
	switch value {
	case ContextTaxiBooking, ContextTaxiGoodsMission, ContextDispatchMission,
		ContextHaulageBooking, ContextVendorOrder, ContextFleetPrivateOrder:
		return true
	default:
		return false
	}
}

// ContextPath is appended to an owner's internal base.
const ContextPath = "/internal/support-context/v1/:contextType/:ref"

// AssetSummary identifies what did the work without describing it.
//
// A class and a product code are enough for "the wrong vehicle came"; a plate
// or a registration is a personal identifier and is deliberately absent.
type AssetSummary struct {
	AssetRef    string `json:"asset_ref,omitempty"`
	AssetClass  string `json:"asset_class,omitempty"`
	ProductCode string `json:"product_code,omitempty"`
}

// OperationalContext is the whole of what an owner returns.
//
// ActorRefs carries REFERENCES (customer_ref, provider_ref, organization_ref),
// never names or contact details -- Support already resolves identity through
// its own identity clients, under its own rules.
//
// Milestones are named, coarse timestamps ("accepted_at", "picked_up_at").
// They say WHEN something happened, never WHERE.
type OperationalContext struct {
	SchemaVersion   int                  `json:"schema_version"`
	SourceService   string               `json:"source_service"`
	ContextType     string               `json:"context_type"`
	SourceRef       string               `json:"source_ref"`
	SourceVersion   int64                `json:"source_version"`
	State           string               `json:"state"`
	ActorRefs       map[string]string    `json:"actor_refs"`
	Asset           *AssetSummary        `json:"asset,omitempty"`
	GuaranteeStatus string               `json:"guarantee_status,omitempty"`
	RecoveryStatus  string               `json:"recovery_status,omitempty"`
	RecoveryCaseRef string               `json:"recovery_case_ref,omitempty"`
	Milestones      map[string]time.Time `json:"milestones"`
	ObservedAt      time.Time            `json:"observed_at"`
}

// ContextRequest is Support asking about one job.
//
// ExpectedActorRef is what makes the read safe across users. Support names the
// complainant; the owner answers only if that actor is actually party to the
// job. A complaint cannot be used to read somebody else's trip by guessing its
// reference -- the owner, which knows who was on the trip, refuses.
type ContextRequest struct {
	ContextType      string
	SourceRef        string
	ExpectedActorRef string
}

// ErrContextNotFound means the owner holds no such job for that actor.
//
// Unknown reference and wrong actor answer identically, so the endpoint does
// not confirm that a reference exists to a caller who is not party to it.
var ErrContextNotFound = errors.New("supportcase: no such operational context for that actor")

// ErrContextUnavailable means the owner could not be asked. Distinct from
// not-found: a complaint about a trip whose owner is briefly down is still a
// complaint about a real trip.
var ErrContextUnavailable = errors.New("supportcase: operational context is unavailable")

// ContextProjector is what an owning service implements.
//
// It is handed the request and returns the minimized context. It must return
// ErrContextNotFound when the reference is unknown OR the expected actor is not
// party to it -- the two must be indistinguishable to the caller.
type ContextProjector interface {
	ProjectSupportContext(ctx context.Context, request ContextRequest) (OperationalContext, error)
}

// ContextProjectorFunc adapts a function.
type ContextProjectorFunc func(context.Context, ContextRequest) (OperationalContext, error)

// ProjectSupportContext implements ContextProjector.
func (f ContextProjectorFunc) ProjectSupportContext(ctx context.Context,
	request ContextRequest) (OperationalContext, error) {
	return f(ctx, request)
}

// RegisterContextRoute mounts the read on an owner's internal group.
//
// verifier authenticates the caller; only ContextCaller is then admitted. The
// route is a GET because the read changes nothing, and the handler has no way
// to: the projector is handed a value and returns a value.
func RegisterContextRoute(group *gin.RouterGroup, verifier *serviceauth.Verifier,
	projector ContextProjector) {
	if group == nil || verifier == nil || projector == nil {
		return
	}
	group.GET(ContextPath, func(c *gin.Context) {
		caller, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false,
				"error": gin.H{"code": "service_authentication_required"}})
			return
		}
		if caller != ContextCaller {
			c.JSON(http.StatusForbidden, gin.H{"success": false,
				"error": gin.H{"code": "caller_not_allowed"}})
			return
		}
		request := ContextRequest{
			ContextType:      strings.ToUpper(strings.TrimSpace(c.Param("contextType"))),
			SourceRef:        strings.TrimSpace(c.Param("ref")),
			ExpectedActorRef: strings.TrimSpace(c.Query("actor_ref")),
		}
		if !ValidContextType(request.ContextType) || request.SourceRef == "" ||
			request.ExpectedActorRef == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false,
				"error": gin.H{"code": "validation_failed"}})
			return
		}
		projected, err := projector.ProjectSupportContext(c.Request.Context(), request)
		if errors.Is(err, ErrContextNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false,
				"error": gin.H{"code": "not_found"}})
			return
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false,
				"error": gin.H{"code": "context_unavailable"}})
			return
		}
		projected.SchemaVersion = OperationalContextSchemaVersion
		projected.ContextType = request.ContextType
		projected.SourceRef = request.SourceRef
		if projected.ActorRefs == nil {
			projected.ActorRefs = map[string]string{}
		}
		if projected.Milestones == nil {
			projected.Milestones = map[string]time.Time{}
		}
		if projected.ObservedAt.IsZero() {
			projected.ObservedAt = time.Now().UTC()
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": projected})
	})
}

// ContextClient reads operational context from one owning service.
type ContextClient struct {
	baseURL string
	secret  []byte
	http    *http.Client
	now     func() time.Time
}

// NewContextClient builds a client. baseURL is the owner's internal base --
// origin plus API base, e.g. "http://taxi:8102/api/v1/taxi" -- to which
// ContextPath is appended.
func NewContextClient(baseURL string, secret []byte, timeout time.Duration) *ContextClient {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &ContextClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		secret:  secret, http: &http.Client{Timeout: timeout},
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Configured reports whether the client can reach its owner.
func (c *ContextClient) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0
}

// Read asks the owner about one job.
func (c *ContextClient) Read(ctx context.Context, request ContextRequest) (OperationalContext, error) {
	if !c.Configured() {
		return OperationalContext{}, ErrContextUnavailable
	}
	path := strings.NewReplacer(":contextType", url.PathEscape(request.ContextType),
		":ref", url.PathEscape(request.SourceRef)).Replace(ContextPath)
	endpoint := c.baseURL + path + "?actor_ref=" + url.QueryEscape(request.ExpectedActorRef)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return OperationalContext{}, err
	}
	if err = serviceauth.SignRequest(httpRequest, ContextCaller, c.secret, nil, c.now()); err != nil {
		return OperationalContext{}, err
	}
	response, err := c.http.Do(httpRequest)
	if err != nil {
		return OperationalContext{}, fmt.Errorf("%w: %v", ErrContextUnavailable, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return OperationalContext{}, fmt.Errorf("%w: %v", ErrContextUnavailable, err)
	}
	switch {
	case response.StatusCode == http.StatusNotFound:
		return OperationalContext{}, ErrContextNotFound
	case response.StatusCode != http.StatusOK:
		return OperationalContext{}, fmt.Errorf("%w: status %d", ErrContextUnavailable,
			response.StatusCode)
	}
	var envelope struct {
		Data OperationalContext `json:"data"`
	}
	if err = json.NewDecoder(bytes.NewReader(raw)).Decode(&envelope); err != nil {
		return OperationalContext{}, fmt.Errorf("%w: malformed context", ErrContextUnavailable)
	}
	return envelope.Data, nil
}

// Minimize serialises a context for storage as a snapshot.
//
// It exists so the snapshot Support stores is byte-for-byte the contract's own
// shape: if a field is not on OperationalContext, it cannot be in the stored
// JSON, whatever the owner happened to send.
func Minimize(context OperationalContext) ([]byte, error) {
	return json.Marshal(context)
}
