package topology

import (
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/subscription"
)

// HardDeleteNode removes a node from all subscription managed views and from
// the global node pool. The pool callbacks remain responsible for persistence
// cleanup of node rows and subscription-node relations.
func HardDeleteNode(subManager *SubscriptionManager, pool *GlobalNodePool, hash node.Hash) bool {
	if subManager == nil || pool == nil {
		return false
	}

	referencedSubs := make(map[string]struct{})
	if entry, ok := pool.GetEntry(hash); ok {
		for _, subID := range entry.SubscriptionIDs() {
			referencedSubs[subID] = struct{}{}
		}
	}

	subManager.Range(func(id string, sub *subscription.Subscription) bool {
		if sub == nil {
			return true
		}
		if _, ok := sub.ManagedNodes().LoadNode(hash); ok {
			referencedSubs[id] = struct{}{}
		}
		return true
	})

	if len(referencedSubs) == 0 {
		return false
	}

	for subID := range referencedSubs {
		if sub := subManager.Lookup(subID); sub != nil {
			sub.WithOpLock(func() {
				sub.ManagedNodes().Delete(hash)
			})
		}
		pool.RemoveNodeFromSub(hash, subID)
	}
	return true
}
