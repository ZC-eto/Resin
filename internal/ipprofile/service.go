package ipprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Resinat/Resin/internal/config"
	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/state"
	"github.com/Resinat/Resin/internal/topology"
	"github.com/oschwald/maxminddb-golang"
	"golang.org/x/time/rate"
)

type RuntimeSettings struct {
	LocalLookupEnabled      bool
	OnlineProvider          config.IPProfileOnlineProvider
	OnlineAPIKey            string
	OnlineRequestsPerMinute int
	CacheTTL                time.Duration
	BackgroundEnabled       bool
	RefreshOnEgressChange   bool
}

type Config struct {
	Pool                 *topology.GlobalNodePool
	Engine               *state.StateEngine
	LocalASNMMDBPath     string
	LocalPrivacyMMDBPath string
	LegacyIPinfoToken    string
	BackgroundBatchSize  int
	RuntimeSettings      func() RuntimeSettings
	HTTPClient           *http.Client
}

type Service struct {
	pool                *topology.GlobalNodePool
	engine              *state.StateEngine
	asnDB               *maxminddb.Reader
	privacyDB           *maxminddb.Reader
	httpClient          *http.Client
	legacyIPinfoToken   string
	runtimeSettings     func() RuntimeSettings
	backgroundBatchSize int

	queue    chan queueItem
	stopCh   chan struct{}
	wg       sync.WaitGroup
	queuedMu sync.Mutex
	queued   map[string]struct{}

	cacheMu sync.RWMutex
	cache   map[string]cachedProfile

	limiterMu        sync.Mutex
	onlineLimiter    *rate.Limiter
	onlineLimiterRPM int
}

type queueItem struct {
	hash  node.Hash
	force bool
}

type cachedProfile struct {
	profile  node.NodeProfile
	expiryAt time.Time
}

type asnRecord struct {
	ASN    string `maxminddb:"asn"`
	Name   string `maxminddb:"name"`
	Type   string `maxminddb:"type"`
	Domain string `maxminddb:"domain"`
}

type privacyRecord struct {
	Hosting bool   `maxminddb:"hosting"`
	Proxy   bool   `maxminddb:"proxy"`
	Tor     bool   `maxminddb:"tor"`
	Relay   bool   `maxminddb:"relay"`
	VPN     bool   `maxminddb:"vpn"`
	Service string `maxminddb:"service"`
}

type residentialProxyResponse struct {
	IP              string `json:"ip"`
	LastSeen        string `json:"last_seen"`
	PercentDaysSeen int    `json:"percent_days_seen"`
	Service         string `json:"service"`
}

type proxycheckIPResponse struct {
	Proxy        string `json:"proxy"`
	Type         string `json:"type"`
	Provider     string `json:"provider"`
	Organisation string `json:"organisation"`
	ASN          string `json:"asn"`
}

func NewService(cfg Config) *Service {
	svc := &Service{
		pool:                cfg.Pool,
		engine:              cfg.Engine,
		httpClient:          cfg.HTTPClient,
		legacyIPinfoToken:   strings.TrimSpace(cfg.LegacyIPinfoToken),
		runtimeSettings:     cfg.RuntimeSettings,
		backgroundBatchSize: cfg.BackgroundBatchSize,
		stopCh:              make(chan struct{}),
		queued:              make(map[string]struct{}),
		cache:               make(map[string]cachedProfile),
	}
	if svc.backgroundBatchSize <= 0 {
		svc.backgroundBatchSize = 16
	}
	svc.queue = make(chan queueItem, svc.backgroundBatchSize*8)
	if svc.httpClient == nil {
		svc.httpClient = &http.Client{Timeout: 8 * time.Second}
	}
	if strings.TrimSpace(cfg.LocalASNMMDBPath) != "" {
		db, err := maxminddb.Open(cfg.LocalASNMMDBPath)
		if err != nil {
			log.Printf("[ipprofile] open ASN MMDB failed: %v", err)
		} else {
			svc.asnDB = db
		}
	}
	if strings.TrimSpace(cfg.LocalPrivacyMMDBPath) != "" {
		db, err := maxminddb.Open(cfg.LocalPrivacyMMDBPath)
		if err != nil {
			log.Printf("[ipprofile] open privacy MMDB failed: %v", err)
		} else {
			svc.privacyDB = db
		}
	}
	return svc
}

