package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIError is the structured error type that handlers emit via c.Error().
// The errorMiddleware renders it to a uniform JSON shape.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *APIError) Error() string { return e.Message }

// With attaches a key/value pair that will be merged into the response body.
func (e *APIError) With(k string, v any) *APIError {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[k] = v
	return e
}

func ErrBadRequest(msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func ErrNotFound(what string) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: what + " not found"}
}

func ErrConflict(code, msg string) *APIError {
	return &APIError{Status: http.StatusConflict, Code: code, Message: msg}
}

func ErrInternal() *APIError {
	return &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal server error"}
}

// errorMiddleware renders any APIError pushed onto c.Errors into a JSON body
// with shape {"error": ..., "code": ..., ...details}. Unknown errors render as
// 500 internal. No-error requests pass through untouched.
func (h *Handler) errorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		last := c.Errors.Last().Err

		var apiErr *APIError
		if errors.As(last, &apiErr) {
			body := gin.H{"error": apiErr.Message, "code": apiErr.Code}
			for k, v := range apiErr.Details {
				body[k] = v
			}
			c.AbortWithStatusJSON(apiErr.Status, body)
			return
		}
		h.Logger.Error("unhandled handler error", "err", last)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
			"code":  "internal",
		})
	}
}
