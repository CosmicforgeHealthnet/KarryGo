package bikecapability

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/assignment"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
)

// matchAuthority adapts this feature to assignment's gate interface.
//
// It lives here rather than in the router because the translation is real
// logic -- it decides what "the store did not answer" means to the gate -- and
// burying that in a composition root is how it ends up untested. The two
// decision structs are identical in shape and kept separate because Go's
// nominal typing is what stops assignment importing this whole feature.
type matchAuthority struct{ service *Service }

// NewMatchAuthority builds the adapter assignment's capability gate uses.
func NewMatchAuthority(service *Service) assignment.CapabilityAuthority {
	return matchAuthority{service: service}
}

func (a matchAuthority) LoadProviderBikes(
	ctx context.Context, providerIDs []string, requirement parcelfit.Requirement,
) (map[string][]parcelfit.Capability, error) {
	return a.service.LoadProviderBikeFits(ctx, providerIDs, requirement)
}

func (a matchAuthority) RecordDecisions(ctx context.Context, decisions []assignment.CapabilityDecision) error {
	converted := make([]MatchDecision, 0, len(decisions))
	for _, decision := range decisions {
		converted = append(converted, MatchDecision{
			MissionID:         decision.MissionID,
			ProviderID:        decision.ProviderID,
			BikeID:            decision.BikeID,
			Decision:          decision.Decision,
			ReasonCodes:       decision.ReasonCodes,
			Evidence:          decision.Evidence,
			DistanceM:         decision.DistanceM,
			CapabilityVersion: decision.CapabilityVersion,
			PolicyVersion:     decision.PolicyVersion,
			Shadow:            decision.Shadow,
		})
	}
	return a.service.RecordMatchDecisions(ctx, converted)
}

func (a matchAuthority) PolicyVersion(ctx context.Context) string {
	return a.service.CapabilityPolicyVersion(ctx)
}

func (a matchAuthority) ResolveProduct(requirement parcelfit.Requirement) (string, bool) {
	return a.service.ProductForRequirement(requirement)
}

// NearbyAdapter narrows Dispatch's live availability store to the two fields
// the supply preview may see.
//
// The narrowing is the point. NearbyProvider from the availability feature
// carries a provider id and a live fix; handing all of it to a preview that
// must expose no identity would leave the guarantee resting on the preview's
// discipline rather than on its inputs.
type NearbyAdapter struct {
	Live interface {
		GetNearby(context.Context, availability.NearbyProvidersRequest) ([]availability.NearbyProvider, error)
	}
}

// NearbyForPreview satisfies NearbyProviders.
func (a NearbyAdapter) NearbyForPreview(
	ctx context.Context, latitude, longitude, radiusKm float64, limit int,
) ([]NearbyProvider, error) {
	if a.Live == nil {
		return nil, errors.New("availability index is not configured")
	}
	found, err := a.Live.GetNearby(ctx, availability.NearbyProvidersRequest{
		Latitude: latitude, Longitude: longitude, RadiusKM: radiusKm, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]NearbyProvider, 0, len(found))
	for _, provider := range found {
		out = append(out, NearbyProvider{
			ProviderID: provider.ProviderID,
			DistanceKm: provider.DistanceKM,
		})
	}
	return out, nil
}

// Wiring is everything Section 8 adds, built once.
type Wiring struct {
	Service   *Service
	Handler   *Handler
	Previewer *Previewer
	Gate      *assignment.CapabilityGate
}

// Config carries the deployment switches.
type Config struct {
	MatchingEnabled               bool
	MatchingEnforce               bool
	SupplyPreviewEnabled          bool
	CertificationRequiredProducts string
	PreviewRadiusKM               float64
}

// Build constructs the capability feature.
//
// It is always constructed, even with every flag off, because the rider
// capability endpoints are how a fleet gets surveyed in the first place: a
// deployment that could not record capability until capability matching was on
// could never reach the completeness needed to turn it on.
func Build(db *pgxpool.Pool, nearby NearbyProviders, config Config) Wiring {
	service := New(Options{
		Repository:                    NewRepository(db),
		CertificationRequiredProducts: config.CertificationRequiredProducts,
	})
	return Wiring{
		Service: service,
		Handler: NewHandler(service),
		Previewer: NewPreviewer(PreviewOptions{
			Service:  service,
			Nearby:   nearby,
			RadiusKm: config.PreviewRadiusKM,
			Enabled:  config.SupplyPreviewEnabled,
		}),
		Gate: assignment.NewCapabilityGate(NewMatchAuthority(service), assignment.CapabilityConfig{
			Enabled: config.MatchingEnabled,
			Enforce: config.MatchingEnforce,
		}),
	}
}

// RiderRoutes returns a registrar for vehicle.RegisterRoutes' extra hook, so
// the capability endpoints sit on the SAME authenticated provider group as the
// rest of the vehicle surface. Reusing that group is what gives them the
// group's rider-auth and role gate rather than a second, weaker one.
func (w Wiring) RiderRoutes() func(*gin.RouterGroup) {
	return func(group *gin.RouterGroup) { RegisterProvider(group, w.Handler) }
}