func (s *Service) Start() {
	if s == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run()
	}()
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	close(s.stopCh)
	s.wg.Wait()
	if s.asnDB != nil {
		_ = s.asnDB.Close()
	}
	if s.privacyDB != nil {
		_ = s.privacyDB.Close()
	}
}

func (s *Service) settingsSnapshot() RuntimeSettings {
	if s == nil || s.runtimeSettings == nil {
		return RuntimeSettings{
			LocalLookupEnabled:      true,
			OnlineProvider:          providerFromLegacyToken(s.legacyIPinfoToken),
			OnlineAPIKey:            strings.TrimSpace(s.legacyIPinfoToken),
			OnlineRequestsPerMinute: 120,
			CacheTTL:                30 * 24 * time.Hour,
			BackgroundEnabled:       true,
			RefreshOnEgressChange:   true,
		}
	}
	settings := s.runtimeSettings()
	settings.OnlineProvider = config.NormalizeIPProfileOnlineProvider(string(settings.OnlineProvider))
	settings.OnlineAPIKey = strings.TrimSpace(settings.OnlineAPIKey)
	if settings.OnlineProvider == config.IPProfileOnlineProviderIPInfo && settings.OnlineAPIKey == "" {
		settings.OnlineAPIKey = strings.TrimSpace(s.legacyIPinfoToken)
	}
	if settings.OnlineRequestsPerMinute <= 0 {
		settings.OnlineRequestsPerMinute = 120
	}
	if settings.CacheTTL <= 0 {
		settings.CacheTTL = 30 * 24 * time.Hour
	}
	return settings
}

func providerFromLegacyToken(token string) config.IPProfileOnlineProvider {
	if strings.TrimSpace(token) == "" {
		return config.IPProfileOnlineProviderDisabled
	}
	return config.IPProfileOnlineProviderIPInfo
}

func (s *Service) enqueue(hash node.Hash, force bool, manual bool) {
	if s == nil {
		return
	}
	if !manual && !s.settingsSnapshot().BackgroundEnabled {
		return
	}

	hashHex := hash.Hex()
	s.queuedMu.Lock()
	if _, exists := s.queued[hashHex]; exists {
		s.queuedMu.Unlock()
		return
	}
	s.queued[hashHex] = struct{}{}
	s.queuedMu.Unlock()

	select {
	case s.queue <- queueItem{hash: hash, force: force}:
	case <-s.stopCh:
		s.dropQueued(hashHex)
	}
}

func (s *Service) Enqueue(hash node.Hash) {
	s.enqueue(hash, false, false)
}

func (s *Service) EnqueueForce(hash node.Hash) {
	s.enqueue(hash, true, true)
}

func (s *Service) SeedExistingNodes(force bool) int {
	if s == nil || s.pool == nil {
		return 0
	}
	hashes := make([]node.Hash, 0, 256)
	s.pool.RangeNodes(func(hash node.Hash, entry *node.NodeEntry) bool {
		if entry.GetEgressIP().IsValid() {
			hashes = append(hashes, hash)
		}
		return true
	})
	if len(hashes) == 0 {
		return 0
	}
	go s.enqueueSeedBatch(hashes, force)
	return len(hashes)
}

func (s *Service) enqueueSeedBatch(hashes []node.Hash, force bool) {
	for _, hash := range hashes {
		if force {
			s.EnqueueForce(hash)
			continue
		}
		s.Enqueue(hash)
	}
}

