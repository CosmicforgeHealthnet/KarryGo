package supplypreview

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Previewer is implemented by an authority that can answer a planning
// question about its own supply.
//
// The signature returns an error only for genuine internal faults. An
// authority that could not reach its own availability index must return
// Unknown(...) with no error, because "I could not tell" is a legitimate,
// actionable answer and a 500 is not.
type Previewer interface {
	PreviewSupply(c *gin.Context, request Request) (Response, error)
}

// PreviewerFunc adapts a function to Previewer.
type PreviewerFunc func(*gin.Context, Request) (Response, error)

// PreviewSupply satisfies Previewer.
func (f PreviewerFunc) PreviewSupply(c *gin.Context, request Request) (Response, error) {
	return f(c, request)
}

// RegisterRoutes mounts the preview on an authority's already-authenticated
// internal commerce group.
//
// Like the capability contract it deliberately does not install its own auth:
// the caller passes a group it has ALREADY protected, so the preview inherits
// exactly the HMAC, allowlist and replay protection its neighbouring routes
// have rather than a second, weaker boundary invented here.
//
// The route is POST because the request carries a body, not because it
// mutates. It does not, and the handler has no way to: Previewer is handed a
// value and returns a value.
func RegisterRoutes(group *gin.RouterGroup, previewer Previewer) {
	if group == nil || previewer == nil {
		return
	}
	group.POST(Path, func(c *gin.Context) {
		var request Request
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false,
					"error": gin.H{"code": "validation_failed", "message": "Preview request is invalid."}})
				return
			}
		}
		response, err := previewer.PreviewSupply(c, request)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false,
				"error": gin.H{"code": "internal_error", "message": "Supply preview is unavailable."}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
	})
}
