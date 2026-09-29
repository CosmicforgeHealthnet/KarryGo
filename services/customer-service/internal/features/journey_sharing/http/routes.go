package sharehttp

import (
	m "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/models"
	u "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/journeyshare"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

func respond(c *gin.Context, data any, err error) {
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": data})
}
func bind(c *gin.Context, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10))
	d.DisallowUnknownFields()
	err := d.Decode(out)
	var rest any
	if err != nil || d.Decode(&rest) != io.EOF {
		respond(c, nil, apperrors.BadRequest("Invalid request body.", nil))
		return false
	}
	return true
}
func owner(c *gin.Context) string { claims, _ := auth.ClaimsFromContext(c); return claims.Subject }
func Register(group *gin.RouterGroup, s *u.Service, signer *auth.TokenSigner, limiter journeyshare.Limiter, viewerOrigin string, trustedGatewayCIDRs ...string) {
	var gateways []netip.Prefix
	for _, cidr := range trustedGatewayCIDRs {
		if prefix, err := netip.ParsePrefix(cidr); err == nil {
			gateways = append(gateways, prefix)
		}
	}
	shares := group.Group("/journey-shares", auth.BearerMiddleware(signer, "customer", "customer"))
	shares.Use(func(c *gin.Context) {
		if err := s.Gate(); err != nil {
			respond(c, nil, err)
			c.Abort()
			return
		}
		c.Next()
	})
	shares.POST("", func(c *gin.Context) {
		started := time.Now()
		defer func() { journeyshare.Observe("create", time.Since(started)) }()
		var in m.Create
		if !bind(c, &in) {
			return
		}
		out, err := s.Create(c, owner(c), c.GetHeader("Idempotency-Key"), in)
		respond(c, out, err)
	})
	shares.GET("", func(c *gin.Context) {
		limit := 50
		offset := 0
		var err error
		if v := c.Query("limit"); v != "" {
			limit, err = strconv.Atoi(v)
		}
		if err == nil && c.Query("offset") != "" {
			offset, err = strconv.Atoi(c.Query("offset"))
		}
		if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 10000 || (c.Query("source_ref") != "" && c.Query("authority") == "") {
			respond(c, nil, apperrors.BadRequest("Invalid pagination or source filter.", nil))
			return
		}
		authority, status := c.Query("authority"), c.Query("status")
		if len(c.Query("source_ref")) > 200 || (authority != "" && authority != "TAXI" && authority != "DISPATCH" && authority != "HAULING" && authority != "VENDOR_COMMERCE") || (status != "" && status != "ACTIVE" && status != "EXPIRED" && status != "REVOKED" && status != "COMPLETED") {
			respond(c, nil, apperrors.BadRequest("Invalid source or status filter.", nil))
			return
		}
		out, e := s.Repository.List(c, owner(c), c.Query("authority"), c.Query("source_ref"), c.Query("status"), limit, offset)
		for i := range out {
			if !out[i].Readable(time.Now().UTC()) && (out[i].Status == "ACTIVE" || out[i].Status == "COMPLETED") {
				out[i].Status = "EXPIRED"
			}
		}
		respond(c, out, e)
	})
	shares.GET("/:shareId", func(c *gin.Context) {
		out, err := s.Repository.Get(c, c.Param("shareId"), owner(c))
		if !out.Readable(time.Now().UTC()) && (out.Status == "ACTIVE" || out.Status == "COMPLETED") {
			out.Status = "EXPIRED"
		}
		respond(c, out, err)
	})
	for _, op := range []string{"revoke", "rotate"} {
		shares.POST("/:shareId/"+op, func(c *gin.Context) {
			started := time.Now()
			defer func() { journeyshare.Observe(op, time.Since(started)) }()
			var in struct {
				ExpectedVersion int64 `json:"expected_version"`
			}
			if !bind(c, &in) {
				return
			}
			out, err := s.Mutate(c, owner(c), c.Param("shareId"), c.GetHeader("Idempotency-Key"), op, in.ExpectedVersion)
			respond(c, out, err)
		})
	}
	group.GET("/public/journey-shares/:token", func(c *gin.Context) {
		started := time.Now()
		defer func() { journeyshare.Observe("public_read", time.Since(started)) }()
		c.Header("Cache-Control", "no-store")
		c.Header("X-Robots-Tag", "noindex, nofollow")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Vary", "Origin")
		origin := c.GetHeader("Origin")
		if origin != "" {
			if viewerOrigin == "" || origin != viewerOrigin {
				respond(c, nil, apperrors.Forbidden("Viewer origin is not allowed.", nil))
				return
			}
			c.Header("Access-Control-Allow-Origin", viewerOrigin)
		}
		if err := s.Gate(); err != nil {
			respond(c, nil, err)
			return
		}
		if limiter == nil {
			respond(c, nil, apperrors.Unavailable("Rate limiting is unavailable.", nil))
			return
		}
		ip := journeyshare.ClientIP(c, gateways)
		token := c.Param("token")
		for _, bucket := range []struct {
			key string
			max int
		}{{"ip:" + m.Digest(ip), 120}, {"token:" + m.Digest(token), 60}} {
			ok, err := limiter.Allow(c, bucket.key, bucket.max, time.Minute)
			if err != nil {
				respond(c, nil, apperrors.Unavailable("Rate limiting is unavailable.", nil))
				return
			}
			if !ok {
				if bucket.max == 120 {
					journeyshare.Observe("per_ip_rate_limit", 0)
				} else {
					journeyshare.Observe("per_token_rate_limit", 0)
				}
				c.Header("Retry-After", "60")
				respond(c, nil, apperrors.New(429, "rate_limited", "Too many share reads.", nil))
				return
			}
		}
		out, err := s.Public(c, token)
		if err != nil {
			journeyshare.Observe("invalid_or_inaccessible_share", 0)
		}
		respond(c, out, err)
	})
}
