package proxy

import (
	"github.com/Resinat/Resin/internal/outbound"
	"github.com/Resinat/Resin/internal/routing"
	"github.com/sagernet/sing-box/adapter"
)

type routedOutbound struct {
	Route    routing.RouteResult
	Outbound adapter.Outbound
}

func resolveRoutedOutbound(
	router *routing.Router,
	pool outbound.PoolAccessor,
	platformName string,
	account string,
	target string,
) (routedOutbound, *ProxyError) {
	return resolveRoutedOutboundWithOptions(
		router,
		pool,
		platformName,
		account,
		target,
		routing.RouteOptions{},
	)
}

func resolveRoutedOutboundWithOptions(
	router *routing.Router,
	pool outbound.PoolAccessor,
	platformName string,
	account string,
	target string,
	opts routing.RouteOptions,
) (routedOutbound, *ProxyError) {
	result, err := router.RouteRequestWithOptions(platformName, account, target, opts)
	if err != nil {
		return routedOutbound{}, mapRouteError(err)
	}

	entry, ok := pool.GetEntry(result.NodeHash)
	if !ok {
		return routedOutbound{}, ErrNoAvailableNodes
	}
	obPtr := entry.Outbound.Load()
	if obPtr == nil {
		return routedOutbound{}, ErrNoAvailableNodes
	}

	return routedOutbound{
		Route:    result,
		Outbound: *obPtr,
	}, nil
}
