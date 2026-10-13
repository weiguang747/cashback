// Package logx is the server's structured logging: JSON lines via log/slog,
// one logger per request (carrying request_id and, once known, user_id),
// and redaction of sensitive values applied to every record.
//
// Redaction is a safety net, not a licence: still don't log request bodies,
// headers, balances or anything you wouldn't show on a support ticket.
package logx

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

// Attribute keys whose values are always replaced, at any nesting level.
var secretKeys = map[string]bool{
	"password":      true,
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"secret":        true,
	"api_key":       true,
	"x-user-id":     true, // the demo credential header; log user_id instead
}

// Attribute keys holding an email address, which is masked rather than dropped.
var emailKeys = map[string]bool{"email": true, "user_email": true}

var (
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// scheme://user:password@host → scheme://user:[REDACTED]@host
	urlPasswordPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://[^:/\s@]*):[^@/\s]+@`)
	// key=value or key: value pairs that look like credentials (libpq DSNs, headers).
	// An Authorization value can be a scheme plus a credential ("Bearer x"), so
	// it takes the rest of the line rather than one word.
	kvSecretPattern   = regexp.MustCompile(`(?i)\b(password|token|secret|api[_-]?key)(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)
	authHeaderPattern = regexp.MustCompile(`(?i)\b(authorization\s*[=:]\s*)[^\r\n,;]+`)
	bearerPattern     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/\-]+=*`)
)

// New returns a JSON logger writing to w at the given level, with redaction.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redact,
	}))
}

// ParseLevel reads LOG_LEVEL-style values (debug, info, warn, error); anything
// else means info.
func ParseLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.TrimSpace(s))); err != nil {
		return slog.LevelInfo
	}
	return l
}

func redact(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	switch {
	case secretKeys[key]:
		return slog.String(a.Key, Redacted)
	case emailKeys[key]:
		return slog.String(a.Key, MaskEmail(a.Value.String()))
	case a.Value.Kind() == slog.KindString:
		return slog.String(a.Key, Scrub(a.Value.String()))
	case a.Value.Kind() == slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, Scrub(err.Error()))
		}
	}
	return a
}

// Scrub removes credentials and email addresses from free text such as
// error messages, which can embed connection strings or user input.
func Scrub(s string) string {
	s = urlPasswordPattern.ReplaceAllString(s, "${1}:"+Redacted+"@")
	s = bearerPattern.ReplaceAllString(s, "Bearer "+Redacted)
	s = authHeaderPattern.ReplaceAllString(s, "${1}"+Redacted)
	s = kvSecretPattern.ReplaceAllString(s, "${1}${2}"+Redacted)
	return emailPattern.ReplaceAllStringFunc(s, MaskEmail)
}

// MaskEmail keeps just enough to tell addresses apart when debugging:
// "andi@example.com" → "a***@e***.com".
func MaskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return Redacted
	}
	host, tld := domain, ""
	if i := strings.LastIndex(domain, "."); i > 0 {
		host, tld = domain[:i], domain[i:]
	}
	return local[:1] + "***@" + host[:1] + "***" + tld
}

// Err is the attribute for an error, scrubbed like any other string.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "")
	}
	return slog.String("error", Scrub(err.Error()))
}

type ctxKey struct{}

// WithLogger returns ctx carrying logger.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// From returns the request's logger, or the default logger outside a request.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
