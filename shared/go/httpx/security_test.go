package httpx

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/gin-gonic/gin"
)

func TestMiddlewareNeverLogsCredentialQueryBodyHeaderPanicOrCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	before := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(before) })
	for _, path := range []string{"/panic", "/error"} {
		r := gin.New()
		r.Use(RequestID(), Logger(), Recovery(), ErrorHandler())
		r.POST("/panic", func(c *gin.Context) { panic("SECRET_PANIC") })
		r.POST("/error", func(c *gin.Context) { Abort(c, apperrors.Internal("Unavailable.", errors.New("SECRET_CAUSE"))) })
		req := httptest.NewRequest(http.MethodPost, path+"?otp=SECRET_QUERY", strings.NewReader(`{"otp":"SECRET_BODY"}`))
		req.Header.Set("Authorization", "Bearer SECRET_HEADER")
		req.Header.Set("X-Request-ID", "SECRET_REQUEST_ID")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		if out.Code != 500 {
			t.Fatalf("status=%d", out.Code)
		}
		for _, secret := range []string{"SECRET_PANIC", "SECRET_CAUSE", "SECRET_QUERY", "SECRET_BODY", "SECRET_HEADER"} {
			if strings.Contains(out.Body.String(), secret) {
				t.Fatal("response exposed secret")
			}
		}
	}
	if strings.Contains(logs.String(), "SECRET_") {
		t.Fatal("middleware exposed secret")
	}
	if !strings.Contains(logs.String(), "http_error") || !strings.Contains(logs.String(), "http_panic") {
		t.Fatal("safe diagnostics missing")
	}
}
