package probe

import (
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/subscription"
	"github.com/Resinat/Resin/internal/topology"
)

func newStaleCleanupTestHarness() (*topology.SubscriptionManager, *topology.GlobalNodePool, *subscription.Subscription, node.Hash, *node.NodeEntry) {
	subManager := topology.NewSubscriptionManager()
	sub := subscription.NewSubscription("sub-1", "sub-1", "", true, false)
	subManager.Register(sub)

	pool := topology.NewGlobalNodePool(topology.PoolConfig{
		MaxLatencyTableEntries: 16,
		MaxConsecutiveFailures: func() int { return 3 },
		LatencyDecayWindow:     func() time.Duration { return 10 * time.Minute },
	})

	raw := []byte(`{"type":"ss","server":"1.1.1.1","port":443}`)
	hash := node.HashFromRawOptions(raw)
	pool.AddNodeFromSub(hash, raw, sub.ID)
	sub.ManagedNodes().StoreNode(hash, subscription.ManagedNode{Tags: []string{"tag-a"}})

	entry, _ := pool.GetEntry(hash)
	storeOutbound(entry)
	entry.CircuitOpenSince.Store(time.Now().Add(-time.Minute).UnixNano())
	return subManager, pool, sub, hash, entry
}

func TestStaleNodeCleaner_DeletesAfterObservedFailureWindow(t *testing.T) {
	subManager, pool, sub, hash, entry := newStaleCleanupTestHarness()
	changed := 0
	deleted := 0
	cleaner := NewStaleNodeCleaner(StaleNodeCleanerConfig{
		Pool: pool,
		RuntimeSettings: func() StaleNodeCleanupSettings {
			return StaleNodeCleanupSettings{
				Enabled:                     true,
				Window:                      2 * time.Hour,
				MaxEgressTestInterval:       10 * time.Minute,
				MaxLatencyTestInterval:      time.Hour,
				MaxAuthorityLatencyInterval: time.Hour,
			}
		},
		OnStateChanged: func(node.Hash) {
			changed++
		},
		DeleteNode: func(h node.Hash) error {
			if !topology.HardDeleteNode(subManager, pool, h) {
				t.Fatalf("HardDeleteNode(%s) returned false", h.Hex())
			}
			deleted++
			return nil
		},
	})

	now := time.Now()
	entry.LastLatencyProbeAttempt.Store(now.Add(-2 * time.Hour).UnixNano())
	cleaner.sweep()
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 after first failed observation", deleted)
	}

	entry.LastLatencyProbeAttempt.Store(now.Add(-1 * time.Hour).UnixNano())
	cleaner.sweep()
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 before window is covered", deleted)
	}

	entry.LastLatencyProbeAttempt.Store(now.UnixNano())
	cleaner.sweep()
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 after observed failure window", deleted)
	}
	if _, ok := pool.GetEntry(hash); ok {
		t.Fatal("node should be removed from pool")
	}
	if _, ok := sub.ManagedNodes().LoadNode(hash); ok {
		t.Fatal("node should be removed from subscription managed nodes")
	}
	if changed == 0 {
		t.Fatal("expected stale cleanup state changes to be reported")
	}
}

func TestStaleNodeCleaner_ResetsWindowAfterRecovery(t *testing.T) {
	_, pool, _, hash, entry := newStaleCleanupTestHarness()
	cleaner := NewStaleNodeCleaner(StaleNodeCleanerConfig{
		Pool: pool,
		RuntimeSettings: func() StaleNodeCleanupSettings {
			return StaleNodeCleanupSettings{
				Enabled:                     true,
				Window:                      2 * time.Hour,
				MaxEgressTestInterval:       10 * time.Minute,
				MaxLatencyTestInterval:      time.Hour,
				MaxAuthorityLatencyInterval: time.Hour,
			}
		},
	})

	entry.LastLatencyProbeAttempt.Store(time.Now().Add(-time.Hour).UnixNano())
	cleaner.sweep()
	if entry.StaleCleanupFailedProbeCount.Load() != 1 {
		t.Fatalf("failed_probe_count=%d, want 1", entry.StaleCleanupFailedProbeCount.Load())
	}

	entry.CircuitOpenSince.Store(0)
	cleaner.sweep()
	if entry.StaleCleanupWindowStartedAt.Load() != 0 ||
		entry.StaleCleanupLastObservedProbeAt.Load() != 0 ||
		entry.StaleCleanupFailedProbeCount.Load() != 0 {
		t.Fatalf("stale cleanup state was not reset for %s", hash.Hex())
	}
}

func TestStaleNodeCleaner_ResetsWindowAfterLongObservationGap(t *testing.T) {
	_, pool, _, _, entry := newStaleCleanupTestHarness()
	cleaner := NewStaleNodeCleaner(StaleNodeCleanerConfig{
		Pool: pool,
		RuntimeSettings: func() StaleNodeCleanupSettings {
			return StaleNodeCleanupSettings{
				Enabled:                     true,
				Window:                      6 * time.Hour,
				MaxEgressTestInterval:       10 * time.Minute,
				MaxLatencyTestInterval:      time.Hour,
				MaxAuthorityLatencyInterval: time.Hour,
			}
		},
	})

	now := time.Now()
	first := now.Add(-5 * time.Hour).UnixNano()
	second := now.UnixNano()

	entry.LastLatencyProbeAttempt.Store(first)
	cleaner.sweep()
	if entry.StaleCleanupFailedProbeCount.Load() != 1 {
		t.Fatalf("failed_probe_count=%d, want 1 after first observation", entry.StaleCleanupFailedProbeCount.Load())
	}

	entry.LastLatencyProbeAttempt.Store(second)
	cleaner.sweep()
	if got := entry.StaleCleanupWindowStartedAt.Load(); got != second {
		t.Fatalf("window_started_at=%d, want reset to %d", got, second)
	}
	if got := entry.StaleCleanupFailedProbeCount.Load(); got != 1 {
		t.Fatalf("failed_probe_count=%d, want reset to 1", got)
	}
}
