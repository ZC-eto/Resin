package service

import (
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/config"
	"github.com/Resinat/Resin/internal/ipprofile"
	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/probe"
	"github.com/Resinat/Resin/internal/subscription"
	"github.com/Resinat/Resin/internal/testutil"
	"github.com/Resinat/Resin/internal/topology"
)

func newUnknownFillProfileService(pool *topology.GlobalNodePool) *ipprofile.Service {
	return ipprofile.NewService(ipprofile.Config{
		Pool:                pool,
		BackgroundBatchSize: 4,
		RuntimeSettings: func() ipprofile.RuntimeSettings {
			return ipprofile.RuntimeSettings{
				OnlineProvider:          config.IPProfileOnlineProviderProxycheck,
				OnlineAPIKey:            "test-key",
				OnlineRequestsPerMinute: 60,
				CacheTTL:                time.Hour,
				BackgroundEnabled:       true,
				RefreshOnEgressChange:   true,
			}
		},
	})
}

func addSubscriptionManagedNode(
	t *testing.T,
	pool *topology.GlobalNodePool,
	sub *subscription.Subscription,
	raw []byte,
	tag string,
) (node.Hash, *node.NodeEntry) {
	t.Helper()

	hash := node.HashFromRawOptions(raw)
	pool.AddNodeFromSub(hash, raw, sub.ID)
	sub.ManagedNodes().StoreNode(hash, subscription.ManagedNode{Tags: []string{tag}})

	entry, ok := pool.GetEntry(hash)
	if !ok {
		t.Fatalf("node %s missing after add", hash.Hex())
	}
	return hash, entry
}

func TestGetSubscription_UnknownNodeCountsAreSplitByProfileState(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()
	pool := newNodeListTestPool(subMgr)

	sub := subscription.NewSubscription("sub-a", "sub-a", "https://example.com/a", true, false)
	subMgr.Register(sub)

	pendingEgressHash, pendingEgressEntry := addSubscriptionManagedNode(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"1.1.1.1","server_port":443}`),
		"pending-egress",
	)
	pendingEgressOutbound := testutil.NewNoopOutbound()
	pendingEgressEntry.Outbound.Store(&pendingEgressOutbound)
	pool.RecordResult(pendingEgressHash, true)

	pendingProfileHash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"2.2.2.2","server_port":443}`),
		"203.0.113.20",
		"pending-profile",
	)

	profiledUnknownHash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"3.3.3.3","server_port":443}`),
		"203.0.113.30",
		"profiled-unknown",
	)
	pool.UpdateNodeProfile(profiledUnknownHash, node.NodeProfile{
		IP:                 netip.MustParseAddr("203.0.113.30"),
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceLocal,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	})

	_, unavailableEntry := addSubscriptionManagedNode(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"4.4.4.4","server_port":443}`),
		"unavailable",
	)
	unavailableEntry.LastEgressUpdateAttempt.Store(time.Now().UnixNano())

	cp := &ControlPlaneService{
		Pool:   pool,
		SubMgr: subMgr,
	}

	resp, err := cp.GetSubscription(sub.ID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if resp.PendingEgressNodeCount != 1 {
		t.Fatalf("pending_egress_node_count = %d, want 1", resp.PendingEgressNodeCount)
	}
	if resp.PendingProfileNodeCount != 1 {
		t.Fatalf("pending_profile_node_count = %d, want 1", resp.PendingProfileNodeCount)
	}
	if resp.ProfiledUnknownNodeCount != 1 {
		t.Fatalf("profiled_unknown_node_count = %d, want 1", resp.ProfiledUnknownNodeCount)
	}
	if resp.UnknownNodeCount != 3 {
		t.Fatalf("unknown_node_count = %d, want 3", resp.UnknownNodeCount)
	}
	if resp.NodeCount != 4 {
		t.Fatalf("node_count = %d, want 4", resp.NodeCount)
	}
	if resp.HealthyNodeCount != 3 {
		t.Fatalf("healthy_node_count = %d, want 3", resp.HealthyNodeCount)
	}
	if pendingProfileHash.Hex() == profiledUnknownHash.Hex() {
		t.Fatal("test setup produced duplicate hashes")
	}
}

