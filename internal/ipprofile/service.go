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

	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/topology"
	"github.com/oschwald/maxminddb-golang"
	"golang.org/x/time/rate"
)

type Config struct {
	Pool                     *topology.GlobalNodePool
	LocalASNMMDBPath         string
	LocalPrivacyMMDBPath     string
	IPinfoToken              string
	OnlineRequestsPerMinute  int
	CacheTTL                 time.Duration
	BackgroundEnabled        bool
	BackgroundBatchSize      int
	HTTPClient               *http.Client
}

type Service struct {
	pool              *topology.GlobalNodePool
	asnDB             *maxminddb.Reader
	privacyDB         *maxminddb.Reader
	httpClient        *http.Client
	ipinfoToken       string
	backgroundEnabled bool
	backgroundBatchSize int
	cacheTTL          time.Duration
	onlineLimiter     *rate.Limiter

	queue   chan node.Hash
	stopCh  chan struct{}
	wg      sync.WaitGroup
	queuedMu sync.Mutex
	queued  map[string]struct{}
	cacheMu sync.RWMutex
	cache   map[string]cachedProfile
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

func NewService(cfg Config) *Service {
	svc := &Service{
		pool:                cfg.Pool,
		httpClient:          cfg.HTTPClient,
		ipinfoToken:         strings.TrimSpace(cfg.IPinfoToken),
		backgroundEnabled:   cfg.BackgroundEnabled,
		backgroundBatchSize: cfg.BackgroundBatchSize,
		cacheTTL:            cfg.CacheTTL,
		stopCh:              make(chan struct{}),
		queued:              make(map[string]struct{}),
		cache:               make(map[string]cachedProfile),
	}
	if svc.backgroundBatchSize <= 0 {
		svc.backgroundBatchSize = 16
	}
	svc.queue = make(chan node.Hash, svc.backgroundBatchSize*4)
	if svc.httpClient == nil {
		svc.httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	if svc.cacheTTL <= 0 {
		svc.cacheTTL = 24 * time.Hour
	}
	if rpm := cfg.OnlineRequestsPerMinute; rpm > 0 {
		svc.onlineLimiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(rpm)), 1)
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
	if s == nil || !s.backgroundEnabled {
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

func (s *Service) Enqueue(hash node.Hash) {
	if s == nil || !s.backgroundEnabled {
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
	case s.queue <- hash:
	case <-s.stopCh:
		s.dropQueued(hashHex)
	}
}

func (s *Service) SeedExistingNodes() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.RangeNodes(func(hash node.Hash, entry *node.NodeEntry) bool {
		if entry.GetEgressIP().IsValid() {
			s.Enqueue(hash)
		}
		return true
	})
}

func (s *Service) run() {
	batch := make([]node.Hash, 0, s.backgroundBatchSize)
	for {
		select {
		case <-s.stopCh:
			return
		case first := <-s.queue:
			batch = append(batch[:0], first)
			draining := true
			for draining && len(batch) < s.backgroundBatchSize {
				select {
				case h := <-s.queue:
					batch = append(batch, h)
				default:
					draining = false
				}
			}
			for _, h := range batch {
				s.profileNode(h)
			}
		}
	}
}

func (s *Service) profileNode(hash node.Hash) {
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
	profile, err := s.LookupProfile(context.Background(), ip)
	if err != nil {
		log.Printf("[ipprofile] lookup %s failed: %v", ip.String(), err)
		return
	}
	s.pool.UpdateNodeProfile(hash, profile)
}

func (s *Service) LookupProfile(ctx context.Context, ip netip.Addr) (node.NodeProfile, error) {
	if !ip.IsValid() {
		return node.NodeProfile{}, fmt.Errorf("invalid ip")
	}
	if profile, ok := s.getCached(ip.String()); ok {
		return profile, nil
	}

	profile := node.NodeProfile{
		IP:                 ip,
		NetworkType:        model.EgressNetworkTypeUnknown,
		Source:             model.EgressProfileSourceUnknown,
		ProfileUpdatedAtNs: time.Now().UnixNano(),
	}

	if localProfile, ok := s.lookupLocal(ip); ok {
		profile = mergeProfiles(profile, localProfile)
	}
	if profile.NetworkType == model.EgressNetworkTypeUnknown && s.ipinfoToken != "" {
		if remoteProfile, ok := s.lookupOnline(ctx, ip); ok {
			profile = mergeProfiles(profile, remoteProfile)
		}
	}
	if profile.Provider == "" && profile.ASNName != "" {
		profile.Provider = profile.ASNName
	}
	s.putCached(ip.String(), profile)
	return profile, nil
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

func (s *Service) lookupOnline(ctx context.Context, ip netip.Addr) (node.NodeProfile, bool) {
	if s.onlineLimiter != nil {
		if err := s.onlineLimiter.Wait(ctx); err != nil {
			return node.NodeProfile{}, false
		}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://ipinfo.io/resproxy/"+ip.String()+"?token="+s.ipinfoToken,
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

func (s *Service) getCached(ip string) (node.NodeProfile, bool) {
	s.cacheMu.RLock()
	cached, ok := s.cache[ip]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(cached.expiryAt) {
		return node.NodeProfile{}, false
	}
	return cached.profile, true
}

func (s *Service) putCached(ip string, profile node.NodeProfile) {
	s.cacheMu.Lock()
	s.cache[ip] = cachedProfile{
		profile:  profile,
		expiryAt: time.Now().Add(s.cacheTTL),
	}
	s.cacheMu.Unlock()
}