func (s *Service) run() {
	batch := make([]queueItem, 0, maxInt(1, s.backgroundBatchSize))
	for {
		select {
		case <-s.stopCh:
			return
		case first := <-s.queue:
			batch = append(batch[:0], first)
			draining := true
			for draining && len(batch) < s.backgroundBatchSize {
				select {
				case item := <-s.queue:
					batch = append(batch, item)
				default:
					draining = false
				}
			}
			for _, item := range batch {
				s.profileNode(item.hash, item.force)
			}
		}
	}
}

func (s *Service) profileNode(hash node.Hash, force bool) {
	defer s.dropQueued(hash.Hex())
	if s == nil || s.pool == nil {
		return
	}
	entry, ok := s.pool.GetEntry(hash)
	if !ok || entry == nil {
		return
	}
	ip := entry.GetEgressIP()
	if !ip.IsValid() {
		return
	}
	profile, err := s.LookupProfile(context.Background(), ip, force)
	if err != nil {
		log.Printf("[ipprofile] lookup %s failed: %v", ip.String(), err)
		return
	}
	s.pool.UpdateNodeProfile(hash, profile)
}

func (s *Service) ReprofileNodeSync(hash node.Hash, force bool) (node.NodeProfile, error) {
	if s == nil || s.pool == nil {
		return node.NodeProfile{}, fmt.Errorf("profile service unavailable")
	}
	entry, ok := s.pool.GetEntry(hash)
	if !ok || entry == nil {
		return node.NodeProfile{}, fmt.Errorf("node not found")
	}
	ip := entry.GetEgressIP()
	if !ip.IsValid() {
		return node.NodeProfile{}, fmt.Errorf("node has no known egress ip")
	}
	profile, err := s.LookupProfile(context.Background(), ip, force)
	if err != nil {
		return node.NodeProfile{}, err
	}
	s.pool.UpdateNodeProfile(hash, profile)
	return profile, nil
}

func (s *Service) LookupProfile(ctx context.Context, ip netip.Addr, force bool) (node.NodeProfile, error) {
	if !ip.IsValid() {
		return node.NodeProfile{}, fmt.Errorf("invalid ip")
	}
	settings := s.settingsSnapshot()
	if !force {
		if profile, ok := s.getCached(ip.String(), settings.CacheTTL); ok {
			if !shouldRefreshCachedUnknownProfile(profile, settings) {
				return profile, nil
			}
		}
		if profile, ok := s.getPersistentCached(ip.String(), settings.CacheTTL); ok {
			if !shouldRefreshCachedUnknownProfile(profile, settings) {
				s.putCached(ip.String(), profile, settings.CacheTTL)
				return profile, nil
			}
		}
	}

	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceUnknown,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}

	if settings.LocalLookupEnabled {
		if localProfile, ok := s.lookupLocal(ip); ok {
			profile = mergeProfiles(profile, localProfile)
		}
	}
	if profile.NetworkType == model.EgressNetworkTypeUnknown {
		if remoteProfile, ok := s.lookupOnline(ctx, ip, settings); ok {
			profile = mergeProfiles(profile, remoteProfile)
		}
	}
	if profile.Provider == "" && profile.ASNName != "" {
		profile.Provider = profile.ASNName
	}

	s.putCached(ip.String(), profile, settings.CacheTTL)
	s.putPersistentCached(profile)
	return profile, nil
}

func shouldRefreshCachedUnknownProfile(profile node.NodeProfile, settings RuntimeSettings) bool {
	if model.NormalizeEgressNetworkType(string(profile.NetworkType)) != model.EgressNetworkTypeUnknown {
		return false
	}
	if config.NormalizeIPProfileOnlineProvider(string(settings.OnlineProvider)) == config.IPProfileOnlineProviderDisabled {
		return false
	}
	if strings.TrimSpace(settings.OnlineAPIKey) == "" {
		return false
	}
	switch model.NormalizeEgressProfileSource(string(profile.Source)) {
	case model.EgressProfileSourceOnline, model.EgressProfileSourceLocalPlusOnline:
		return false
	default:
		return true
	}
}

