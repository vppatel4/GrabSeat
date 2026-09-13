package loadtest

import (
	"testing"
	"time"
)

// TestMultiInstanceBroadcast proves the pub/sub design is genuinely
// cross-instance: a change produced on one backend instance reaches a client
// connected to a different instance. Without Redis pub/sub — if broadcast state
// lived only in one process's memory — this would fail.
//
// Run it against a scaled stack:  docker compose up -d --scale backend=2
// It skips (rather than fails) if only one instance is running, since there'd be
// nothing cross-instance to prove.
func TestMultiInstanceBroadcast(t *testing.T) {
	requireIntegration(t)
	waitStackReady(t)

	// Open several connections; the gateway round-robins, so with 2+ replicas we
	// should land on at least two distinct instances.
	const conns = 6
	clients := make([]*wsClient, conns)
	instances := make(map[string][]*wsClient)
	for i := 0; i < conns; i++ {
		c := connectWS(t)
		clients[i] = c
		defer c.close()
		instances[c.snapshot.InstanceID] = append(instances[c.snapshot.InstanceID], c)
	}

	if len(instances) < 2 {
		t.Skipf("only %d backend instance(s) detected; run 'docker compose up -d --scale backend=2' to exercise this test", len(instances))
	}

	// Pick one client from two different instances.
	var a, b *wsClient
	for _, cs := range instances {
		if a == nil {
			a = cs[0]
		} else if b == nil {
			b = cs[0]
			break
		}
	}

	// Produce a change (a hold). It lands on whichever instance the gateway
	// routes the HTTP request to — possibly neither a's nor b's. Both clients
	// must still receive it, which only works if the event crossed instances.
	seatID := availableSeats(t, 1)[0]
	holder := newSession()
	if code, _, err := hold(holder, seatID); err != nil || code != 200 {
		t.Fatalf("hold failed: code=%d err=%v", code, err)
	}

	a.waitForEvent(t, seatID, "held", 5*time.Second)
	b.waitForEvent(t, seatID, "held", 5*time.Second)
	t.Logf("MULTI-INSTANCE RESULTS: %d distinct instances, event delivered across all", len(instances))
}
