package respond

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

// Deprecated top-level aliases (FLEET-6h).
//
// Fleet's internal service routes used to answer with bespoke bodies such as
// {"status":"ACCEPTED","code":...}. They now answer with the standard
// envelopes above. A caller binary deployed before it learned to read the
// envelope may still decode a top-level field, so a route whose LIVE caller
// does that keeps the field as an alias next to "data" (or "error") in the
// same response. Every call site names the caller and the condition for
// removing its aliases; once every caller reads the envelope the aliases go.

// reservedKeys are envelope members an alias can never shadow.
var reservedKeys = map[string]bool{"data": true, "meta": true, "error": true}

// JSONWithAliases writes the standard success envelope plus deprecated
// top-level alias fields. Aliases never replace data, meta or error.
func JSONWithAliases(c *gin.Context, status int, data any, aliases map[string]any) {
	body := gin.H{"data": data, "meta": Meta{RequestID: httpx.GetRequestID(c)}}
	addAliases(body, aliases)
	c.JSON(status, body)
}

// ErrorWithAliases writes the standard error envelope for err plus deprecated
// top-level alias fields.
func ErrorWithAliases(c *gin.Context, err error, aliases map[string]any) {
	if c.Writer.Written() {
		return
	}
	status, errBody := errorBody(c, err)
	body := gin.H{"error": errBody}
	addAliases(body, aliases)
	c.AbortWithStatusJSON(status, body)
}

// AliasesOf flattens v's JSON object members into an alias map, for a route
// whose legacy body WAS the data object itself.
func AliasesOf(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func addAliases(body gin.H, aliases map[string]any) {
	for key, value := range aliases {
		if !reservedKeys[key] {
			body[key] = value
		}
	}
}
