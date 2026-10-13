package logx

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestMaskEmail(t *testing.T) {
	tests := map[string]string{
		"andi@example.com":    "a***@e***.com",
		"x@mail.co.id":        "x***@m***.id",
		"not-an-email":        Redacted,
		"@example.com":        Redacted,
		"someone@localhost":   "s***@l***",
		"Sarah.W@Example.COM": "S***@E***.COM",
	}
	for in, want := range tests {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrub(t *testing.T) {
	tests := []struct{ in, mustNotContain, mustContain string }{
		{"connect postgres://cashback:s3cret@db:5432/cashback failed", "s3cret", "postgres://cashback:" + Redacted + "@db"},
		{"redis://:hunter2@redis:6379/0 refused", "hunter2", Redacted},
		{"failed to connect to `host=db user=cashback password=s3cret dbname=x`", "s3cret", "password=" + Redacted},
		{"upstream said: Authorization: Bearer abc.def.ghi", "abc.def.ghi", Redacted},
		{`no account for "andi@example.com"`, "andi@example.com", "a***@e***.com"},
		{"api_key='k-123' rejected", "k-123", Redacted},
	}
	for _, tt := range tests {
		got := Scrub(tt.in)
		if strings.Contains(got, tt.mustNotContain) || !strings.Contains(got, tt.mustContain) {
			t.Errorf("Scrub(%q) = %q; must drop %q and contain %q", tt.in, got, tt.mustNotContain, tt.mustContain)
		}
	}
}

// Every record goes through redaction, including attrs added with With,
// nested groups and error values.
func TestLoggerRedacts(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelDebug).With("request_id", "r1", "authorization", "Bearer xyz")

	log.Info("sign in rejected",
		"email", "andi@example.com",
		"user_id", "11111111-1111-4111-8111-111111111111",
		slog.Group("headers", "X-User-ID", "11111111-1111-4111-8111-111111111111", "Cookie", "sid=abc"),
		"error", errors.New("dial postgres://u:pw@db/x: andi@example.com"),
		Err(errors.New("password=pw2 bad")),
	)
	out := buf.String()

	for _, leak := range []string{"xyz", "andi@example.com", "sid=abc", ":pw@", "pw2"} {
		if strings.Contains(out, leak) {
			t.Errorf("log leaked %q:\n%s", leak, out)
		}
	}
	for _, want := range []string{`"request_id":"r1"`, `"email":"a***@e***.com"`, `"user_id":"11111111-1111-4111-8111-111111111111"`,
		`"X-User-ID":"[REDACTED]"`, `"authorization":"[REDACTED]"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %s:\n%s", want, out)
		}
	}
}

// Library messages become JSON records through the default logger, scrubbed.
func TestLibrary(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(New(&buf, slog.LevelInfo))
	defer slog.SetDefault(prev)

	Library{Name: "redis"}.Printf(context.Background(), "dial %s: %v", "redis://:hunter2@redis:6379/0", errors.New("refused"))
	out := buf.String()

	if strings.Contains(out, "hunter2") {
		t.Errorf("library log leaked the password:\n%s", out)
	}
	for _, want := range []string{`"level":"WARN"`, `"library":"redis"`, `"detail":"dial redis://:[REDACTED]@redis:6379/0: refused"`} {
		if !strings.Contains(out, want) {
			t.Errorf("library log missing %s:\n%s", want, out)
		}
	}
}

func TestParseLevel(t *testing.T) {
	if ParseLevel("debug") != slog.LevelDebug || ParseLevel("WARN") != slog.LevelWarn || ParseLevel("nonsense") != slog.LevelInfo || ParseLevel("") != slog.LevelInfo {
		t.Error("ParseLevel mapped levels wrongly")
	}
}
