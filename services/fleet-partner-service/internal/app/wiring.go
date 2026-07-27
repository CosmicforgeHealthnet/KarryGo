package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	authhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/http"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cache"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Components bundles the wired application services so both the API binary
// and the worker binary build the same object graph.
type Components struct {
	Auth          *authusecases.Service
	Staff         *staffusecases.Service
	Organizations *orgusecases.Service
	Providers     *providerusecases.Service
	Resolver      *staffusecases.MembershipResolver
	Authorizer    *accesscontrol.Authorizer
	Sessions      *authrepos.SessionRepository
	Challenges    *authrepos.ChallengeRepository
	Invitations   *staffrepos.InvitationRepository
	ProviderRepo  *providerrepos.Repository
	Contracts     platformclients.ContractSet
}

// BuildComponents assembles the domain services.
//
// The OTP sender is always the notification-service adapter: no logging or
// no-op sender is selectable from configuration, so OTP delivery can never
// falsely succeed in production. Tests inject a fake sender directly.
func BuildComponents(cfg config.Config, pool *pgxpool.Pool, redisClient *redis.Client, metrics *observability.Metrics) *Components {
	outbox := events.NewOutboxRepository(pool)
	publisher := events.NewDomainPublisher(outbox)

	notificationCfg := cfg.Clients["notification-service"]
	sender := authclients.NewNotificationSender(notificationCfg.BaseURL, notificationCfg.Secret, cfg.HTTPClientTimeout)

	var entryCache *cache.Cache
	var limiter ratelimit.Limiter = ratelimit.NewMemoryLimiter()
	if redisClient != nil {
		entryCache = cache.New(redisClient)
		limiter = ratelimit.NewRedisLimiter(redisCounter{client: redisClient})
	}

	// Contracts for capabilities the authoritative services do not expose
	// yet: fail-closed adapters plus reconciliation (see the audit doc).
	contracts := platformclients.NewUnconfiguredContracts()

	members := staffrepos.NewMemberRepository(pool)
	roles := staffrepos.NewRoleRepository(pool)
	invitations := staffrepos.NewInvitationRepository(pool)
	sessions := authrepos.NewSessionRepository(pool)
	challenges := authrepos.NewChallengeRepository(pool)

	organizations := orgusecases.NewService(orgusecases.Options{
		Pool:          pool,
		Organizations: orgrepos.NewOrganizationRepository(pool),
		Onboarding:    orgrepos.NewOnboardingRepository(pool),
		Branches:      orgrepos.NewBranchRepository(pool),
		Settings:      orgrepos.NewSettingsRepository(pool),
		Contracts:     contracts,
		Events:        publisher,
		Metrics:       metrics,
	})

	staffService := staffusecases.NewService(staffusecases.Options{
		Pool:               pool,
		Members:            members,
		Invitations:        invitations,
		Roles:              roles,
		Organizations:      organizations,
		Sender:             sender,
		Sessions:           sessions,
		Events:             publisher,
		Metrics:            metrics,
		InvitationSecret:   cfg.SessionSecret,
		InvitationTTL:      cfg.StaffInvitationTTL,
		AcceptURLTemplate:  cfg.StaffAcceptURL,
		DefaultCountryCode: cfg.DefaultCountryCode,
	})
	// The organization service needs the staff service to create the founding
	// owner membership; wiring it after construction avoids an import cycle.
	organizations.SetMembershipCreator(staffService)

	providerRepo := providerrepos.NewRepository(pool)
	providers := providerusecases.NewService(providerusecases.Options{
		Pool:               pool,
		Providers:          providerRepo,
		Organizations:      organizations,
		Contracts:          contracts,
		Sender:             sender,
		Events:             publisher,
		Metrics:            metrics,
		InvitationSecret:   cfg.SessionSecret,
		InvitationTTL:      cfg.ProviderInvitationTTL,
		AcceptURLTemplate:  cfg.ProviderAcceptURL,
		DefaultCountryCode: cfg.DefaultCountryCode,
	})

	authService := authusecases.NewService(authusecases.Options{
		Pool:               pool,
		Identities:         authrepos.NewIdentityRepository(pool),
		Challenges:         challenges,
		Sessions:           sessions,
		SecurityEvents:     authrepos.NewSecurityEventRepository(pool),
		Sender:             sender,
		Limiter:            limiter,
		Metrics:            metrics,
		Memberships:        staffService,
		Invitations:        staffService,
		OTPSecret:          cfg.OTPSecret,
		SessionSecret:      cfg.SessionSecret,
		ContactSecret:      cfg.ContactSecret,
		AccessSigner:       auth.NewTokenSigner(cfg.AccessTokenSecret),
		OTPLength:          cfg.OTPLength,
		OTPExpiry:          cfg.OTPExpiry,
		OTPMaxAttempts:     cfg.OTPMaxAttempts,
		OTPResendCooldown:  cfg.OTPResendCooldown,
		OTPMaxPerHour:      cfg.OTPMaxPerHour,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		StepUpTTL:          cfg.StepUpTTL,
		DefaultCountryCode: cfg.DefaultCountryCode,
	})

	return &Components{
		Auth:          authService,
		Staff:         staffService,
		Organizations: organizations,
		Providers:     providers,
		Resolver:      staffusecases.NewMembershipResolver(members, organizations),
		Authorizer:    accesscontrol.NewAuthorizer(roles, entryCache, cfg.AuthorizationCacheTTL),
		Sessions:      sessions,
		Challenges:    challenges,
		Invitations:   invitations,
		ProviderRepo:  providerRepo,
		Contracts:     contracts,
	}
}

// Registrars builds the route registrars for the router.
func (c *Components) Registrars(cfg config.Config) (public RouteRegistrar, identity RouteRegistrar, tenantScoped RouteRegistrar) {
	authHandler := authhttp.NewHandler(c.Auth, cfg.ContactSecret)
	corporateHandler := orghttp.NewHandler(c.Organizations, c.Staff, c.Providers, c.Authorizer)

	public = func(group *gin.RouterGroup) {
		authHandler.RegisterPublicRoutes(group)
	}
	identity = func(group *gin.RouterGroup) {
		authHandler.RegisterAuthenticatedRoutes(group)
		corporateHandler.RegisterOnboardingRoutes(group)
	}
	tenantScoped = func(group *gin.RouterGroup) {
		corporateHandler.RegisterTenantRoutes(group)
	}
	return public, identity, tenantScoped
}

// redisCounter adapts go-redis to the rate-limit counter port with a
// mandatory TTL on every key.
type redisCounter struct {
	client *redis.Client
}

func (r redisCounter) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
