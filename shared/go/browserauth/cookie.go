// Package browserauth implements the cookie/origin boundary, not session storage.
package browserauth

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Policy struct {
	Name     string
	Path     string
	Secure   bool
	SameSite http.SameSite
	Origins  []string
}

// Load keeps deployment policy centralized. HttpOnly and host-only scope cannot
// be disabled. Only explicit development/test environments permit HTTP.
func Load(prefix, environment, name, path string) (Policy, error) {
	development := environment == "development" || environment == "test"
	p := Policy{Name: name, Path: path, Secure: !development, SameSite: http.SameSiteLaxMode}
	if v := os.Getenv(prefix + "_COOKIE_NAME"); v != "" {
		p.Name = v
	}
	switch strings.ToLower(os.Getenv(prefix + "_COOKIE_SAMESITE")) {
	case "", "lax":
	case "strict":
		p.SameSite = http.SameSiteStrictMode
	case "none":
		p.SameSite = http.SameSiteNoneMode
	default:
		return p, fmt.Errorf("invalid browser cookie SameSite policy")
	}
	if v := os.Getenv(prefix + "_COOKIE_SECURE"); v != "" {
		if v != "true" && v != "false" {
			return p, fmt.Errorf("invalid browser cookie Secure policy")
		}
		p.Secure = v == "true"
	}
	if !development && !p.Secure || p.SameSite == http.SameSiteNoneMode && !p.Secure {
		return p, fmt.Errorf("browser refresh cookies require Secure")
	}
	if strings.HasPrefix(p.Name, "__Host-") || strings.HasPrefix(p.Name, "__Secure-") && !p.Secure {
		return p, fmt.Errorf("cookie prefix is incompatible with auth-scoped cookie policy")
	}
	raw := os.Getenv(prefix + "_WEB_ORIGINS")
	if raw == "" && development {
		raw = "http://localhost:3000"
	}
	for _, origin := range strings.Split(raw, ",") {
		origin = strings.TrimSpace(origin)
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || strings.Contains(origin, "*") || (u.Scheme != "https" && !(development && u.Scheme == "http")) {
			return p, fmt.Errorf("explicit trusted browser origins are required")
		}
		p.Origins = append(p.Origins, origin)
	}
	if err := (&http.Cookie{Name: p.Name, Value: "validation", Path: p.Path}).Valid(); err != nil {
		return p, fmt.Errorf("invalid browser cookie configuration")
	}
	return p, nil
}

func (p Policy) Set(c *gin.Context, token string, expiry time.Time) {
	age := int(time.Until(expiry).Seconds())
	if age < 1 {
		age = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: p.Name, Value: token, Path: p.Path, HttpOnly: true, Secure: p.Secure, SameSite: p.SameSite, MaxAge: age, Expires: expiry.UTC()})
}
func (p Policy) Clear(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: p.Name, Path: p.Path, HttpOnly: true, Secure: p.Secure, SameSite: p.SameSite, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
}

// Boundary must run before a more general CORS middleware. A browser-changing
// auth request without a trusted Origin fails closed; no Referer guessing.
func (p Policy) Boundary() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Writer.Header().Add("Vary", "Origin")
		origin := c.GetHeader("Origin")
		trusted := false
		for _, allowed := range p.Origins {
			if origin == allowed {
				trusted = true
				break
			}
		}
		if origin != "" && !trusted || origin == "" && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "untrusted_origin", "message": "Trusted browser origin required."}})
			return
		}
		if trusted {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Request-ID")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
