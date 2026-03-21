package service

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"net/url"
	"strconv"
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

type NodeExportItem struct {
	Label  string `json:"label"`
	Scheme string `json:"scheme"`
	URI    string `json:"uri"`
}

type NodeExportResponse struct {
	NodeHash string           `json:"node_hash"`
	Exports  []NodeExportItem `json:"exports"`
}

type nodeTLSExportOptions struct {
	Enabled    bool   `json:"enabled"`
	ServerName string `json:"server_name"`
	Insecure   bool   `json:"insecure"`
}

type nodeProxyExportOptions struct {
	Type       string                `json:"type"`
	Server     string                `json:"server"`
	ServerPort int                   `json:"server_port"`
	Port       int                   `json:"port"`
	Username   string                `json:"username"`
	Password   string                `json:"password"`
	TLS        *nodeTLSExportOptions `json:"tls"`
}

type endpointSeedResolution struct {
	ip netip.Addr
	ok bool
}

var lookupNodeEndpointIPs = func(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func (o nodeProxyExportOptions) normalizedType() string {
	return strings.ToLower(strings.TrimSpace(o.Type))
}

func (o nodeProxyExportOptions) effectivePort() int {
	if o.ServerPort > 0 {
		return o.ServerPort
	}
	return o.Port
}

func (o nodeProxyExportOptions) canSeedEgressFromEndpoint() bool {
	switch o.normalizedType() {
	case "http", "socks":
		return strings.TrimSpace(o.Server) != "" && o.effectivePort() > 0
	default:
		return false
	}
}

func decodeNodeProxyExportOptions(raw json.RawMessage) (nodeProxyExportOptions, error) {
	var options nodeProxyExportOptions
	if err := json.Unmarshal(raw, &options); err != nil {
		return nodeProxyExportOptions{}, err
	}
	return options, nil
}

func isDirectProfileCandidateIP(addr netip.Addr) bool {
	if !addr.IsValid() || !addr.IsGlobalUnicast() {
		return false
	}
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() {
		return false
	}
	return true
}

func resolveNodeEndpointSeedIP(
	raw nodeProxyExportOptions,
	cache map[string]endpointSeedResolution,
) (netip.Addr, bool) {
	host := strings.TrimSpace(raw.Server)
	if host == "" {
		return netip.Addr{}, false
	}

	cacheKey := strings.ToLower(host)
	if cache != nil {
		if cached, ok := cache[cacheKey]; ok {
			return cached.ip, cached.ok
		}
	}

	resolve := func() (netip.Addr, bool) {
		if ip, err := netip.ParseAddr(host); err == nil {
			if isDirectProfileCandidateIP(ip) {
				return ip, true
			}
			return netip.Addr{}, false
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		addrs, err := lookupNodeEndpointIPs(ctx, host)
		if err != nil {
			return netip.Addr{}, false
		}
		for _, addr := range addrs {
			if isDirectProfileCandidateIP(addr) {
				return addr, true
			}
		}
		return netip.Addr{}, false
	}

	ip, ok := resolve()
	if cache != nil {
		cache[cacheKey] = endpointSeedResolution{ip: ip, ok: ok}
	}
	return ip, ok
}

func (s *ControlPlaneService) trySeedUnknownNodeEgressFromEndpoint(
	h node.Hash,
	entry *node.NodeEntry,
	cache map[string]endpointSeedResolution,
) bool {
	if s == nil || s.Pool == nil || entry == nil {
		return false
	}
	if !entry.HasOutbound() || entry.IsCircuitOpen() || entry.GetEgressIP().IsValid() {
		return false
	}

	raw, err := decodeNodeProxyExportOptions(entry.RawOptions)
	if err != nil || !raw.canSeedEgressFromEndpoint() {
		return false
	}

	ip, ok := resolveNodeEndpointSeedIP(raw, cache)
	if !ok {
		return false
	}
	s.Pool.UpdateNodeEgressIP(h, &ip, nil)
	return true
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

// ExportNode returns proxy URI exports for raw HTTP/HTTPS/SOCKS nodes.
func (s *ControlPlaneService) ExportNode(hashStr string) (*NodeExportResponse, error) {
	h, err := node.ParseHex(hashStr)
	if err != nil {
		return nil, invalidArg("node_hash: invalid format")
	}
	entry, ok := s.Pool.GetEntry(h)
	if !ok {
		return nil, notFound("node not found")
	}

	raw, err := decodeNodeProxyExportOptions(entry.RawOptions)
	if err != nil {
		return nil, internal("decode node raw options", err)
	}
	port := raw.effectivePort()
	if strings.TrimSpace(raw.Server) == "" || port <= 0 {
		return nil, conflict("node raw options missing server or port")
	}

	switch raw.normalizedType() {
	case "http":
		item := NodeExportItem{
			Label:  "HTTP",
			Scheme: "http",
		}
		query := url.Values{}
		if raw.TLS != nil && raw.TLS.Enabled {
			item.Label = "HTTPS"
			item.Scheme = "https"
			serverName := strings.TrimSpace(raw.TLS.ServerName)
			if serverName != "" {
				query.Set("sni", serverName)
				query.Set("servername", serverName)
				query.Set("peer", serverName)
			}
			if raw.TLS.Insecure {
				query.Set("allowInsecure", "1")
			}
		}
		item.URI = buildProxyExportURI(item.Scheme, raw.Server, port, raw.Username, raw.Password, query)
		return &NodeExportResponse{
			NodeHash: h.Hex(),
			Exports:  []NodeExportItem{item},
		}, nil
	case "socks":
		item := NodeExportItem{
			Label:  "SOCKS5",
			Scheme: "socks5",
			URI:    buildProxyExportURI("socks5", raw.Server, port, raw.Username, raw.Password, nil),
		}
		return &NodeExportResponse{
			NodeHash: h.Hex(),
			Exports:  []NodeExportItem{item},
		}, nil
	default:
		return nil, conflict("node protocol does not support generic HTTP/SOCKS export")
	}
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

func buildProxyExportURI(
	scheme string,
	host string,
	port int,
	username string,
	password string,
	query url.Values,
) string {
	uri := &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	switch {
	case username != "" && password != "":
		uri.User = url.UserPassword(username, password)
	case username != "":
		uri.User = url.User(username)
	case password != "":
		uri.User = url.UserPassword("", password)
	}
	if len(query) > 0 {
		uri.RawQuery = query.Encode()
	}
	return uri.String()
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

// FillSystemUnknownNodes manually pushes all repairable unknown nodes into the
// egress/profile repair pipeline. HTTP/SOCKS nodes with a resolvable raw server
// address are seeded directly to a known egress IP before profiling.
func (s *ControlPlaneService) FillSystemUnknownNodes() (*UnknownNodesFillResult, error) {
	if s == nil || s.Pool == nil {
		return nil, internal("system unknown-node repair unavailable", nil)
	}

	result := &UnknownNodesFillResult{}
	egressQueue := make([]node.Hash, 0, 128)
	profileQueue := make([]node.Hash, 0, 128)
	endpointCache := make(map[string]endpointSeedResolution)

	s.Pool.Range(func(h node.Hash, entry *node.NodeEntry) bool {
		if entry == nil || !entry.HasOutbound() {
			return true
		}
		s.queueUnknownNodeRepair(result, h, entry, true, endpointCache, &egressQueue, &profileQueue)
		return true
	})

	s.flushUnknownNodeRepairQueues(result, true, egressQueue, profileQueue)
	return result, nil
}