func (s *Service) lookupLocal(ip netip.Addr) (node.NodeProfile, bool) {
	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceLocal,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}
	found := false

	if s.asnDB != nil {
		var record asnRecord
		if err := s.asnDB.Lookup(ip.AsSlice(), &record); err == nil {
			profile.ASN = parseASN(record.ASN)
			profile.ASNName = strings.TrimSpace(record.Name)
			profile.ASNType = strings.ToUpper(strings.TrimSpace(record.Type))
			if profile.Provider == "" {
				profile.Provider = strings.TrimSpace(record.Name)
			}
			found = found || record.ASN != "" || record.Name != "" || record.Type != ""
		}
	}

	var privacy privacyRecord
	if s.privacyDB != nil {
		if err := s.privacyDB.Lookup(ip.AsSlice(), &privacy); err == nil {
			if privacy.Service != "" {
				profile.Provider = strings.TrimSpace(privacy.Service)
			}
			found = found || privacy.Hosting || privacy.Proxy || privacy.Tor || privacy.Relay || privacy.VPN || privacy.Service != ""
		}
	}

	profile.NetworkType = classifyLocalNetworkType(profile.ASNType, privacy)
	return profile, found
}

func (s *Service) lookupOnline(ctx context.Context, ip netip.Addr, settings RuntimeSettings) (node.NodeProfile, bool) {
	if settings.OnlineProvider == config.IPProfileOnlineProviderDisabled {
		return node.NodeProfile{}, false
	}
	apiKey := strings.TrimSpace(settings.OnlineAPIKey)
	if apiKey == "" {
		return node.NodeProfile{}, false
	}
	if err := s.waitOnline(ctx, settings.OnlineRequestsPerMinute); err != nil {
		return node.NodeProfile{}, false
	}

	switch settings.OnlineProvider {
	case config.IPProfileOnlineProviderProxycheck:
		return s.lookupProxycheck(ctx, ip, apiKey)
	case config.IPProfileOnlineProviderIPInfo:
		return s.lookupIPInfo(ctx, ip, apiKey)
	default:
		return node.NodeProfile{}, false
	}
}

func (s *Service) waitOnline(ctx context.Context, rpm int) error {
	if rpm <= 0 {
		rpm = 120
	}
	s.limiterMu.Lock()
	if s.onlineLimiter == nil || s.onlineLimiterRPM != rpm {
		s.onlineLimiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(rpm)), 1)
		s.onlineLimiterRPM = rpm
	}
	limiter := s.onlineLimiter
	s.limiterMu.Unlock()
	return limiter.Wait(ctx)
}

func (s *Service) lookupIPInfo(ctx context.Context, ip netip.Addr, apiKey string) (node.NodeProfile, bool) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://ipinfo.io/resproxy/"+ip.String()+"?token="+apiKey,
		nil,
	)
	if err != nil {
		return node.NodeProfile{}, false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return node.NodeProfile{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return node.NodeProfile{}, false
	}
	if resp.StatusCode != http.StatusOK {
		return node.NodeProfile{}, false
	}

	var payload residentialProxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return node.NodeProfile{}, false
	}

	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        classifyRemoteNetworkType(payload.Service),
		Provider:           strings.TrimSpace(payload.Service),
		Source:             model.EgressProfileSourceOnline,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}
	return profile, true
}

