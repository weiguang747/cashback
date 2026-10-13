package httpx

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cashback/server/internal/logx"
)

// RequestIDHeader carries a request's id both ways. The app can send one to
// correlate its own logs; the server always returns the id it used, so a
// user-reported error can be found in the server logs.
const RequestIDHeader = "X-Request-ID"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// RequestLogger gives every request a request id and a logger carrying it
// (in the request context, see logx.From), then writes one access log line
// when the request finishes.
//
// It logs the route template (/v1/payments), not headers, bodies or query
// strings, and only a truncated client IP.
func RequestLogger(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		id := c.GetHeader(RequestIDHeader)
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewString()
		}
		c.Header(RequestIDHeader, id)

		logger := base.With("request_id", id)
		c.Request = c.Request.WithContext(logx.WithLogger(c.Request.Context(), logger))

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case route == "/health" && status < 500:
			level = slog.LevelDebug // polled by hosting; not worth a line each time
		case status >= 500:
			level = slog.LevelError
		case status == 401 || status == 403 || status == 429:
			level = slog.LevelWarn
		}
		// Use the request's final logger: requireUser adds user_id to it.
		logx.From(c.Request.Context()).LogAttrs(c.Request.Context(), level, "http request",
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.Int("response_bytes", c.Writer.Size()),
			slog.String("client_ip", truncateIP(c.ClientIP())),
		)
	}
}

// AddToLogger adds attributes to the request's logger for every later log
// line of this request, including its access log.
func AddToLogger(c *gin.Context, args ...any) {
	ctx := c.Request.Context()
	c.Request = c.Request.WithContext(logx.WithLogger(ctx, logx.From(ctx).With(args...)))
}

// truncateIP keeps the network, not the host: 203.0.113.42 → 203.0.113.0,
// IPv6 to its /48. Enough to spot abuse from one network, not to identify a
// person.
func truncateIP(s string) string {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		if host, _, splitErr := net.SplitHostPort(s); splitErr == nil {
			return truncateIP(host)
		}
		return ""
	}
	bits := 48
	if addr.Is4() {
		bits = 24
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.Addr().String()
}

// Recovery returns a 500 JSON error instead of an empty body when a handler
// panics, and logs the panic with its stack and the request id.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		logx.From(c.Request.Context()).Error("panic",
			slog.String("panic", logx.Scrub(fmt.Sprint(recovered))),
			slog.String("stack", string(debug.Stack())),
		)
		Error(c, http.StatusInternalServerError, "something went wrong")
	})
}
