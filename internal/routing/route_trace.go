package routing

import (
	"net/netip"
	"strings"
)

type RouteAction string

const (
	RouteActionRandomAssign         RouteAction = "RANDOM_ASSIGN"
	RouteActionLeaseCreate          RouteAction = "LEASE_CREATE"
	RouteActionLeaseReuse           RouteAction = "LEASE_REUSE"
	RouteActionForcedRotate         RouteAction = "FORCED_ROTATE"
	RouteActionManualRotateReassign RouteAction = "MANUAL_ROTATE_REASSIGN"
	RouteActionTTLReassign          RouteAction = "TTL_REASSIGN"
	RouteActionPlatformReassign     RouteAction = "PLATFORM_REASSIGN"
	RouteActionSameIPFailover       RouteAction = "SAME_IP_FAILOVER"
)

type RotateSource string

const (
	RotateSourceUnknown       RotateSource = ""
	RotateSourceRequestHeader RotateSource = "REQUEST_HEADER"
	RotateSourceAdminAPI      RotateSource = "ADMIN_API"
	RotateSourceTokenAPI      RotateSource = "TOKEN_API"
	RotateSourceTTLExpired    RotateSource = "TTL_EXPIRED"
	RotateSourceLeaseInvalid  RotateSource = "LEASE_UNAVAILABLE"
)

func normalizeRotateSource(raw string) RotateSource {
	switch RotateSource(strings.ToUpper(strings.TrimSpace(raw))) {
	case RotateSourceRequestHeader:
		return RotateSourceRequestHeader
	case RotateSourceAdminAPI:
		return RotateSourceAdminAPI
	case RotateSourceTokenAPI:
		return RotateSourceTokenAPI
	case RotateSourceTTLExpired:
		return RotateSourceTTLExpired
	case RotateSourceLeaseInvalid:
		return RotateSourceLeaseInvalid
	default:
		return RotateSourceUnknown
	}
}

func invalidIPString(ip netip.Addr) string {
	if !ip.IsValid() {
		return ""
	}
	return ip.String()
}
