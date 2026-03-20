package service

import (
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/config"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/probe"
	"github.com/Resinat/Resin/internal/testutil"
	"github.com/Resinat/Resin/internal/topology"
)

func TestGetSystemTaskStatus_IncludesKnownEgressAndStaleCleanup(t *testing.T) {
	pool := topology.NewGlobalNodePool(topology.PoolConfig{
		MaxLatencyTableEntries: 16,
		MaxConsecutiveFailures: func() int { return 3 },
		LatencyDecayWindow:     func() time.Duration { return 10 * time.Minute },
	})

	knownHash := node.HashFromRawOptions([]byte(`{"type":"ss","server":"1.1.1.1","port":443}`))
	unknownHash := node.HashFromRawOptions([]byte(`{"type":"ss","server":"2.2.2.2","port":443}`))
	pool.AddNodeFromSub(knownHash, []byte(`{"type":"ss","server":"1.1.1.1","port":443}`), "sub-a")
	pool.AddNodeFromSub(unknownHash, []byte(`{"type":"ss","server":"2.2.2.2","port":443}`), "sub-a")

	knownEntry, _ := pool.GetEntry(knownHash)
	unknownEntry, _ := pool.GetEntry(unknownHash)
	storeProbeTestOutbound := func(entry *node.NodeEntry) {
		ob := testutil.NewNoopOutbound()
		entry.Outbound.Store(&ob)
	}
	storeProbeTestOutbound(knownEntry)
	storeProbeTestOutbound(unknownEntry)
	knownEntry.SetEgressIP(netip.MustParseAddr("203.0.113.10"))
	unknownEntry.StaleCleanupWindowStartedAt.Store(time.Now().Add(-time.Hour).UnixNano())

	runtimeCfg := &atomic.Pointer[config.RuntimeConfig]{}
	cfg := config.NewDefaultRuntimeConfig()
	cfg.StaleNodeCleanupEnabled = true
	runtimeCfg.Store(cfg)

	cp := &ControlPlaneService{
		RuntimeCfg: runtimeCfg,
		ProbeMgr: probe.NewProbeManager(probe.ProbeConfig{
			Pool: pool,
		}),
		StaleCleaner: probe.NewStaleNodeCleaner(probe.StaleNodeCleanerConfig{
			Pool: pool,
			RuntimeSettings: func() probe.StaleNodeCleanupSettings {
				return probe.StaleNodeCleanupSettings{Enabled: true, Window: 7 * 24 * time.Hour}
			},
		}),
	}

	status := cp.GetSystemTaskStatus()
	if status.Probe.KnownEgressNodes != 1 {
		t.Fatalf("known_egress_nodes=%d, want 1", status.Probe.KnownEgressNodes)
	}
	if status.Probe.UnknownEgressNodes != 1 {
		t.Fatalf("unknown_egress_nodes=%d, want 1", status.Probe.UnknownEgressNodes)
	}
	if !status.StaleCleanup.Enabled {
		t.Fatal("stale cleanup should report enabled")
	}
	if status.StaleCleanup.TrackedCandidates != 1 {
		t.Fatalf("tracked_candidates=%d, want 1", status.StaleCleanup.TrackedCandidates)
	}
}
