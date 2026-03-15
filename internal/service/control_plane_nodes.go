package service

import (
	"strings"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/probe"
	"github.com/Resinat/Resin/internal/subscription"
)

type NodeReprofileBatchResult struct {
	Requested int      `json:"requested"`
	Accepted  int      `json:"accepted"`
	Failed    []string `json:"failed"`
}

// ------------------------------------------------------------------
// Nodes
// ------------------------------------------------------------------

// NodeFilters holds query filters for listing nodes.
type NodeFilters struct {
	PlatformID              *string
	SubscriptionID          *string
	Region                  *string
	NetworkType             *string
	CircuitOpen             *bool
	HasOutbound             *bool
	Profiled                *bool
	EgressIP                *string
	ProbedSince             *time.Time
	TagKeyword              *string
	MinQualityScore         *int
	MaxReferenceLatencyMs   *int
	MinEgressStabilityScore *int
	MaxCircuitOpenCount     *int
}

// ListNodes returns nodes from the pool with optional filters.
func (s *ControlPlaneService) ListNodes(filters NodeFilters) ([]NodeSummary, error) {
	// If platform_id filter, get the platform view.
	var platformView map[node.Hash]struct{}
	if filters.PlatformID != nil {
		plat, ok := s.Pool.GetPlatform(*filters.PlatformID)
		if !ok {
			return nil, notFound("platform not found")
		}
		platformView = make(map[node.Hash]struct{}, plat.View().Size())
		plat.View().Range(func(h node.Hash) bool {
			platformView[h] = struct{}{}
			return true
		})
	}

	var subNodes map[node.Hash]struct{}
	if filters.SubscriptionID != nil {
		sub := s.SubMgr.Lookup(*filters.SubscriptionID)
		if sub == nil {
			return nil, notFound("subscription not found")
		}
		subNodes = make(map[node.Hash]struct{})
		sub.ManagedNodes().RangeNodes(func(h node.Hash, managed subscription.ManagedNode) bool {
			if managed.Evicted {
				return true
			}
			subNodes[h] = struct{}{}
			return true
		})
	}

	var result []NodeSummary
	appendIfMatched := func(h node.Hash, entry *node.NodeEntry) {
		if !s.nodeEntryMatchesFilters(entry, filters) {
			return
		}
		result = append(result, s.nodeEntryToSummary(h, entry))
	}

	appendIfMatchedHash := func(h node.Hash) {
		entry, ok := s.Pool.GetEntry(h)
		if !ok {
			return
		}
		appendIfMatched(h, entry)
	}

	switch {
	case platformView != nil && subNodes != nil:
		// Iterate the smaller candidate set, then intersect by membership.
		if len(platformView) <= len(subNodes) {
			for h := range platformView {
				if _, ok := subNodes[h]; !ok {
					continue
				}
				appendIfMatchedHash(h)
			}
		} else {
			for h := range subNodes {
				if _, ok := platformView[h]; !ok {
					continue
				}
				appendIfMatchedHash(h)
			}
		}
	case platformView != nil:
		for h := range platformView {
			appendIfMatchedHash(h)
		}
	case subNodes != nil:
		for h := range subNodes {
			appendIfMatchedHash(h)
		}
	default:
		s.Pool.Range(func(h node.Hash, entry *node.NodeEntry) bool {
			appendIfMatched(h, entry)
			return true
		})
	}

	if result == nil {
		result = []NodeSummary{}
	}
	return result, nil
}

