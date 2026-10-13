package main

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cashback/server/internal/httpx"
)

// Lists are paged by cursor, newest first: ?limit=N (default 20, at most
// 100) and ?cursor= the previous page's nextCursor. A cursor marks a row by
// (created_at, id), so rows added while the user scrolls never shift the
// next page the way an offset would.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// page is a parsed limit and cursor. limit is one more than the page size,
// so the query result says whether there is a next page.
type page struct {
	size            int
	limit           int32
	beforeCreatedAt *time.Time
	beforeID        *string
}

// pageParams reads limit and cursor, responding 400 and returning false
// when either is invalid.
func pageParams(c *gin.Context) (page, bool) {
	p := page{size: defaultPageSize}
	if q := c.Query("limit"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > maxPageSize {
			httpx.Error(c, http.StatusBadRequest, "limit must be a whole number from 1 to "+strconv.Itoa(maxPageSize))
			return page{}, false
		}
		p.size = n
	}
	p.limit = int32(p.size + 1)

	if raw := c.Query("cursor"); raw != "" {
		at, id, ok := decodeCursor(raw)
		if !ok {
			httpx.ErrorCode(c, http.StatusBadRequest, "invalid_cursor", "cursor is not valid; start again from the first page")
			return page{}, false
		}
		p.beforeCreatedAt, p.beforeID = &at, &id
	}
	return p, true
}

// trim cuts rows to the page size and returns the cursor for the next page,
// or nil when this is the last one. key gives a row's (created_at, id).
func trim[T any](p page, rows []T, key func(T) (time.Time, string)) ([]T, *string) {
	if len(rows) <= p.size {
		return rows, nil
	}
	rows = rows[:p.size]
	cursor := encodeCursor(key(rows[len(rows)-1]))
	return rows, &cursor
}

// A cursor is opaque to clients: base64url of "<unix microseconds>_<uuid>".
// Postgres keeps microseconds, so the time round-trips exactly.
func encodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + "_" + id))
}

func decodeCursor(raw string) (time.Time, string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", false
	}
	micros, id, found := strings.Cut(string(b), "_")
	if !found {
		return time.Time{}, "", false
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	// Checked here so a forged cursor can't reach Postgres as a 500.
	if _, err := uuid.Parse(id); err != nil {
		return time.Time{}, "", false
	}
	return time.UnixMicro(n), id, true
}
