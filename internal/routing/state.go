package routing

import (
	"net/netip"

	"github.com/Resinat/Resin/internal/node"
	"github.com/puzpuzpuz/xsync/v4"
)

// PlatformRoutingState encapsulates the routing state for a single platform.
// This struct is stored in the Router's state map.
type PlatformRoutingState struct {
	Leases           *LeaseTable
	IPLoadStats      *IPLoadStats
	PendingRotations *xsync.Map[string, RotateIntent]
}

type RotateIntent struct {
	Source           RotateSource
	PreviousNodeHash node.Hash
	PreviousEgressIP netip.Addr
	RequestedAtNs    int64
}

// NewPlatformRoutingState creates a new state instance.
func NewPlatformRoutingState() *PlatformRoutingState {
	stats := NewIPLoadStats()
	return &PlatformRoutingState{
		Leases:           NewLeaseTable(stats),
		IPLoadStats:      stats,
		PendingRotations: xsync.NewMap[string, RotateIntent](),
	}
}
