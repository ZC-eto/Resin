package probe

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/scanloop"
	"github.com/Resinat/Resin/internal/topology"
)

const minimumFailedProbesForStaleCleanup = int64(2)

type StaleNodeCleanupSettings struct {
	Enabled                     bool
	Window                      time.Duration
	MaxEgressTestInterval       time.Duration
	MaxLatencyTestInterval      time.Duration
	MaxAuthorityLatencyInterval time.Duration
}

type StaleNodeCleanupStatus struct {
	Enabled           bool  `json:"enabled"`
	TrackedCandidates int   `json:"tracked_candidates"`
	DeletedLastRun    int   `json:"deleted_last_run"`
	LastRunAtNs       int64 `json:"last_run_at_ns"`
}

type StaleNodeCleanerConfig struct {
	Pool            *topology.GlobalNodePool
	RuntimeSettings func() StaleNodeCleanupSettings
	OnStateChanged  func(hash node.Hash)
	DeleteNode      func(hash node.Hash) error
}

// StaleNodeCleaner tracks nodes that keep failing real probe cycles and
// hard-deletes them once the observed failed-probe window spans the configured duration.
type StaleNodeCleaner struct {
	pool            *topology.GlobalNodePool
	runtimeSettings func() StaleNodeCleanupSettings
	onStateChanged  func(hash node.Hash)
	deleteNode      func(hash node.Hash) error

	stopCh chan struct{}
	wg     sync.WaitGroup

	lastRunAtNs    atomic.Int64
	deletedLastRun atomic.Int64
}

func NewStaleNodeCleaner(cfg StaleNodeCleanerConfig) *StaleNodeCleaner {
	return &StaleNodeCleaner{
		pool:            cfg.Pool,
		runtimeSettings: cfg.RuntimeSettings,
		onStateChanged:  cfg.OnStateChanged,
		deleteNode:      cfg.DeleteNode,
		stopCh:          make(chan struct{}),
	}
}

func (c *StaleNodeCleaner) Start() {
	if c == nil {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		scanloop.Run(c.stopCh, scanloop.DefaultMinInterval, scanloop.DefaultJitterRange, c.sweep)
	}()
}

func (c *StaleNodeCleaner) Stop() {
	if c == nil {
		return
	}
	close(c.stopCh)
	c.wg.Wait()
}

func (c *StaleNodeCleaner) Status() StaleNodeCleanupStatus {
	status := StaleNodeCleanupStatus{
		LastRunAtNs:    c.lastRunAtNs.Load(),
		DeletedLastRun: int(c.deletedLastRun.Load()),
	}
	if c == nil || c.pool == nil {
		return status
	}
	settings := c.currentSettings()
	status.Enabled = settings.Enabled
	c.pool.RangeNodes(func(_ node.Hash, entry *node.NodeEntry) bool {
		if entry != nil && entry.StaleCleanupWindowStartedAt.Load() > 0 {
			status.TrackedCandidates++
		}
		return true
	})
	return status
}

func (c *StaleNodeCleaner) sweep() {
	if c == nil {
		return
	}
	now := time.Now()
	c.lastRunAtNs.Store(now.UnixNano())
	settings := c.currentSettings()
	if c.pool == nil || !settings.Enabled || settings.Window <= 0 {
		c.deletedLastRun.Store(0)
		return
	}

	toDelete := make([]node.Hash, 0)
	c.pool.RangeNodes(func(hash node.Hash, entry *node.NodeEntry) bool {
		if c.shouldDeleteNode(hash, entry, settings) {
			toDelete = append(toDelete, hash)
		}
		return true
	})

	deleted := 0
	for _, hash := range toDelete {
		if c.deleteNode == nil {
			continue
		}
		if err := c.deleteNode(hash); err != nil {
			log.Printf("[stale-cleanup] delete node %s failed: %v", hash.Hex(), err)
			continue
		}
		deleted++
	}
	c.deletedLastRun.Store(int64(deleted))
}

func (c *StaleNodeCleaner) currentSettings() StaleNodeCleanupSettings {
	if c == nil || c.runtimeSettings == nil {
		return StaleNodeCleanupSettings{}
	}
	settings := c.runtimeSettings()
	if settings.MaxEgressTestInterval <= 0 {
		settings.MaxEgressTestInterval = 24 * time.Hour
	}
	if settings.MaxLatencyTestInterval <= 0 {
		settings.MaxLatencyTestInterval = time.Hour
	}
	if settings.MaxAuthorityLatencyInterval <= 0 {
		settings.MaxAuthorityLatencyInterval = 3 * time.Hour
	}
	return settings
}

func (c *StaleNodeCleaner) shouldDeleteNode(
	hash node.Hash,
	entry *node.NodeEntry,
	settings StaleNodeCleanupSettings,
) bool {
	if entry == nil {
		return false
	}
	if !isNodeFailingForStaleCleanup(entry) {
		c.resetNodeState(hash, entry)
		return false
	}

	observedAtNs, expectedGap := staleCleanupObservation(entry, settings)
	if observedAtNs <= 0 {
		c.resetNodeState(hash, entry)
		return false
	}

	resetWindow := false
	prevObserved := entry.StaleCleanupLastObservedProbeAt.Load()
	if prevObserved > 0 && observedAtNs > prevObserved && expectedGap > 0 {
		if time.Duration(observedAtNs-prevObserved) > expectedGap*2 {
			resetWindow = true
		}
	}

	if entry.ObserveFailedProbe(observedAtNs, resetWindow) {
		c.markStateChanged(hash)
	}

	windowStarted := entry.StaleCleanupWindowStartedAt.Load()
	lastObserved := entry.StaleCleanupLastObservedProbeAt.Load()
	failedCount := entry.StaleCleanupFailedProbeCount.Load()
	if windowStarted <= 0 || lastObserved <= 0 || failedCount < minimumFailedProbesForStaleCleanup {
		return false
	}

	return time.Duration(lastObserved-windowStarted) >= settings.Window
}

func (c *StaleNodeCleaner) resetNodeState(hash node.Hash, entry *node.NodeEntry) {
	if entry != nil && entry.ResetStaleCleanupWindow() {
		c.markStateChanged(hash)
	}
}

func (c *StaleNodeCleaner) markStateChanged(hash node.Hash) {
	if c == nil || c.onStateChanged == nil {
		return
	}
	c.onStateChanged(hash)
}

func isNodeFailingForStaleCleanup(entry *node.NodeEntry) bool {
	if entry == nil {
		return false
	}
	return entry.IsCircuitOpen() || (!entry.HasOutbound() && entry.GetLastError() != "")
}

func staleCleanupObservation(entry *node.NodeEntry, settings StaleNodeCleanupSettings) (int64, time.Duration) {
	if entry == nil {
		return 0, 0
	}

	lastEgress := entry.LastEgressUpdateAttempt.Load()
	lastLatency := entry.LastLatencyProbeAttempt.Load()
	lastAuthority := entry.LastAuthorityLatencyProbeAttempt.Load()
	latencyObserved := lastLatency
	if lastAuthority > latencyObserved {
		latencyObserved = lastAuthority
	}

	if !entry.HasOutbound() {
		return lastEgress, currentEgressProbeInterval(entry, settings.MaxEgressTestInterval)
	}
	if lastEgress >= latencyObserved {
		return lastEgress, currentEgressProbeInterval(entry, settings.MaxEgressTestInterval)
	}

	expectedGap := settings.MaxLatencyTestInterval
	if settings.MaxAuthorityLatencyInterval > expectedGap {
		expectedGap = settings.MaxAuthorityLatencyInterval
	}
	return latencyObserved, expectedGap
}
