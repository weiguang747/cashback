// Package idempotency makes retried requests safe: the first request with
// an Idempotency-Key runs, and later ones with the same key get its stored
// response instead of running again.
package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Outcome says what Begin found for a key.
type Outcome int

const (
	// Started: the key is new and now locked; run the request, then call
	// Complete (or Release if it failed unexpectedly).
	Started Outcome = iota
	// Replay: an earlier request with this key finished; send its Response.
	Replay
	// InProgress: an earlier request with this key is still running.
	InProgress
	// Mismatch: the key was used before with a different request body.
	Mismatch
)

// Response is a stored HTTP response.
type Response struct {
	Status int
	Body   []byte
}

type Result struct {
	Outcome  Outcome
	Response Response // set for Replay
}

// Store locks keys and remembers responses. Scope separates keys of
// different users, so one user can never replay another's response.
type Store interface {
	Begin(ctx context.Context, scope, key, fingerprint string) (Result, error)
	Complete(ctx context.Context, scope, key, fingerprint string, resp Response) error
	Release(ctx context.Context, scope, key string) error
}

const (
	// pendingTTL bounds how long a crashed request can block its key.
	pendingTTL = time.Minute
	// doneTTL is how long a finished response can be replayed.
	doneTTL = 24 * time.Hour
)

type record struct {
	State       string `json:"state"` // "pending" or "done"
	Fingerprint string `json:"fingerprint"`
	Status      int    `json:"status,omitempty"`
	Body        string `json:"body,omitempty"`
}

// Redis is a Store backed by Redis.
type Redis struct{ client *redis.Client }

func NewRedis(client *redis.Client) *Redis { return &Redis{client: client} }

func redisKey(scope, key string) string { return "idem:" + scope + ":" + key }

func (r *Redis) Begin(ctx context.Context, scope, key, fingerprint string) (Result, error) {
	k := redisKey(scope, key)
	pending, _ := json.Marshal(record{State: "pending", Fingerprint: fingerprint})

	// Two tries: the key can expire between a failed SETNX and the GET.
	for range 2 {
		ok, err := r.client.SetNX(ctx, k, pending, pendingTTL).Result()
		if err != nil {
			return Result{}, fmt.Errorf("idempotency lock: %w", err)
		}
		if ok {
			return Result{Outcome: Started}, nil
		}

		raw, err := r.client.Get(ctx, k).Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return Result{}, fmt.Errorf("idempotency read: %w", err)
		}
		var rec record
		if err := json.Unmarshal(raw, &rec); err != nil {
			return Result{}, fmt.Errorf("idempotency record: %w", err)
		}
		switch {
		case rec.Fingerprint != fingerprint:
			return Result{Outcome: Mismatch}, nil
		case rec.State == "done":
			return Result{Outcome: Replay, Response: Response{Status: rec.Status, Body: []byte(rec.Body)}}, nil
		default:
			return Result{Outcome: InProgress}, nil
		}
	}
	return Result{}, errors.New("idempotency lock: key kept expiring")
}

func (r *Redis) Complete(ctx context.Context, scope, key, fingerprint string, resp Response) error {
	done, _ := json.Marshal(record{State: "done", Fingerprint: fingerprint, Status: resp.Status, Body: string(resp.Body)})
	return r.client.Set(ctx, redisKey(scope, key), done, doneTTL).Err()
}

func (r *Redis) Release(ctx context.Context, scope, key string) error {
	return r.client.Del(ctx, redisKey(scope, key)).Err()
}
