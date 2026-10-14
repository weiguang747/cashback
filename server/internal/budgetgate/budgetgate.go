// Package budgetgate reserves cash back against a campaign's budget in Redis,
// so payments don't queue on the campaign row lock. Postgres stays the
// source of truth: a reservation only becomes money when the payment
// commits a cashback_pending row and the award worker credits it. Redis is
// the fast gate in front of that, and the reconciler keeps it honest.
//
// Per campaign, Redis holds (all keys share the hash tag {campaignID}, so
// each script touches one Redis Cluster slot):
//
//	campaign:{id}:remaining      budget not yet reserved; missing = closed
//	campaign:{id}:grant:<pid>    the amount reserved for a payment, or "released"
//	campaign:{id}:inflight       hash: payment id -> "amount:reserved_at_unix"
//
// Only the reconciler removes committed reservations from inflight (Prune).
// That's what lets it compare Redis with the database exactly: every
// reservation since its last pass is still listed.
package budgetgate

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Gate is what a payment needs. A sharded budget (N counters for one hot
// campaign) would be another implementation of it.
type Gate interface {
	// Reserve takes up to want from the budget for a payment, rounded down
	// to a multiple of increment. When less than want is left it grants
	// what's left, like the database path, so the budget ends at zero.
	Reserve(ctx context.Context, campaignID, paymentID string, want, increment int64) (Grant, error)
	// Release returns a reservation whose payment never committed. It is
	// safe to call more than once and for a payment that has no reservation.
	Release(ctx context.Context, campaignID, paymentID string) error
}

type Outcome int

const (
	Granted Outcome = iota + 1
	// Exhausted: less than one increment is left.
	Exhausted
	// Duplicate: this payment already reserved (Amount), or was released (0).
	Duplicate
	// Closed: the campaign has no counter (never seeded, or closed for a
	// switch or for ending the campaign). Grant nothing.
	Closed
)

func (o Outcome) String() string {
	switch o {
	case Granted:
		return "granted"
	case Exhausted:
		return "exhausted"
	case Duplicate:
		return "duplicate"
	case Closed:
		return "closed"
	}
	return "unknown"
}

type Grant struct {
	Outcome Outcome
	Amount  int64 // reserved (Granted or Duplicate), in minor units
	Left    int64 // budget left after this reservation (Granted)
}

// Inflight is a reservation the reconciler hasn't yet seen committed.
type Inflight struct {
	Amount     int64
	ReservedAt time.Time // Redis's clock
}

// Snapshot is one campaign's Redis state, read atomically.
type Snapshot struct {
	Open      bool  // the counter exists
	Remaining int64 // meaningful when Open
	Now       time.Time
	Inflight  map[string]Inflight
}

// grantTTL is how long a payment's grant is remembered: far longer than a
// payment can take, so Duplicate and Release always find it.
const grantTTL = 7 * 24 * time.Hour

func remainingKey(campaignID string) string { return "campaign:{" + campaignID + "}:remaining" }
func inflightKey(campaignID string) string  { return "campaign:{" + campaignID + "}:inflight" }
func lockKey(campaignID string) string      { return "campaign:{" + campaignID + "}:reconcile" }
func grantKey(campaignID, paymentID string) string {
	return "campaign:{" + campaignID + "}:grant:" + paymentID
}

// The scripts keep amounts as strings whenever they go back into Redis:
// Lua numbers are doubles, and Redis turns them into strings with 14
// significant digits. Amounts and budgets are far below 2^53, so the
// arithmetic itself is exact; %.0f prints it exactly.
var (
	reserveScript = redis.NewScript(`
local g = redis.call('GET', KEYS[2])
if g then
  if g == 'released' then return {'duplicate', 0, 0} end
  return {'duplicate', tonumber(g), 0}
end
local r = redis.call('GET', KEYS[1])
if not r then return {'closed', 0, 0} end
r = tonumber(r)
local want, inc = tonumber(ARGV[2]), tonumber(ARGV[3])
local amt = math.min(want, r)
if inc > 1 then amt = amt - (amt % inc) end
if amt <= 0 then return {'exhausted', 0, r} end
local s = string.format('%.0f', amt)
local left = redis.call('DECRBY', KEYS[1], s)
redis.call('SET', KEYS[2], s, 'EX', ARGV[4])
local now = redis.call('TIME')
redis.call('HSET', KEYS[3], ARGV[1], s .. ':' .. now[1])
return {'granted', amt, left}
`)

	// Gives the amount back only while the counter exists: after Close, the
	// budget stays in the database for EndCampaign or a reseed to account for.
	releaseScript = redis.NewScript(`
local g = redis.call('GET', KEYS[2])
if not g or g == 'released' then return 0 end
redis.call('SET', KEYS[2], 'released', 'KEEPTTL')
redis.call('HDEL', KEYS[3], ARGV[1])
if redis.call('EXISTS', KEYS[1]) == 1 then redis.call('INCRBY', KEYS[1], g) end
return 1
`)

	snapshotScript = redis.NewScript(`
return {redis.call('GET', KEYS[1]) or false, redis.call('TIME')[1], redis.call('HGETALL', KEYS[2])}
`)

	// Corrects drift. INCRBY rather than SET, so reservations made since the
	// snapshot keep their effect.
	adjustScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return false end
return redis.call('INCRBY', KEYS[1], ARGV[1])
`)

	seedScript = redis.NewScript(`
redis.call('SET', KEYS[1], ARGV[1])
redis.call('DEL', KEYS[2])
return 1
`)

	unlockScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0
`)
)

// Redis is the Gate backed by one Redis counter per campaign.
type Redis struct{ client *redis.Client }

