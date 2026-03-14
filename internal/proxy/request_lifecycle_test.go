package proxy

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/Resinat/Resin/internal/routing"
)

func TestRequestLifecycleSetRouteResultOmitsInvalidIPs(t *testing.T) {
	lifecycle := newRequestLifecycle(
		NoOpEventEmitter{},
		&http.Request{Method: http.MethodConnect, RemoteAddr: "127.0.0.1:12345"},
		ProxyTypeForward,
		true,
	)

	lifecycle.setRouteResult(routing.RouteResult{
		PlatformID:       "plat-1",
		PlatformName:     "StickyPlatform",
		AccessMode:       "STICKY",
		LeaseAction:      "LEASE_CREATE",
		EgressIP:         netip.MustParseAddr("23.144.124.194"),
		PreviousEgressIP: netip.Addr{},
	})

	if lifecycle.log.EgressIP != "23.144.124.194" {
		t.Fatalf("egress ip: got %q", lifecycle.log.EgressIP)
	}
	if lifecycle.log.PreviousEgressIP != "" {
		t.Fatalf("previous egress ip: got %q, want empty", lifecycle.log.PreviousEgressIP)
	}
}

func TestRequestLifecycleSetRouteResultKeepsValidPreviousIP(t *testing.T) {
	lifecycle := newRequestLifecycle(
		NoOpEventEmitter{},
		&http.Request{Method: http.MethodConnect, RemoteAddr: "127.0.0.1:12345"},
		ProxyTypeForward,
		true,
	)

	lifecycle.setRouteResult(routing.RouteResult{
		PreviousEgressIP: netip.MustParseAddr("23.144.124.194"),
	})

	if lifecycle.log.PreviousEgressIP != "23.144.124.194" {
		t.Fatalf("previous egress ip: got %q", lifecycle.log.PreviousEgressIP)
	}
}
