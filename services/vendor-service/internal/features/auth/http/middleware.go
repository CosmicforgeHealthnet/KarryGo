package vendorauthhttp

import (
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"

	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
)

// RequireAuth verifies the bearer access token, then re-resolves the full
// staff/session/vendor context from the database on every request (never
// from JWT claims beyond the opaque session id) — see
// docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md for why this is
// deliberately more expensive than trusting embedded claims: it is what
// makes staff suspension and permission changes take effect on the very next
// request, not just at token expiry.
func RequireAuth(signer *sharedauth.TokenSigner, sessions *vendorauthrepositories.SessionRepository, staff vendorauthusecases.StaffResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := verifyBearer(c, signer)
		if !ok {
			return
		}

		session, err := sessions.GetByID(c.Request.Context(), claims.SessionID)
		if err != nil || !session.IsActive(time.Now()) || session.StaffID != claims.Subject {
			httpx.RespondError(c, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil))
			c.Abort()
			return
		}

		affiliation, err := staff.GetByID(c.Request.Context(), claims.Subject)
		if err != nil || !affiliation.Active {
			httpx.RespondError(c, apperrors.Forbidden("Your access has been suspended. Contact your business owner.", nil))
			c.Abort()
			return
		}

		authctx.Set(c, authctx.StaffContext{StaffID: affiliation.StaffID, VendorID: affiliation.VendorID, SessionID: claims.SessionID})
		c.Next()
	}
}

func verifyBearer(c *gin.Context, signer *sharedauth.TokenSigner) (sharedauth.Claims, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		c.Abort()
		return sharedauth.Claims{}, false
	}
	token := header
	const prefix = "Bearer "
	if len(header) > len(prefix) && header[:len(prefix)] == prefix {
		token = header[len(prefix):]
	}
	claims, err := signer.Verify(token)
	if err != nil {
		httpx.RespondError(c, apperrors.Unauthorized("Your session has expired. Please sign in again.", err))
		c.Abort()
		return sharedauth.Claims{}, false
	}
	if claims.Type != sharedauth.TokenTypeAccess || claims.Role != vendorauthusecases.AccessRole || claims.Service != vendorauthusecases.AccessService {
		httpx.RespondError(c, apperrors.Forbidden("You do not have access to this action.", nil))
		c.Abort()
		return sharedauth.Claims{}, false
	}
	return claims, true
}