func TestFillSubscriptionUnknownNodes_QueuesEgressAndProfiles(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()
	pool := newNodeListTestPool(subMgr)

	sub := subscription.NewSubscription("sub-a", "sub-a", "https://example.com/a", true, false)
	subMgr.Register(sub)

	pendingEgressHash, pendingEgressEntry := addSubscriptionManagedNode(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"10.0.0.1","server_port":443}`),
		"pending-egress",
	)
	pendingEgressOutbound := testutil.NewNoopOutbound()
	pendingEgressEntry.Outbound.Store(&pendingEgressOutbound)
	pool.RecordResult(pendingEgressHash, true)

	pendingProfileHash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"10.0.0.2","server_port":443}`),
		"203.0.113.42",
		"pending-profile",
	)

	profiledUnknownHash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"10.0.0.3","server_port":443}`),
		"203.0.113.43",
		"profiled-unknown",
	)
	pool.UpdateNodeProfile(profiledUnknownHash, node.NodeProfile{
		IP:                 netip.MustParseAddr("203.0.113.43"),
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceOnline,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	})

	_, unavailableEntry := addSubscriptionManagedNode(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"10.0.0.4","server_port":443}`),
		"unavailable",
	)
	unavailableEntry.LastEgressUpdateAttempt.Store(time.Now().UnixNano())

	evictedHash, _ := addSubscriptionManagedNode(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"10.0.0.5","server_port":443}`),
		"evicted",
	)
	evictedManaged, ok := sub.ManagedNodes().LoadNode(evictedHash)
	if !ok {
		t.Fatal("evicted node missing from managed set")
	}
	evictedManaged.Evicted = true
	sub.ManagedNodes().StoreNode(evictedHash, evictedManaged)
	pool.RemoveNodeFromSub(evictedHash, sub.ID)

	probeMgr := probe.NewProbeManager(probe.ProbeConfig{
		Pool:        pool,
		Concurrency: 1,
	})
	defer probeMgr.Stop()

	profileSvc := newUnknownFillProfileService(pool)

	cp := &ControlPlaneService{
		Pool:       pool,
		SubMgr:     subMgr,
		ProbeMgr:   probeMgr,
		ProfileSvc: profileSvc,
	}

	result, err := cp.FillSubscriptionUnknownNodes(sub.ID)
	if err != nil {
		t.Fatalf("FillSubscriptionUnknownNodes: %v", err)
	}
	if result.Matched != 5 {
		t.Fatalf("matched = %d, want 5", result.Matched)
	}
	if result.QueuedEgress != 1 {
		t.Fatalf("queued_egress = %d, want 1", result.QueuedEgress)
	}
	if result.QueuedProfile != 2 {
		t.Fatalf("queued_profile = %d, want 2", result.QueuedProfile)
	}
	if result.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2", result.Skipped)
	}
	if result.Failed != 0 {
		t.Fatalf("failed = %d, want 0", result.Failed)
	}
	if state := profileSvc.TaskState(pendingProfileHash); state != ipprofile.NodeTaskStateQueued {
		t.Fatalf("pending profile task state = %s, want %s", state, ipprofile.NodeTaskStateQueued)
	}
	if state := profileSvc.TaskState(profiledUnknownHash); state != ipprofile.NodeTaskStateQueued {
		t.Fatalf("profiled unknown task state = %s, want %s", state, ipprofile.NodeTaskStateQueued)
	}
}

func TestAutoFillSubscriptionUnknownNodes_SkipsOnlineProfiledUnknown(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()
	pool := newNodeListTestPool(subMgr)

	sub := subscription.NewSubscription("sub-a", "sub-a", "https://example.com/a", true, false)
	subMgr.Register(sub)

	localUnknownHash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"20.0.0.1","server_port":443}`),
		"203.0.113.50",
		"local-unknown",
	)
	pool.UpdateNodeProfile(localUnknownHash, node.NodeProfile{
		IP:                 netip.MustParseAddr("203.0.113.50"),
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceLocal,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	})

	onlineUnknownHash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"20.0.0.2","server_port":443}`),
		"203.0.113.51",
		"online-unknown",
	)
	pool.UpdateNodeProfile(onlineUnknownHash, node.NodeProfile{
		IP:                 netip.MustParseAddr("203.0.113.51"),
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceOnline,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	})

	profileSvc := newUnknownFillProfileService(pool)
	cp := &ControlPlaneService{
		Pool:       pool,
		SubMgr:     subMgr,
		ProfileSvc: profileSvc,
	}

	if err := cp.AutoFillSubscriptionUnknownNodes(sub.ID); err != nil {
		t.Fatalf("AutoFillSubscriptionUnknownNodes: %v", err)
	}
	if state := profileSvc.TaskState(localUnknownHash); state != ipprofile.NodeTaskStateQueued {
		t.Fatalf("local unknown task state = %s, want %s", state, ipprofile.NodeTaskStateQueued)
	}
	if state := profileSvc.TaskState(onlineUnknownHash); state != ipprofile.NodeTaskStateIdle {
		t.Fatalf("online unknown task state = %s, want %s", state, ipprofile.NodeTaskStateIdle)
	}
}

func TestAutoFillSubscriptionUnknownNodes_MissingServicesAreReported(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()
	pool := newNodeListTestPool(subMgr)
	sub := subscription.NewSubscription("sub-a", "sub-a", "https://example.com/a", true, false)
	subMgr.Register(sub)

	raw := []byte(`{"type":"shadowsocks","server":"30.0.0.1","server_port":443}`)
	hash := addRoutableNodeForSubscriptionWithTag(t, pool, sub, raw, "203.0.113.60", "missing-service")
	_ = hash

	cp := &ControlPlaneService{
		Pool:   pool,
		SubMgr: subMgr,
	}

	result, err := cp.FillSubscriptionUnknownNodes(sub.ID)
	if err != nil {
		t.Fatalf("FillSubscriptionUnknownNodes with missing services: %v", err)
	}
	if result.QueuedProfile != 0 {
		t.Fatalf("queued_profile = %d, want 0", result.QueuedProfile)
	}
	if result.Failed != 1 {
		t.Fatalf("failed = %d, want 1", result.Failed)
	}
}

func TestFillSubscriptionUnknownNodes_TracksQueuedProfileStateWithRuntimeConfig(t *testing.T) {
	subMgr := topology.NewSubscriptionManager()
	pool := newNodeListTestPool(subMgr)

	sub := subscription.NewSubscription("sub-a", "sub-a", "https://example.com/a", true, false)
	subMgr.Register(sub)

	hash := addRoutableNodeForSubscriptionWithTag(
		t,
		pool,
		sub,
		[]byte(`{"type":"shadowsocks","server":"40.0.0.1","server_port":443}`),
		"203.0.113.70",
		"runtime-config",
	)
	profileSvc := newUnknownFillProfileService(pool)
	runtimeCfg := &atomic.Pointer[config.RuntimeConfig]{}
	cfg := config.NewDefaultRuntimeConfig()
	cfg.IPProfileOnlineProvider = string(config.IPProfileOnlineProviderProxycheck)
	cfg.IPProfileOnlineAPIKey = "test-key"
	runtimeCfg.Store(cfg)

	cp := &ControlPlaneService{
		Pool:       pool,
		SubMgr:     subMgr,
		ProfileSvc: profileSvc,
		RuntimeCfg: runtimeCfg,
	}

	if _, err := cp.FillSubscriptionUnknownNodes(sub.ID); err != nil {
		t.Fatalf("FillSubscriptionUnknownNodes: %v", err)
	}
	if state := profileSvc.TaskState(hash); state != ipprofile.NodeTaskStateQueued {
		t.Fatalf("queued task state = %s, want %s", state, ipprofile.NodeTaskStateQueued)
	}
}