func (s *Service) lookupProxycheck(ctx context.Context, ip netip.Addr, apiKey string) (node.NodeProfile, bool) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://proxycheck.io/v2/"+ip.String()+"?key="+apiKey+"&vpn=1&asn=1&risk=1",
		nil,
	)
	if err != nil {
		return node.NodeProfile{}, false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return node.NodeProfile{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return node.NodeProfile{}, false
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return node.NodeProfile{}, false
	}
	var status string
	if raw, ok := payload["status"]; ok {
		_ = json.Unmarshal(raw, &status)
	}
	if !strings.EqualFold(strings.TrimSpace(status), "ok") {
		return node.NodeProfile{}, false
	}
	rawIP, ok := payload[ip.String()]
	if !ok {
		return node.NodeProfile{}, false
	}
	var detail proxycheckIPResponse
	if err := json.Unmarshal(rawIP, &detail); err != nil {
		return node.NodeProfile{}, false
	}
	provider := strings.TrimSpace(detail.Provider)
	if provider == "" {
		provider = strings.TrimSpace(detail.Organisation)
	}
	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        classifyProxycheckNetworkType(detail.Type, detail.Proxy),
		ASN:                parseASN(detail.ASN),
		Provider:           provider,
		Source:             model.EgressProfileSourceOnline,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}
	return profile, true
}

func classifyLocalNetworkType(asnType string, privacy privacyRecord) model.EgressNetworkType {
	switch {
	case privacy.Hosting || privacy.Proxy || privacy.Relay || privacy.Tor || privacy.VPN:
		return model.EgressNetworkTypeDatacenter
	}

	switch strings.ToLower(strings.TrimSpace(asnType)) {
	case "mobile":
		return model.EgressNetworkTypeMobile
	case "hosting", "business", "education", "government":
		return model.EgressNetworkTypeDatacenter
	default:
		return model.EgressNetworkTypeUnknown
	}
}

func classifyRemoteNetworkType(service string) model.EgressNetworkType {
	switch {
	case strings.HasSuffix(service, "_mobile"):
		return model.EgressNetworkTypeMobile
	case strings.HasSuffix(service, "_datacenter"):
		return model.EgressNetworkTypeDatacenter
	case strings.TrimSpace(service) != "":
		return model.EgressNetworkTypeResidential
	default:
		return model.EgressNetworkTypeUnknown
	}
}

func classifyProxycheckNetworkType(networkType, proxyFlag string) model.EgressNetworkType {
	normalized := strings.ToLower(strings.TrimSpace(networkType))
	switch {
	case strings.Contains(normalized, "residential"):
		return model.EgressNetworkTypeResidential
	case strings.Contains(normalized, "wireless"), strings.Contains(normalized, "mobile"), strings.Contains(normalized, "cell"):
		return model.EgressNetworkTypeMobile
	case strings.Contains(normalized, "hosting"),
		strings.Contains(normalized, "business"),
		strings.Contains(normalized, "education"),
		strings.Contains(normalized, "government"):
		return model.EgressNetworkTypeDatacenter
	case strings.EqualFold(strings.TrimSpace(proxyFlag), "yes"):
		return model.EgressNetworkTypeDatacenter
	default:
		return model.EgressNetworkTypeUnknown
	}
}

func mergeProfiles(base node.NodeProfile, overlay node.NodeProfile) node.NodeProfile {
	if overlay.NetworkType != model.EgressNetworkTypeUnknown {
		base.NetworkType = overlay.NetworkType
	}
	if overlay.ASN != 0 {
		base.ASN = overlay.ASN
	}
	if overlay.ASNName != "" {
		base.ASNName = overlay.ASNName
	}
	if overlay.ASNType != "" {
		base.ASNType = overlay.ASNType
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	switch {
	case base.Source == model.EgressProfileSourceUnknown:
		base.Source = overlay.Source
	case base.Source != overlay.Source && overlay.Source != model.EgressProfileSourceUnknown:
		base.Source = model.EgressProfileSourceLocalPlusOnline
	}
	if overlay.ProfileUpdatedAtNs > base.ProfileUpdatedAtNs {
		base.ProfileUpdatedAtNs = overlay.ProfileUpdatedAtNs
	}
	return base
}

func parseASN(raw string) int64 {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(raw), "AS"))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func (s *Service) dropQueued(hashHex string) {
	s.queuedMu.Lock()
	delete(s.queued, hashHex)
	s.queuedMu.Unlock()
}