func (s *ControlPlaneService) nodeEntryMatchesFilters(entry *node.NodeEntry, filters NodeFilters) bool {
	// Node tag fuzzy search filter.
	if filters.TagKeyword != nil {
		keyword := strings.ToLower(strings.TrimSpace(*filters.TagKeyword))
		if keyword != "" {
			matched := false
			for _, subID := range entry.SubscriptionIDs() {
				sub := s.SubMgr.Lookup(subID)
				if sub == nil {
					continue
				}
				managed, ok := sub.ManagedNodes().LoadNode(entry.Hash)
				if !ok {
					continue
				}
				tags := managed.Tags
				for _, tag := range tags {
					displayTag := sub.Name() + "/" + tag
					if strings.Contains(strings.ToLower(displayTag), keyword) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				return false
			}
		}
	}

	// Region filter.
	if filters.Region != nil {
		region := entry.GetRegion(nil)
		if s.GeoIP != nil {
			region = entry.GetRegion(s.GeoIP.Lookup)
		}
		if region == "" || region != *filters.Region {
			return false
		}
	}
	if filters.NetworkType != nil {
		if string(entry.GetEgressNetworkType()) != strings.ToUpper(strings.TrimSpace(*filters.NetworkType)) {
			return false
		}
	}
	// Circuit open filter.
	if filters.CircuitOpen != nil {
		if entry.IsCircuitOpen() != *filters.CircuitOpen {
			return false
		}
	}
	// Has outbound filter.
	if filters.HasOutbound != nil {
		if entry.HasOutbound() != *filters.HasOutbound {
			return false
		}
	}
	if filters.Profiled != nil {
		if entry.HasProfile() != *filters.Profiled {
			return false
		}
	}
	// Egress IP filter.
	if filters.EgressIP != nil {
		egressIP := entry.GetEgressIP()
		if !egressIP.IsValid() || egressIP.String() != *filters.EgressIP {
			return false
		}
	}
	// Probed since filter.
	if filters.ProbedSince != nil {
		lastUpdate := entry.LastLatencyProbeAttempt.Load()
		if lastUpdate < filters.ProbedSince.UnixNano() {
			return false
		}
	}
	if filters.MinQualityScore != nil && int(entry.QualityScore.Load()) < *filters.MinQualityScore {
		return false
	}
	if filters.MaxReferenceLatencyMs != nil {
		if latencyMs, ok := entry.ReferenceLatencyMs(s.currentLatencyAuthorities()); !ok || latencyMs > float64(*filters.MaxReferenceLatencyMs) {
			return false
		}
	}
	if filters.MinEgressStabilityScore != nil && entry.EgressStabilityScore() < *filters.MinEgressStabilityScore {
		return false
	}
	if filters.MaxCircuitOpenCount != nil && int(entry.CircuitOpenCountTotal.Load()) > *filters.MaxCircuitOpenCount {
		return false
	}
	return true
}

// GetNode returns a single node by hash.
func (s *ControlPlaneService) GetNode(hashStr string) (*NodeSummary, error) {
	h, err := node.ParseHex(hashStr)
	if err != nil {
		return nil, invalidArg("node_hash: invalid format")
	}
	entry, ok := s.Pool.GetEntry(h)
	if !ok {
		return nil, notFound("node not found")
	}
	ns := s.nodeEntryToSummary(h, entry)
	return &ns, nil
}

// ProbeEgress triggers a synchronous egress probe and returns results.
func (s *ControlPlaneService) ProbeEgress(hashStr string) (*probe.EgressProbeResult, error) {
	h, err := node.ParseHex(hashStr)
	if err != nil {
		return nil, invalidArg("node_hash: invalid format")
	}
	entry, ok := s.Pool.GetEntry(h)
	if !ok {
		return nil, notFound("node not found")
	}
	result, err := s.ProbeMgr.ProbeEgressSync(h)
	if err != nil {
		return nil, internal("egress probe failed", err)
	}
	result.Region = entry.GetRegion(nil)
	if s.GeoIP != nil {
		result.Region = entry.GetRegion(s.GeoIP.Lookup)
	}
	return result, nil
}

// ProbeLatency triggers a synchronous latency probe and returns results.
func (s *ControlPlaneService) ProbeLatency(hashStr string) (*probe.LatencyProbeResult, error) {
	h, err := node.ParseHex(hashStr)
	if err != nil {
		return nil, invalidArg("node_hash: invalid format")
	}
	if _, ok := s.Pool.GetEntry(h); !ok {
		return nil, notFound("node not found")
	}
	result, err := s.ProbeMgr.ProbeLatencySync(h)
	if err != nil {
		return nil, internal("latency probe failed", err)
	}
	return result, nil
}

// ReprofileNode refreshes the current node network profile based on its current egress IP.
func (s *ControlPlaneService) ReprofileNode(hashStr string) (*NodeSummary, error) {
	if s.ProfileSvc == nil {
		return nil, internal("profile service unavailable", nil)
	}
	h, err := node.ParseHex(hashStr)
	if err != nil {
		return nil, invalidArg("node_hash: invalid format")
	}
	if _, err := s.ProfileSvc.ReprofileNodeSync(h, true); err != nil {
		switch err.Error() {
		case "node not found":
			return nil, notFound("node not found")
		case "node has no known egress ip":
			return nil, conflict("node has no known egress ip")
		default:
			return nil, internal("reprofile node failed", err)
		}
	}
	return s.GetNode(hashStr)
}

// ReprofileNodes refreshes multiple nodes synchronously.
func (s *ControlPlaneService) ReprofileNodes(hashes []string) (*NodeReprofileBatchResult, error) {
	if s.ProfileSvc == nil {
		return nil, internal("profile service unavailable", nil)
	}
	result := &NodeReprofileBatchResult{
		Requested: len(hashes),
		Failed:    []string{},
	}
	for _, hashStr := range hashes {
		h, err := node.ParseHex(hashStr)
		if err != nil {
			result.Failed = append(result.Failed, hashStr+": invalid hash")
			continue
		}
		if _, err := s.ProfileSvc.ReprofileNodeSync(h, true); err != nil {
			result.Failed = append(result.Failed, hashStr+": "+err.Error())
			continue
		}
		result.Accepted++
	}
	return result, nil
}

// QueueReprofileKnownNodes enqueues all nodes with known egress IP for forced reprofiling.
func (s *ControlPlaneService) QueueReprofileKnownNodes() (*NodeReprofileBatchResult, error) {
	if s.ProfileSvc == nil {
		return nil, internal("profile service unavailable", nil)
	}
	accepted := s.ProfileSvc.SeedExistingNodes(true)
	return &NodeReprofileBatchResult{
		Requested: accepted,
		Accepted:  accepted,
		Failed:    []string{},
	}, nil
}
