// Package preferences is the Customer Service authority for reusable,
// structured quality preferences. Vendor Commerce consumes a narrow
// authenticated contract and snapshots the result into an order.
package preferences

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Preference struct {
	ID, CustomerID, Type, Value string
	Revision                    int64 `json:"revision"`
}

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) List(ctx context.Context, customerID string) ([]Preference, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("preference authority unavailable")
	}
	rows, err := s.db.Query(ctx, `SELECT id::text,customer_id::text,preference_type,preference_value,revision FROM customer_quality_preferences WHERE customer_id=$1 ORDER BY preference_type`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Preference, 0)
	for rows.Next() {
		var p Preference
		if err = rows.Scan(&p.ID, &p.CustomerID, &p.Type, &p.Value, &p.Revision); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Store) Upsert(ctx context.Context, customerID, typ, value string) (Preference, error) {
	typ, value = strings.ToUpper(strings.TrimSpace(typ)), strings.TrimSpace(value)
	if customerID == "" || value == "" {
		return Preference{}, errors.New("preference type and value are required")
	}
	switch typ {
	case "RIPENESS", "FIRMNESS", "CUT_SIZE", "PREPARATION", "CLEANING", "SORTING":
	default:
		return Preference{}, errors.New("invalid preference type")
	}
	var p Preference
	err := s.db.QueryRow(ctx, `INSERT INTO customer_quality_preferences(id,customer_id,preference_type,preference_value) VALUES($1,$2,$3,$4) ON CONFLICT(customer_id,preference_type) DO UPDATE SET preference_value=EXCLUDED.preference_value,revision=customer_quality_preferences.revision+1,updated_at=now() RETURNING id::text,customer_id::text,preference_type,preference_value,revision`, uuid.NewString(), customerID, typ, value).Scan(&p.ID, &p.CustomerID, &p.Type, &p.Value, &p.Revision)
	return p, err
}

func customerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Type != sharedauth.TokenTypeAccess || claims.Role != "customer" || claims.Service != "customer" || strings.TrimSpace(claims.Subject) == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(claims.Subject), true
}

func RegisterRoutes(root *gin.RouterGroup, store *Store, signer *sharedauth.TokenSigner) {
	if root == nil || store == nil || signer == nil {
		return
	}
	g := root.Group("/preferences")
	g.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	g.GET("", func(c *gin.Context) {
		id, ok := customerID(c)
		if !ok {
			return
		}
		values, err := store.List(c, id)
		if err != nil {
			httpx.Abort(c, apperrors.Internal("Preference lookup failed.", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": values})
	})
	g.PUT("/:type", func(c *gin.Context) {
		id, ok := customerID(c)
		if !ok {
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		if c.ShouldBindJSON(&body) != nil {
			httpx.Abort(c, apperrors.BadRequest("Preference value is required.", nil))
			return
		}
		value, err := store.Upsert(c, id, c.Param("type"), body.Value)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("Preference is invalid.", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
	})
}

// RegisterInternalRoutes exposes the read-only authority used by trusted
// platform services. The HMAC middleware is the only authentication path.
func RegisterInternalRoutes(root *gin.RouterGroup, store *Store, verifier *serviceauth.Verifier) {
	if root == nil || store == nil || verifier == nil {
		return
	}
	g := root.Group("/internal")
	g.Use(serviceauth.Middleware(verifier))
	g.GET("/customers/:customerId/quality-preferences", func(c *gin.Context) {
		values, err := store.List(c, strings.TrimSpace(c.Param("customerId")))
		if err != nil {
			httpx.RespondError(c, apperrors.Internal("Preference lookup failed.", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": values})
	})
}
