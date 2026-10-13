// Package httpx holds HTTP helpers shared by every endpoint, so all
// responses follow the same conventions.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/logx"
)

// StatusClientClosedRequest records a request the client gave up on before
// it was answered (nginx's 499), e.g. after the app's timeout. No one reads
// the response; the status is for the access log.
const StatusClientClosedRequest = 499

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

// ClientGone reports whether the client has disconnected, which cancels the
// request context and makes any database or Redis call in flight fail.
func ClientGone(c *gin.Context) bool {
	return errors.Is(c.Request.Context().Err(), context.Canceled)
}

// InternalError ends a request that failed unexpectedly: it logs err and
// returns a generic 500 without internal details. If the client already
// disconnected, the failure is only the cancellation, not a server fault, so
// it is logged at info with client_canceled and recorded as 499.
func InternalError(c *gin.Context, msg string, err error) {
	log := logx.From(c.Request.Context())
	if ClientGone(c) {
		log.Info(msg, slog.Bool("client_canceled", true), logx.Err(err))
		Error(c, StatusClientClosedRequest, "request canceled")
		return
	}
	log.Error(msg, logx.Err(err))
	Error(c, http.StatusInternalServerError, "something went wrong")
}
