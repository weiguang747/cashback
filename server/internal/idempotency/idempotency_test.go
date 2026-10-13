package idempotency

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Runs against a real Redis only when TEST_REDIS_URL is set, e.g.
//
//	TEST_REDIS_URL=redis://localhost:6379/15 go test ./...
func newTestRedis(t *testing.T) (*Redis, *redis.Client) {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { client.Close() })
	return NewRedis(client), client
}

func TestRedisLifecycle(t *testing.T) {
	r, client := newTestRedis(t)
	ctx := context.Background()
	scope, key := "test-user", "key-"+time.Now().Format("150405.000000000")
	t.Cleanup(func() { client.Del(ctx, redisKey(scope, key)) })

	res, err := r.Begin(ctx, scope, key, "fp1")
	if err != nil || res.Outcome != Started {
		t.Fatalf("first Begin = %+v, %v; want Started", res, err)
	}
	if ttl := client.TTL(ctx, redisKey(scope, key)).Val(); ttl <= 0 || ttl > pendingTTL {
		t.Errorf("pending TTL = %v, want within %v", ttl, pendingTTL)
	}

	if res, _ := r.Begin(ctx, scope, key, "fp1"); res.Outcome != InProgress {
		t.Errorf("Begin while pending = %v, want InProgress", res.Outcome)
	}
	if res, _ := r.Begin(ctx, scope, key, "fp2"); res.Outcome != Mismatch {
		t.Errorf("Begin with other fingerprint = %v, want Mismatch", res.Outcome)
	}

	if err := r.Complete(ctx, scope, key, "fp1", Response{Status: 201, Body: []byte(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}
	if ttl := client.TTL(ctx, redisKey(scope, key)).Val(); ttl <= pendingTTL {
		t.Errorf("done TTL = %v, want about %v", ttl, doneTTL)
	}
	res, err = r.Begin(ctx, scope, key, "fp1")
	if err != nil || res.Outcome != Replay || res.Response.Status != 201 || string(res.Response.Body) != `{"ok":true}` {
		t.Errorf("Begin after Complete = %+v, %v; want Replay of the stored response", res, err)
	}

	// Another user's identical key is independent.
	if res, _ := r.Begin(ctx, "other-user", key, "fp1"); res.Outcome != Started {
		t.Errorf("other scope = %v, want Started", res.Outcome)
	}
	client.Del(ctx, redisKey("other-user", key))

	if err := r.Release(ctx, scope, key); err != nil {
		t.Fatal(err)
	}
	if res, _ := r.Begin(ctx, scope, key, "fp1"); res.Outcome != Started {
		t.Errorf("Begin after Release = %v, want Started", res.Outcome)
	}
}
