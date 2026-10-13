package main

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cashback/server/internal/httpx"
	"cashback/server/internal/logx"
)

type user struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

const userKey = "user"

// requireUser loads the signed-in user from the X-User-ID header and stops
// the request with 401 if there isn't one.
//
// This is a demo stand-in for auth: anyone who knows a user's id can act as
// them. Replace this one function with a token check when real auth lands.
//
// Users it has loaded are cached briefly (see userCache), so most requests
// skip the database lookup.
func requireUser(s store, users *userCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.GetHeader("X-User-ID")
		// Check the format first so a bad value can't reach Postgres as a 500.
		// The header is the demo credential, so it's never logged when rejected.
		if _, err := uuid.Parse(id); err != nil {
			reason := "malformed_user_id"
			if id == "" {
				reason = "missing_user_id"
			}
			logx.From(ctx).Warn("auth rejected", "reason", reason)
			httpx.Error(c, http.StatusUnauthorized, "sign in required")
			return
		}

		if u, ok := users.get(id); ok {
			setCurrentUser(c, u)
			return
		}
		row, err := s.GetUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			logx.From(ctx).Warn("auth rejected", "reason", "unknown_user")
			httpx.Error(c, http.StatusUnauthorized, "sign in required")
			return
		}
		if err != nil {
			httpx.InternalError(c, "get user failed", err)
			return
		}

		u := user{ID: row.ID, Email: row.Email, Name: row.Name}
		users.put(u)
		setCurrentUser(c, u)
	}
}

// setCurrentUser marks the request as the user's and runs the rest of the chain.
func setCurrentUser(c *gin.Context, u user) {
	c.Set(userKey, u)
	// Every later log line for this request, including the access log,
	// says whose request it was.
	httpx.AddToLogger(c, "user_id", u.ID)
	c.Next()
}

const (
	// userCacheTTL is how long requireUser trusts a user it already loaded.
	// Users are never deleted while the server runs (only by re-creating the
	// database), so this only delays noticing that.
	userCacheTTL = 30 * time.Second
	// userCacheMax bounds memory. Only users that exist are cached, so
	// random ids can't fill it.
	userCacheMax = 10_000
)

// userCache remembers recently loaded users by id, for a short time.
type userCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	now     func() time.Time
	entries map[string]cachedUser
}

type cachedUser struct {
	user    user
	expires time.Time
}

func newUserCache(ttl time.Duration, max int) *userCache {
	return &userCache{ttl: ttl, max: max, now: time.Now, entries: map[string]cachedUser{}}
}

func (uc *userCache) get(id string) (user, bool) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	e, ok := uc.entries[id]
	if !ok || !uc.now().Before(e.expires) {
		return user{}, false
	}
	return e.user, true
}

func (uc *userCache) put(u user) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	now := uc.now()
	if len(uc.entries) >= uc.max {
		for id, e := range uc.entries {
			if !now.Before(e.expires) {
				delete(uc.entries, id)
			}
		}
		// Still full of live entries: start over rather than grow.
		if len(uc.entries) >= uc.max {
			clear(uc.entries)
		}
	}
	uc.entries[u.ID] = cachedUser{user: u, expires: now.Add(uc.ttl)}
}

// currentUser returns the user that requireUser stored on the request.
func currentUser(c *gin.Context) user {
	return c.MustGet(userKey).(user)
}
