package ipprofile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/config"
	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/state"
	"github.com/Resinat/Resin/internal/topology"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestProfileService(t *testing.T) (*Service, *state.StateEngine, *topology.GlobalNodePool) {
	t.Helper()

	engine, closer, err := state.PersistenceBootstrap(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("PersistenceBootstrap: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	pool := topology.NewGlobalNodePool(topology.PoolConfig{
		MaxLatencyTableEntries: 16,
		MaxConsecutiveFailures: func() int { return 3 },
		LatencyDecayWindow:     func() time.Duration { return 10 * time.Minute },
	})

	svc := NewService(Config{
		Pool:   pool,
		Engine: engine,
		RuntimeSettings: func() RuntimeSettings {
			return RuntimeSettings{CacheTTL: 24 * time.Hour}
		},
	})
	return svc, engine, pool
}

func TestDeleteCachedIPIfUnused_RemovesCacheAfterLastNodeDisappears(t *testing.T) {
	svc, engine, pool := newTestProfileService(t)

	raw := json.RawMessage(`{"type":"ss","server":"1.1.1.1","port":443}`)
	hash := node.HashFromRawOptions(raw)
	ip := netip.MustParseAddr("203.0.113.10")

	pool.AddNodeFromSub(hash, raw, "sub-a")
	entry, ok := pool.GetEntry(hash)
	if !ok {
		t.Fatal("node missing from pool")
	}
	entry.SetEgressIP(ip)

	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        model.EgressNetworkTypeResidential,
		Provider:           "Example ISP",
		Source:             model.EgressProfileSourceOnline,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}
	svc.putCached(ip.String(), profile, time.Hour)
	svc.putPersistentCached(profile)

	if deleted := svc.DeleteCachedIPIfUnused(ip); deleted {
		t.Fatal("cache should stay while IP is still in use")
	}
	if got, ok := svc.getCached(ip.String(), time.Hour); !ok || got.IP != ip {
		t.Fatalf("expected in-memory cache to remain, got ok=%v profile=%+v", ok, got)
	}
	persisted, err := engine.GetEgressProfileCache(ip.String())
	if err != nil {
		t.Fatalf("GetEgressProfileCache: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persistent cache to remain")
	}

	pool.RemoveNodeFromSub(hash, "sub-a")

	if deleted := svc.DeleteCachedIPIfUnused(ip); !deleted {
		t.Fatal("expected cache deletion after last node disappeared")
	}
	if _, ok := svc.getCached(ip.String(), time.Hour); ok {
		t.Fatal("expected in-memory cache to be deleted")
	}
	persisted, err = engine.GetEgressProfileCache(ip.String())
	if err != nil {
		t.Fatalf("GetEgressProfileCache after delete: %v", err)
	}
	if persisted != nil {
		t.Fatalf("expected persistent cache to be deleted, got %+v", persisted)
	}
}

func TestDeleteCachedIPIfUnused_KeepsSharedEgressIPCache(t *testing.T) {
	svc, engine, pool := newTestProfileService(t)

	ip := netip.MustParseAddr("203.0.113.20")
	rawA := json.RawMessage(`{"type":"ss","server":"2.2.2.2","port":443}`)
	rawB := json.RawMessage(`{"type":"ss","server":"3.3.3.3","port":443}`)
	hashA := node.HashFromRawOptions(rawA)
	hashB := node.HashFromRawOptions(rawB)

	pool.AddNodeFromSub(hashA, rawA, "sub-a")
	pool.AddNodeFromSub(hashB, rawB, "sub-b")

	entryA, _ := pool.GetEntry(hashA)
	entryB, _ := pool.GetEntry(hashB)
	entryA.SetEgressIP(ip)
	entryB.SetEgressIP(ip)

	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        model.EgressNetworkTypeDatacenter,
		Provider:           "Shared Provider",
		Source:             model.EgressProfileSourceLocalPlusOnline,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}
	svc.putPersistentCached(profile)

	pool.RemoveNodeFromSub(hashA, "sub-a")

	if deleted := svc.DeleteCachedIPIfUnused(ip); deleted {
		t.Fatal("cache should remain while another node still uses the same IP")
	}
	persisted, err := engine.GetEgressProfileCache(ip.String())
	if err != nil {
		t.Fatalf("GetEgressProfileCache: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected shared persistent cache to remain")
	}
}

func TestLookupProfile_RefreshesUnknownCacheWhenOnlineProviderBecomesAvailable(t *testing.T) {
	engine, closer, err := state.PersistenceBootstrap(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("PersistenceBootstrap: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	pool := topology.NewGlobalNodePool(topology.PoolConfig{
		MaxLatencyTableEntries: 16,
		MaxConsecutiveFailures: func() int { return 3 },
		LatencyDecayWindow:     func() time.Duration { return 10 * time.Minute },
	})

	ip := netip.MustParseAddr("203.0.113.40")
	var calls int
	svc := NewService(Config{
		Pool:   pool,
		Engine: engine,
		RuntimeSettings: func() RuntimeSettings {
			return RuntimeSettings{
				LocalLookupEnabled:      false,
				OnlineProvider:          config.IPProfileOnlineProviderProxycheck,
				OnlineAPIKey:            "demo-key",
				OnlineRequestsPerMinute: 120,
				CacheTTL:                24 * time.Hour,
			}
		},
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if !strings.Contains(req.URL.Host, "proxycheck.io") {
					t.Fatalf("unexpected host: %s", req.URL.Host)
				}
				body := `{"status":"ok","203.0.113.40":{"proxy":"no","type":"Residential","provider":"Example ISP","organisation":"Example ISP","asn":"AS64500"}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})

	staleUnknown := node.NodeProfile{
		IP:                 ip,
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceLocal,
		ProfileUpdatedAtNs: time.Now().Add(-time.Hour).UnixNano(),
	}
	svc.putPersistentCached(staleUnknown)

	profile, err := svc.LookupProfile(t.Context(), ip, false)
	if err != nil {
		t.Fatalf("LookupProfile: %v", err)
	}
	if calls != 1 {
		t.Fatalf("online lookup calls: got %d, want 1", calls)
	}
	if profile.NetworkType != model.EgressNetworkTypeResidential {
		t.Fatalf("network type: got %q, want %q", profile.NetworkType, model.EgressNetworkTypeResidential)
	}
	if profile.Source != model.EgressProfileSourceOnline {
		t.Fatalf("profile source: got %q, want %q", profile.Source, model.EgressProfileSourceOnline)
	}

	persisted, err := engine.GetEgressProfileCache(ip.String())
	if err != nil {
		t.Fatalf("GetEgressProfileCache: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persistent cache after refresh")
	}
	if persisted.EgressNetworkType != string(model.EgressNetworkTypeResidential) {
		t.Fatalf("persistent network type: got %q, want %q", persisted.EgressNetworkType, model.EgressNetworkTypeResidential)
	}
}
