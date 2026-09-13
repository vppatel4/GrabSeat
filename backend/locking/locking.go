// Package locking is the ephemeral side of GrabSeat: Redis. It owns the atomic
// seat hold, the pub/sub fan-out that makes the design multi-instance, and the
// expiry listener that auto-releases abandoned holds.
package locking

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"grabseat/model"
)

const (
	holdPrefix   = "hold:"
	bookedPrefix = "booked:"
	eventChannel = "seat_events"
	// db 0 is go-redis's default; the keyspace-expiry channel is scoped per db.
	expiredChannel = "__keyevent@0__:expired"
)

// Hold results returned by TryHold.
const (
	HoldAcquired = "OK"     // this caller now holds the seat
	HoldTaken    = "HELD"   // someone else already holds it
	HoldBooked   = "BOOKED" // the seat is already sold
)

// holdScript runs the whole hold decision in one atomic step on Redis:
// reject if the seat is already booked, otherwise SET NX EX. Doing it as one
// script means there is no window between "is it booked?" and "grab it".
var holdScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
  return 'BOOKED'
end
if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'EX', ARGV[2]) then
  return 'OK'
end
return 'HELD'
`)

// bookScript flips a hold into a permanent booked marker atomically: drop the
// hold, set the booked key with no expiry. The Postgres row is the real record;
// this key is just a fast cache so the hold path can reject sold seats.
var bookScript = redis.NewScript(`
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], ARGV[1])
return 'OK'
`)

type Client struct {
	rdb *redis.Client
	log *slog.Logger
}

func New(addr, password string, log *slog.Logger) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           0,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		// go-redis reconnects on its own; keep a modest pool for the load test.
		PoolSize: 20,
	})
	return &Client{rdb: rdb, log: log}
}

func (c *Client) Close() error { return c.rdb.Close() }

func (c *Client) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }

func (c *Client) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return c.rdb.Ping(ctx).Err() == nil
}

func holdKey(seatID int) string   { return holdPrefix + strconv.Itoa(seatID) }
func bookedKey(seatID int) string { return bookedPrefix + strconv.Itoa(seatID) }

// TryHold attempts to hold a seat. It returns one of the Hold* constants and,
// when acquired, the wall-clock time the hold will expire.
func (c *Client) TryHold(ctx context.Context, seatID int, session string, ttl time.Duration) (result string, expiresAtMs int64, err error) {
	res, err := holdScript.Run(ctx, c.rdb,
		[]string{holdKey(seatID), bookedKey(seatID)},
		session, int(ttl.Seconds()),
	).Text()
	if err != nil {
		return "", 0, fmt.Errorf("hold script: %w", err)
	}
	if res == HoldAcquired {
		expiresAtMs = time.Now().Add(ttl).UnixMilli()
	}
	return res, expiresAtMs, nil
}

// HoldOwner returns the session currently holding a seat, if any.
func (c *Client) HoldOwner(ctx context.Context, seatID int) (session string, held bool, err error) {
	v, err := c.rdb.Get(ctx, holdKey(seatID)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// MarkBooked atomically clears the hold and records the permanent booked marker.
func (c *Client) MarkBooked(ctx context.Context, seatID int, session string) error {
	return bookScript.Run(ctx, c.rdb,
		[]string{holdKey(seatID), bookedKey(seatID)}, session).Err()
}

// RehydrateBooked repopulates the booked cache from the durable record. Run on
// startup and whenever Redis comes back, so the hold path knows which seats are
// already sold even if Redis was wiped.
func (c *Client) RehydrateBooked(ctx context.Context, booked map[int]string) error {
	if len(booked) == 0 {
		return nil
	}
	pipe := c.rdb.Pipeline()
	for seatID, session := range booked {
		pipe.Set(ctx, bookedKey(seatID), session, 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// HeldSeats returns the live hold state (who holds what, and until when) by
// scanning the hold keys. Used to build the snapshot a client gets on connect.
func (c *Client) HeldSeats(ctx context.Context) (map[int]model.SeatState, error) {
	out := make(map[int]model.SeatState)
	var cursor uint64
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, holdPrefix+"*", 200).Result()
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			pipe := c.rdb.Pipeline()
			gets := make([]*redis.StringCmd, len(keys))
			ttls := make([]*redis.DurationCmd, len(keys))
			for i, k := range keys {
				gets[i] = pipe.Get(ctx, k)
				ttls[i] = pipe.PTTL(ctx, k)
			}
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				return nil, err
			}
			for i, k := range keys {
				seatID, ok := seatIDFromKey(k, holdPrefix)
				if !ok {
					continue
				}
				session, err := gets[i].Result()
				if err != nil {
					continue // key expired between scan and get; skip it
				}
				var expiresAtMs int64
				if d, err := ttls[i].Result(); err == nil && d > 0 {
					expiresAtMs = time.Now().Add(d).UnixMilli()
				}
				out[seatID] = model.SeatState{
					SeatID:      seatID,
					Status:      model.StatusHeld,
					HeldBy:      session,
					ExpiresAtMs: expiresAtMs,
				}
			}
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return out, nil
}

// Publish broadcasts a seat event to every backend instance via Redis pub/sub.
func (c *Client) Publish(ctx context.Context, ev model.SeatEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return c.rdb.Publish(ctx, eventChannel, b).Err()
}

// SubscribeSeatEvents delivers every published seat event to handler until ctx
// is cancelled. go-redis's Channel() reconnects and re-subscribes on its own, so
// a brief Redis outage just pauses the stream rather than ending it.
func (c *Client) SubscribeSeatEvents(ctx context.Context, handler func(model.SeatEvent)) {
	sub := c.rdb.Subscribe(ctx, eventChannel)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var ev model.SeatEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				c.log.Warn("bad seat event payload", "err", err)
				continue
			}
			handler(ev)
		}
	}
}

// SubscribeExpiries listens for hold keys hitting their TTL. Every instance
// receives the expiry, so each one can free the seat for its own connected
// clients — no polling, no cleanup job. handler is called with the seat id.
func (c *Client) SubscribeExpiries(ctx context.Context, handler func(seatID int)) {
	sub := c.rdb.Subscribe(ctx, expiredChannel)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			seatID, ok := seatIDFromKey(msg.Payload, holdPrefix)
			if !ok {
				continue // some other key expired; not our concern
			}
			handler(seatID)
		}
	}
}

func seatIDFromKey(key, prefix string) (int, bool) {
	if !strings.HasPrefix(key, prefix) {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
	if err != nil {
		return 0, false
	}
	return id, true
}
