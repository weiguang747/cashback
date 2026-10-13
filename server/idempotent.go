package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
	"cashback/server/internal/idempotency"
)

// idempotencyKey reads the required Idempotency-Key header, responding 400
// and returning false when it's missing or malformed.
func idempotencyKey(c *gin.Context) (string, bool) {
	key := c.GetHeader("Idempotency-Key")
	if !idempotencyKeyPattern.MatchString(key) {
		httpx.ErrorCode(c, http.StatusBadRequest, "invalid_idempotency_key",
			"Idempotency-Key header must be 8-128 letters, digits, - or _")
		return "", false
	}
	return key, true
}

// outcome is a money-moving handler's result: sent now, and remembered so a
// retry with the same key gets exactly the same response.
type outcome struct {
	Status   int
	Body     any
	Replayed bool // the database already had this key; nothing new happened
}

// runIdempotent runs a request that moves money at most once per key. Redis
// locks the key and caches the response; run must also be idempotent in the
// database (a unique key on the transactions it writes), which covers the
// case where Redis loses the key. An error from run is unexpected: the key
// is released so the client can retry.
func runIdempotent(c *gin.Context, idem idempotency.Store, key, fingerprint string, run func() (outcome, error)) {
	ctx := c.Request.Context()
	scope := currentUser(c).ID

	begin, err := idem.Begin(ctx, scope, key, fingerprint)
	if err != nil {
		// Fail closed: never move money without the duplicate guard.
		log.Printf("idempotency begin: %v", err)
		httpx.ErrorCode(c, http.StatusServiceUnavailable, "unavailable", "this is unavailable right now, please try again")
		return
	}
	switch begin.Outcome {
	case idempotency.Replay:
		c.Header("Idempotent-Replayed", "true")
		c.Data(begin.Response.Status, gin.MIMEJSON, begin.Response.Body)
		return
	case idempotency.InProgress:
		httpx.ErrorCode(c, http.StatusConflict, "in_progress", "this request is already being processed")
		return
	case idempotency.Mismatch:
		httpx.ErrorCode(c, http.StatusUnprocessableEntity, "idempotency_key_reused", "this Idempotency-Key was already used for a different request")
		return
	}

	// From here on, record the outcome even if the client disconnects.
	bg := context.WithoutCancel(ctx)

	out, err := run()
	if err != nil {
		log.Printf("%s %s: %v", c.Request.Method, c.FullPath(), err)
		if err := idem.Release(bg, scope, key); err != nil {
			log.Printf("idempotency release: %v", err)
		}
		httpx.Error(c, http.StatusInternalServerError, "something went wrong")
		return
	}

	raw, err := json.Marshal(out.Body)
	if err != nil {
		log.Printf("encode response: %v", err)
		httpx.Error(c, http.StatusInternalServerError, "something went wrong")
		return
	}
	// A failed cache write is only logged: the database's unique key still
	// stops a retry from moving money twice.
	if err := idem.Complete(bg, scope, key, fingerprint, idempotency.Response{Status: out.Status, Body: raw}); err != nil {
		log.Printf("idempotency complete: %v", err)
	}
	if out.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.Data(out.Status, gin.MIMEJSON, raw)
}

// keyReused is the response when the database finds a key already used
// for a different request.
var keyReused = outcome{
	Status: http.StatusUnprocessableEntity,
	Body:   httpx.ErrorResponse{Code: "idempotency_key_reused", Error: "this Idempotency-Key was already used for a different request"},
}
