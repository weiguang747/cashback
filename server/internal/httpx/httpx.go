// Package httpx holds HTTP helpers shared by every endpoint, so all
// responses follow the same conventions.
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is the body of every failed request: {"error": "..."}.
// The app reads Error to show a message to the user, and Code, when set,
// to react to a specific failure such as "insufficient_funds".
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// Error aborts the request and writes a JSON error body.
func Error(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: message})
}

// ErrorCode is Error with a machine-readable code the app can branch on.
func ErrorCode(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: message, Code: code})
}

// UseJSONErrors makes the router's own failures (unknown route, wrong
// method, panic) return the same JSON error shape as handlers do.
func UseJSONErrors(r *gin.Engine) {
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		Error(c, http.StatusNotFound, "not found")
	})
	r.NoMethod(func(c *gin.Context) {
		Error(c, http.StatusMethodNotAllowed, "method not allowed")
	})
}
