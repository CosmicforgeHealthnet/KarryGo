package config

import (
    "fmt"
    "os"
    "strconv"
    "strings"
    "time"

    "cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
)

type Config struct {
    AppEnv string
    HTTPAddr string
    Origins map[string]string
    AllowedOrigins []string
    Timeout time.Duration
    BreakerThreshold int
    BreakerCooldown time.Duration
}

// This process is the public transport boundary only. It owns no customer,
// partner, payment, or administrator identities, secrets, databases or workers.
func Load() (Config, error) {
    c := Config{AppEnv: value("APP_ENV", "development"), HTTPAddr: value("HTTP_ADDR", ":8080"),
        Origins: map[string]string{},
        Timeout: time.Duration(number("GATEWAY_UPSTREAM_TIMEOUT_SECONDS", 15))*time.Second,
        BreakerThreshold: number("GATEWAY_BREAKER_THRESHOLD", 5),
        BreakerCooldown: time.Duration(number("GATEWAY_BREAKER_COOLDOWN_SECONDS", 30))*time.Second,
    }
    for _, name := range []string{models.UpstreamCustomer, models.UpstreamTaxi, models.UpstreamDispatch,
        models.UpstreamHauling, models.UpstreamWallet, models.UpstreamNotification,
        models.UpstreamSupport, models.UpstreamVendor, models.UpstreamFleet, models.UpstreamPartnerWeb} {
        if v:=strings.TrimSpace(os.Getenv("GATEWAY_UPSTREAM_"+strings.ToUpper(name)+"_URL")); v!="" { c.Origins[name]=v }
    }
    if c.Origins[models.UpstreamPartnerWeb]=="" {
        return Config{}, fmt.Errorf("GATEWAY_UPSTREAM_PARTNER_WEB_URL required for partner API compatibility")
    }
    for _, origin:=range strings.Split(value("GATEWAY_ALLOWED_ORIGINS", "http://localhost:3000,https://karry-go.vercel.app"), ",") {
        if o:=strings.TrimSpace(origin);o!="" { c.AllowedOrigins=append(c.AllowedOrigins,o) }
    }
    if c.Timeout<=0 || c.BreakerThreshold<1 || c.BreakerCooldown<=0 || len(c.AllowedOrigins)==0 {
        return Config{}, fmt.Errorf("invalid public gateway settings")
    }
    return c,nil
}
func value(key, fallback string) string { if v:=os.Getenv(key);v!="" { return v }; return fallback }
func number(key string, fallback int) int { if v,err:=strconv.Atoi(os.Getenv(key));err==nil { return v }; return fallback }
