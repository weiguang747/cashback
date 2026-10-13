// Package httpx holds HTTP helpers shared by every endpoint, so all
// responses follow the same conventions.
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

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

// Timeout gives each request a deadline. Database and Redis calls take the
// request context, so a query stuck behind a lock or a slow network stops
// at the deadline and releases its pool connection, instead of piling up
// until the pool is exhausted. Handlers that fail this way answer 503
// "timeout" (see InternalError).
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// TimedOut reports whether the request ran past its Timeout deadline.
func TimedOut(c *gin.Context) bool {
	return errors.Is(c.Request.Context().Err(), context.DeadlineExceeded)
}

// ClientGone reports whether the client has disconnected, which cancels the
// request context and makes any database or Redis call in flight fail.
func ClientGone(c *gin.Context) bool {
	return errors.Is(c.Request.Context().Err(), context.Canceled)
}

// InternalError ends a request that failed unexpectedly: it logs err and
// returns a generic 500 without internal details. If the client already
// disconnected, the failure is only the cancellation, not a server fault, so
// it is logged at info with client_canceled and recorded as 499. If the
// request ran out of time, it is a server fault (logged at error with
// timed_out) but a temporary one: 503 "timeout", so clients retry.
func InternalError(c *gin.Context, msg string, err error) {
	log := logx.From(c.Request.Context())
	if ClientGone(c) {
		log.Info(msg, slog.Bool("client_canceled", true), logx.Err(err))
		Error(c, StatusClientClosedRequest, "request canceled")
		return
	}
	if TimedOut(c) {
		log.Error(msg, slog.Bool("timed_out", true), logx.Err(err))
		ErrorCode(c, http.StatusServiceUnavailable, "timeout", "the server took too long to respond, please try again")
		return
	}
	log.Error(msg, logx.Err(err))
	Error(c, http.StatusInternalServerError, "something went wrong")
}