func (s *Service) getCached(ip string, ttl time.Duration) (node.NodeProfile, bool) {
	s.cacheMu.RLock()
	cached, ok := s.cache[ip]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(cached.expiryAt) {
		return node.NodeProfile{}, false
	}
	return cached.profile, true
}

func (s *Service) putCached(ip string, profile node.NodeProfile, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	s.cacheMu.Lock()
	s.cache[ip] = cachedProfile{
		profile:  profile,
		expiryAt: time.Now().Add(ttl),
	}
	s.cacheMu.Unlock()
}

func (s *Service) getPersistentCached(ip string, ttl time.Duration) (node.NodeProfile, bool) {
	if s == nil || s.engine == nil || s.engine.CacheRepo == nil {
		return node.NodeProfile{}, false
	}
	entry, err := s.engine.GetEgressProfileCache(ip)
	if err != nil || entry == nil {
		return node.NodeProfile{}, false
	}
	if entry.EgressProfileUpdatedAtNs <= 0 {
		return node.NodeProfile{}, false
	}
	if ttl > 0 && time.Since(time.Unix(0, entry.EgressProfileUpdatedAtNs)) > ttl {
		return node.NodeProfile{}, false
	}
	parsedIP, err := netip.ParseAddr(entry.EgressIP)
	if err != nil {
		return node.NodeProfile{}, false
	}
	return node.NodeProfile{
		IP:                 parsedIP,
		NetworkType:        model.NormalizeEgressNetworkType(entry.EgressNetworkType),
		ASN:                entry.EgressASN,
		ASNName:            entry.EgressASNName,
		ASNType:            entry.EgressASNType,
		Provider:           entry.EgressProvider,
		Source:             model.NormalizeEgressProfileSource(entry.EgressProfileSource),
		ProfileUpdatedAtNs: entry.EgressProfileUpdatedAtNs,
	}, true
}

func (s *Service) putPersistentCached(profile node.NodeProfile) {
	if s == nil || s.engine == nil || s.engine.CacheRepo == nil || !profile.IP.IsValid() {
		return
	}
	entry := model.EgressProfileCacheEntry{
		EgressIP:                 profile.IP.String(),
		EgressNetworkType:        string(model.NormalizeEgressNetworkType(string(profile.NetworkType))),
		EgressASN:                profile.ASN,
		EgressASNName:            strings.TrimSpace(profile.ASNName),
		EgressASNType:            strings.TrimSpace(profile.ASNType),
		EgressProvider:           strings.TrimSpace(profile.Provider),
		EgressProfileSource:      string(model.NormalizeEgressProfileSource(string(profile.Source))),
		EgressProfileUpdatedAtNs: profile.ProfileUpdatedAtNs,
	}
	if err := s.engine.UpsertEgressProfileCache(entry); err != nil {
		log.Printf("[ipprofile] persist cache %s failed: %v", entry.EgressIP, err)
	}
}

func (s *Service) DeleteCachedIPIfUnused(ip netip.Addr) bool {
	if s == nil || !ip.IsValid() {
		return false
	}
	ipStr := ip.String()
	if s.pool != nil {
		inUse := false
		s.pool.RangeNodes(func(_ node.Hash, entry *node.NodeEntry) bool {
			if entry == nil {
				return true
			}
			if entry.GetEgressIP() == ip {
				inUse = true
				return false
			}
			return true
		})
		if inUse {
			return false
		}
	}

	s.cacheMu.Lock()
	delete(s.cache, ipStr)
	s.cacheMu.Unlock()

	if s.engine == nil || s.engine.CacheRepo == nil {
		return true
	}
	if err := s.engine.DeleteEgressProfileCache(ipStr); err != nil {
		log.Printf("[ipprofile] delete cache %s failed: %v", ipStr, err)
		return false
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