func NewRedis(client *redis.Client) *Redis { return &Redis{client: client} }

func (r *Redis) Reserve(ctx context.Context, campaignID, paymentID string, want, increment int64) (Grant, error) {
	if want <= 0 {
		return Grant{Outcome: Exhausted}, nil
	}
	res, err := reserveScript.Run(ctx, r.client,
		[]string{remainingKey(campaignID), grantKey(campaignID, paymentID), inflightKey(campaignID)},
		paymentID, want, max(increment, 1), int64(grantTTL/time.Second)).Slice()
	if err != nil {
		return Grant{}, fmt.Errorf("budget reserve: %w", err)
	}
	if len(res) != 3 {
		return Grant{}, fmt.Errorf("budget reserve: unexpected reply %v", res)
	}
	amount, _ := res[1].(int64)
	left, _ := res[2].(int64)
	switch res[0] {
	case "granted":
		return Grant{Outcome: Granted, Amount: amount, Left: left}, nil
	case "exhausted":
		return Grant{Outcome: Exhausted, Left: left}, nil
	case "duplicate":
		return Grant{Outcome: Duplicate, Amount: amount}, nil
	case "closed":
		return Grant{Outcome: Closed}, nil
	}
	return Grant{}, fmt.Errorf("budget reserve: unexpected reply %v", res)
}

func (r *Redis) Release(ctx context.Context, campaignID, paymentID string) error {
	err := releaseScript.Run(ctx, r.client,
		[]string{remainingKey(campaignID), grantKey(campaignID, paymentID), inflightKey(campaignID)},
		paymentID).Err()
	if err != nil {
		return fmt.Errorf("budget release: %w", err)
	}
	return nil
}

// Seed sets the counter and forgets every in-flight reservation. Call it
// only while nothing can reserve or commit for the campaign: with the
// campaign row locked and no payment still holding an earlier reservation.
func (r *Redis) Seed(ctx context.Context, campaignID string, remaining int64) error {
	if err := seedScript.Run(ctx, r.client, []string{remainingKey(campaignID), inflightKey(campaignID)}, remaining).Err(); err != nil {
		return fmt.Errorf("budget seed: %w", err)
	}
	return nil
}

// Close removes the counter, so every later Reserve gets Closed. In-flight
// reservations stay listed until their payments commit or are released.
func (r *Redis) Close(ctx context.Context, campaignID string) error {
	if err := r.client.Del(ctx, remainingKey(campaignID)).Err(); err != nil {
		return fmt.Errorf("budget close: %w", err)
	}
	return nil
}

func (r *Redis) Snapshot(ctx context.Context, campaignID string) (Snapshot, error) {
	res, err := snapshotScript.Run(ctx, r.client, []string{remainingKey(campaignID), inflightKey(campaignID)}).Slice()
	if err != nil {
		return Snapshot{}, fmt.Errorf("budget snapshot: %w", err)
	}
	if len(res) != 3 {
		return Snapshot{}, fmt.Errorf("budget snapshot: unexpected reply %v", res)
	}
	s := Snapshot{Inflight: map[string]Inflight{}}
	if v, ok := res[0].(string); ok {
		if s.Remaining, err = strconv.ParseInt(v, 10, 64); err != nil {
			return Snapshot{}, fmt.Errorf("budget snapshot: counter %q: %w", v, err)
		}
		s.Open = true
	}
	now, err := strconv.ParseInt(fmt.Sprint(res[1]), 10, 64)
	if err != nil {
		return Snapshot{}, fmt.Errorf("budget snapshot: time %v: %w", res[1], err)
	}
	s.Now = time.Unix(now, 0)
	fields, _ := res[2].([]any)
	for i := 0; i+1 < len(fields); i += 2 {
		id, _ := fields[i].(string)
		v, _ := fields[i+1].(string)
		amount, at, ok := strings.Cut(v, ":")
		a, err1 := strconv.ParseInt(amount, 10, 64)
		t, err2 := strconv.ParseInt(at, 10, 64)
		if !ok || err1 != nil || err2 != nil {
			return Snapshot{}, fmt.Errorf("budget snapshot: inflight %q = %q", id, v)
		}
		s.Inflight[id] = Inflight{Amount: a, ReservedAt: time.Unix(t, 0)}
	}
	return s, nil
}

// Adjust adds delta to the counter if it exists. It reports whether it did.
func (r *Redis) Adjust(ctx context.Context, campaignID string, delta int64) (bool, error) {
	err := adjustScript.Run(ctx, r.client, []string{remainingKey(campaignID)}, delta).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("budget adjust: %w", err)
	}
	return true, nil
}

// Prune forgets in-flight reservations whose payments have committed.
func (r *Redis) Prune(ctx context.Context, campaignID string, paymentIDs []string) error {
	if len(paymentIDs) == 0 {
		return nil
	}
	if err := r.client.HDel(ctx, inflightKey(campaignID), paymentIDs...).Err(); err != nil {
		return fmt.Errorf("budget prune: %w", err)
	}
	return nil
}

// ErrLocked: another reconciler is working on the campaign.
var ErrLocked = errors.New("campaign is being reconciled elsewhere")

// Lock makes one reconciler at a time work on a campaign (one per server
// may run). It expires after ttl in case the holder dies.
func (r *Redis) Lock(ctx context.Context, campaignID, token string, ttl time.Duration) (unlock func(context.Context), err error) {
	ok, err := r.client.SetNX(ctx, lockKey(campaignID), token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("budget lock: %w", err)
	}
	if !ok {
		return nil, ErrLocked
	}
	return func(ctx context.Context) {
		unlockScript.Run(ctx, r.client, []string{lockKey(campaignID)}, token)
	}, nil
}
