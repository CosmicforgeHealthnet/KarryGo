// Package respond implements the corporate API success and error envelopes.
//
// Success:
//
//	{"data": ..., "meta": {"requestId": "..."}}
//
// Errors reuse shared apperrors codes and always carry the request ID:
//
//	{"error": {"code": "...", "message": "...", "details": [...], "requestId": "..."}}
//
// Error messages are safe public messages; stack traces, SQL, credentials and
// downstream response bodies are never included.
package respond

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Meta is attached to every success envelope.
type Meta struct {
	RequestID  string  `json:"requestId"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

type successEnvelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// ErrorBody is the wire shape of the error envelope.
type ErrorBody struct {
	Code      apperrors.Code             `json:"code"`
	Message   string                     `json:"message"`
	Details   []apperrors.FieldViolation `json:"details"`
	RequestID string                     `json:"requestId"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// OK writes a 200 success envelope.
func OK(c *gin.Context, data any) {
	JSON(c, http.StatusOK, data)
}

// Created writes a 201 success envelope.
func Created(c *gin.Context, data any) {
	JSON(c, http.StatusCreated, data)
}

// JSON writes a success envelope with the given status.
func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, successEnvelope{
		Data: data,
		Meta: Meta{RequestID: httpx.GetRequestID(c)},
	})
}

// List writes a list success envelope with an optional next cursor.
func List(c *gin.Context, data any, nextCursor string) {
	meta := Meta{RequestID: httpx.GetRequestID(c)}
	if nextCursor != "" {
		meta.NextCursor = &nextCursor
	}
	c.JSON(http.StatusOK, successEnvelope{Data: data, Meta: meta})
}

// Error writes the error envelope for err. 5xx causes are logged server-side
// (redacted to the error chain) and never serialized to the client.
func Error(c *gin.Context, err error) {
	if c.Writer.Written() {
		return
	}
	appErr := apperrors.From(err)
	requestID := httpx.GetRequestID(c)
	if appErr.Status >= http.StatusInternalServerError {
		log.Printf("request_id=%s status=%d code=%s error=%v", requestID, appErr.Status, appErr.Code, err)
	}
	details := appErr.Fields
	if details == nil {
		details = []apperrors.FieldViolation{}
	}
	c.AbortWithStatusJSON(appErr.Status, errorEnvelope{
		Error: ErrorBody{
			Code:      appErr.Code,
			Message:   appErr.Message,
			Details:   details,
			RequestID: requestID,
		},
	})
}

// ErrorHandler is the terminal middleware that converts errors attached to
// the gin context into the corporate error envelope.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		Error(c, c.Errors.Last().Err)
	}
}
