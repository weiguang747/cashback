// Package httpx holds HTTP helpers shared by every endpoint, so all
// responses follow the same conventions.
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is the body of every failed request: {"error": "..."}.
// The app reads this field to show a message to the user.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Error aborts the request and writes a JSON error body.
func Error(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: message})
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

// Recovery returns a 500 JSON error instead of an empty body when a handler panics.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		Error(c, http.StatusInternalServerError, "something went wrong")
	})
}
