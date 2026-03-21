package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/state"
	"github.com/Resinat/Resin/internal/subscription"
	"github.com/Resinat/Resin/internal/topology"
)

// ------------------------------------------------------------------
// Subscription
// ------------------------------------------------------------------

// SubscriptionResponse is the API response for a subscription.
type SubscriptionResponse struct {
	ID                       string                       `json:"id"`
	Name                     string                       `json:"name"`
	SourceType               string                       `json:"source_type"`
	URL                      string                       `json:"url"`
	Content                  string                       `json:"content"`
	Sources                  []SubscriptionSourceResponse `json:"sources"`
	UpdateInterval           string                       `json:"update_interval"`
	NodeCount                int                          `json:"node_count"`
	HealthyNodeCount         int                          `json:"healthy_node_count"`
	ResidentialNodeCount     int                          `json:"residential_node_count"`
	DatacenterNodeCount      int                          `json:"datacenter_node_count"`
	MobileNodeCount          int                          `json:"mobile_node_count"`
	UnknownNodeCount         int                          `json:"unknown_node_count"`
	PendingEgressNodeCount   int                          `json:"pending_egress_node_count"`
	PendingProfileNodeCount  int                          `json:"pending_profile_node_count"`
	ProfiledUnknownNodeCount int                          `json:"profiled_unknown_node_count"`
	AverageQualityScore      *float64                     `json:"average_quality_score,omitempty"`
	Ephemeral                bool                         `json:"ephemeral"`
	EphemeralNodeEvictDelay  string                       `json:"ephemeral_node_evict_delay"`
	Enabled                  bool                         `json:"enabled"`
	CreatedAt                string                       `json:"created_at"`
	LastChecked              string                       `json:"last_checked,omitempty"`
	LastUpdated              string                       `json:"last_updated,omitempty"`
	LastError                string                       `json:"last_error,omitempty"`
}

type SubscriptionSourceResponse struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

type SubscriptionSourceInput struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Enabled *bool  `json:"enabled"`
}

type UnknownNodesFillResult struct {
	Matched       int `json:"matched"`
	SeededEgress  int `json:"seeded_egress"`
	QueuedEgress  int `json:"queued_egress"`
	QueuedProfile int `json:"queued_profile"`
	Skipped       int `json:"skipped"`
	Failed        int `json:"failed"`
}

type SubscriptionFillUnknownNodesResult = UnknownNodesFillResult

type subscriptionUnknownNodeCounts struct {
	pendingEgress   int
	pendingProfile  int
	profiledUnknown int
}

func (s *ControlPlaneService) subToResponse(sub *subscription.Subscription) SubscriptionResponse {
	nodeCount := 0
	healthyNodeCount := 0
	residentialNodeCount := 0
	datacenterNodeCount := 0
	mobileNodeCount := 0
	pendingEgressNodeCount := 0
	pendingProfileNodeCount := 0
	profiledUnknownNodeCount := 0
	var qualitySum int
	var qualityCount int
	if managed := sub.ManagedNodes(); managed != nil {
		managed.RangeNodes(func(h node.Hash, n subscription.ManagedNode) bool {
			if n.Evicted {
				return true
			}
			nodeCount++
			if s != nil && s.Pool != nil {
				entry, ok := s.Pool.GetEntry(h)
				if ok && entry.IsHealthy() {
					healthyNodeCount++
				}
				if ok {
					unknownCounts := s.classifySubscriptionUnknownNodeCounts(h, entry)
					pendingEgressNodeCount += unknownCounts.pendingEgress
					pendingProfileNodeCount += unknownCounts.pendingProfile
					profiledUnknownNodeCount += unknownCounts.profiledUnknown

					switch entry.GetEgressNetworkType() {
					case model.EgressNetworkTypeResidential:
						residentialNodeCount++
					case model.EgressNetworkTypeDatacenter:
						datacenterNodeCount++
					case model.EgressNetworkTypeMobile:
						mobileNodeCount++
					}
					if entry.HasProfile() {
						qualitySum += int(entry.QualityScore.Load())
						qualityCount++
					}
				}
			}
			return true
		})
	}

	var averageQualityScore *float64
	if qualityCount > 0 {
		value := float64(qualitySum) / float64(qualityCount)
		averageQualityScore = &value
	}

	resp := SubscriptionResponse{
		ID:                       sub.ID,
		Name:                     sub.Name(),
		SourceType:               sub.SourceType(),
		URL:                      sub.URL(),
		Content:                  sub.Content(),
		Sources:                  toSubscriptionSourceResponses(sub.Sources()),
		UpdateInterval:           time.Duration(sub.UpdateIntervalNs()).String(),
		NodeCount:                nodeCount,
		HealthyNodeCount:         healthyNodeCount,
		ResidentialNodeCount:     residentialNodeCount,
		DatacenterNodeCount:      datacenterNodeCount,
		MobileNodeCount:          mobileNodeCount,
		UnknownNodeCount:         pendingEgressNodeCount + pendingProfileNodeCount + profiledUnknownNodeCount,
		PendingEgressNodeCount:   pendingEgressNodeCount,
		PendingProfileNodeCount:  pendingProfileNodeCount,
		ProfiledUnknownNodeCount: profiledUnknownNodeCount,
		AverageQualityScore:      averageQualityScore,
		Ephemeral:                sub.Ephemeral(),
		EphemeralNodeEvictDelay:  time.Duration(sub.EphemeralNodeEvictDelayNs()).String(),
		Enabled:                  sub.Enabled(),
		CreatedAt:                time.Unix(0, sub.CreatedAtNs).UTC().Format(time.RFC3339Nano),
	}
	if lc := sub.LastCheckedNs.Load(); lc > 0 {
		resp.LastChecked = time.Unix(0, lc).UTC().Format(time.RFC3339Nano)
	}
	if lu := sub.LastUpdatedNs.Load(); lu > 0 {
		resp.LastUpdated = time.Unix(0, lu).UTC().Format(time.RFC3339Nano)
	}
	resp.LastError = sub.GetLastError()
	return resp
}

func (s *ControlPlaneService) classifySubscriptionUnknownNodeCounts(
	h node.Hash,
	entry *node.NodeEntry,
) subscriptionUnknownNodeCounts {
	if entry == nil {
		return subscriptionUnknownNodeCounts{}
	}
	switch s.resolveNodeProfileState(h, entry) {
	case "UNPROBED", "PENDING_EGRESS", "PROBING_EGRESS":
		return subscriptionUnknownNodeCounts{pendingEgress: 1}
	case "PENDING_PROFILE", "QUEUED_PROFILE", "PROFILING":
		return subscriptionUnknownNodeCounts{pendingProfile: 1}
	case "PROFILED_UNKNOWN":
		return subscriptionUnknownNodeCounts{profiledUnknown: 1}
	default:
		return subscriptionUnknownNodeCounts{}
	}
}

func shouldAutoQueueUnknownProfile(entry *node.NodeEntry) bool {
	if entry == nil {
		return false
	}
	switch entry.GetEgressProfileSource() {
	case model.EgressProfileSourceOnline, model.EgressProfileSourceLocalPlusOnline:
		return false
	default:
		return true
	}
}

func (s *ControlPlaneService) queueUnknownNodeRepair(
	result *UnknownNodesFillResult,
	h node.Hash,
	entry *node.NodeEntry,
	manual bool,
	endpointCache map[string]endpointSeedResolution,
	egressQueue *[]node.Hash,
	profileQueue *[]node.Hash,
) {
	if result == nil || entry == nil {
		return
	}

	switch s.resolveNodeProfileState(h, entry) {
	case "UNPROBED", "PENDING_EGRESS", "PROBING_EGRESS":
		result.Matched++
		if s.trySeedUnknownNodeEgressFromEndpoint(h, entry, endpointCache) {
			result.SeededEgress++
			*profileQueue = append(*profileQueue, h)
			return
		}
		*egressQueue = append(*egressQueue, h)
	case "PENDING_PROFILE", "QUEUED_PROFILE", "PROFILING":
		result.Matched++
		*profileQueue = append(*profileQueue, h)
	case "PROFILED_UNKNOWN":
		result.Matched++
		if manual || shouldAutoQueueUnknownProfile(entry) {
			*profileQueue = append(*profileQueue, h)
			return
		}
		result.Skipped++
	}
}

func (s *ControlPlaneService) flushUnknownNodeRepairQueues(
	result *UnknownNodesFillResult,
	manual bool,
	egressQueue []node.Hash,
	profileQueue []node.Hash,
) {
	if result == nil {
		return
	}

	for _, h := range egressQueue {
		if s == nil || s.ProbeMgr == nil {
			result.Failed++
			continue
		}
		s.ProbeMgr.TriggerImmediateEgressProbe(h)
		result.QueuedEgress++
	}

	for _, h := range profileQueue {
		if s == nil || s.ProfileSvc == nil {
			result.Failed++
			continue
		}
		if manual {
			s.ProfileSvc.EnqueueForce(h)
		} else {
			s.ProfileSvc.Enqueue(h)
		}
		result.QueuedProfile++
	}
}

// ListSubscriptions returns all subscriptions, optionally filtered by enabled.
func (s *ControlPlaneService) ListSubscriptions(enabled *bool) ([]SubscriptionResponse, error) {
	var result []SubscriptionResponse
	s.SubMgr.Range(func(id string, sub *subscription.Subscription) bool {
		if enabled != nil && sub.Enabled() != *enabled {
			return true
		}
		result = append(result, s.subToResponse(sub))
		return true
	})
	if result == nil {
		result = []SubscriptionResponse{}
	}
	return result, nil
}

// GetSubscription returns a single subscription by ID.
func (s *ControlPlaneService) GetSubscription(id string) (*SubscriptionResponse, error) {
	sub := s.SubMgr.Lookup(id)
	if sub == nil {
		return nil, notFound("subscription not found")
	}
	r := s.subToResponse(sub)
	return &r, nil
}

// CreateSubscriptionRequest holds create subscription parameters.
type CreateSubscriptionRequest struct {
	Name                    *string                   `json:"name"`
	SourceType              *string                   `json:"source_type"`
	URL                     *string                   `json:"url"`
	Content                 *string                   `json:"content"`
	Sources                 []SubscriptionSourceInput `json:"sources"`
	UpdateInterval          *string                   `json:"update_interval"`
	Enabled                 *bool                     `json:"enabled"`
	Ephemeral               *bool                     `json:"ephemeral"`
	EphemeralNodeEvictDelay *string                   `json:"ephemeral_node_evict_delay"`
}

const minSubscriptionUpdateInterval = 30 * time.Second
const defaultSubscriptionEphemeralNodeEvictDelay = 72 * time.Hour

func parseSubscriptionSourceType(raw *string) (string, *ServiceError) {
	if raw == nil {
		return subscription.SourceTypeRemote, nil
	}
	value := strings.ToLower(strings.TrimSpace(*raw))
	switch value {
	case subscription.SourceTypeRemote, subscription.SourceTypeLocal:
		return value, nil
	default:
		return "", invalidArg("source_type: must be remote or local")
	}
}

func toSubscriptionSourceResponses(sources []model.SubscriptionSource) []SubscriptionSourceResponse {
	if len(sources) == 0 {
		return []SubscriptionSourceResponse{}
	}
	items := make([]SubscriptionSourceResponse, 0, len(sources))
	for _, source := range sources {
		items = append(items, SubscriptionSourceResponse{
			ID:      source.ID,
			Label:   source.Label,
			Type:    source.Type,
			URL:     source.URL,
			Content: source.Content,
			Enabled: source.Enabled,
		})
	}
	return items
}

func normalizeSubscriptionSourcesInput(sources []SubscriptionSourceInput) ([]model.SubscriptionSource, *ServiceError) {
	if len(sources) == 0 {
		return nil, invalidArg("sources: must contain at least one source")
	}
	normalized := make([]model.SubscriptionSource, 0, len(sources))
	for index, source := range sources {
		sourceType := strings.ToLower(strings.TrimSpace(source.Type))
		switch sourceType {
		case subscription.SourceTypeRemote:
			url := strings.TrimSpace(source.URL)
			if url == "" {
				return nil, invalidArg(fmt.Sprintf("sources[%d].url: required for remote source", index))
			}
			if _, verr := parseHTTPAbsoluteURL(fmt.Sprintf("sources[%d].url", index), url); verr != nil {
				return nil, verr
			}
			if strings.TrimSpace(source.Content) != "" {
				return nil, invalidArg(fmt.Sprintf("sources[%d].content: not allowed for remote source", index))
			}
		case subscription.SourceTypeLocal:
			if strings.TrimSpace(source.Content) == "" {
				return nil, invalidArg(fmt.Sprintf("sources[%d].content: required for local source", index))
			}
			if strings.TrimSpace(source.URL) != "" {
				return nil, invalidArg(fmt.Sprintf("sources[%d].url: not allowed for local source", index))
			}
		default:
			return nil, invalidArg(fmt.Sprintf("sources[%d].type: must be remote or local", index))
		}
		enabled := true
		if source.Enabled != nil {
			enabled = *source.Enabled
		}
		sourceID := strings.TrimSpace(source.ID)
		if sourceID == "" {
			sourceID = fmt.Sprintf("source-%d", index+1)
		}
		normalized = append(normalized, model.SubscriptionSource{
			ID:      sourceID,
			Label:   strings.TrimSpace(source.Label),
			Type:    sourceType,
			URL:     strings.TrimSpace(source.URL),
			Content: source.Content,
			Enabled: enabled,
		})
	}
	enabledCount := 0
	for _, source := range normalized {
		if source.Enabled {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return nil, invalidArg("sources: at least one source must be enabled")
	}
	return normalized, nil
}

func buildLegacySubscriptionSource(
	sourceType string,
	url string,
	content string,
) []model.SubscriptionSource {
	return []model.SubscriptionSource{{
		ID:      "source-1",
		Type:    sourceType,
		URL:     url,
		Content: content,
		Enabled: true,
	}}
}

func primarySourceFields(sources []model.SubscriptionSource) (string, string, string) {
	if len(sources) == 0 {
		return subscription.SourceTypeRemote, "", ""
	}
	return sources[0].Type, sources[0].URL, sources[0].Content
}

// CreateSubscription creates a new subscription.
func (s *ControlPlaneService) CreateSubscription(req CreateSubscriptionRequest) (*SubscriptionResponse, error) {
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		return nil, invalidArg("name is required")
	}
	name := strings.TrimSpace(*req.Name)

	sourceType, verr := parseSubscriptionSourceType(req.SourceType)
	if verr != nil {
		return nil, verr
	}

	subURL := ""
	content := ""
	var sources []model.SubscriptionSource
	if len(req.Sources) > 0 {
		var sourceErr *ServiceError
		sources, sourceErr = normalizeSubscriptionSourcesInput(req.Sources)
		if sourceErr != nil {
			return nil, sourceErr
		}
		sourceType, subURL, content = primarySourceFields(sources)
	} else {
		switch sourceType {
		case subscription.SourceTypeRemote:
			if req.URL == nil || strings.TrimSpace(*req.URL) == "" {
				return nil, invalidArg("url is required for remote subscription")
			}
			subURL = strings.TrimSpace(*req.URL)
			if _, verr := parseHTTPAbsoluteURL("url", subURL); verr != nil {
				return nil, verr
			}
			if req.Content != nil && strings.TrimSpace(*req.Content) != "" {
				return nil, invalidArg("content is not allowed for remote subscription")
			}
		case subscription.SourceTypeLocal:
			if req.Content == nil || strings.TrimSpace(*req.Content) == "" {
				return nil, invalidArg("content is required for local subscription")
			}
			content = *req.Content
			if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
				return nil, invalidArg("url is not allowed for local subscription")
			}
		default:
			return nil, invalidArg("source_type: must be remote or local")
		}
		sources = buildLegacySubscriptionSource(sourceType, subURL, content)
	}

	updateInterval := 5 * time.Minute
	if req.UpdateInterval != nil {
		d, err := time.ParseDuration(*req.UpdateInterval)
		if err != nil {
			return nil, invalidArg("update_interval: " + err.Error())
		}
		if d < minSubscriptionUpdateInterval {
			return nil, invalidArg("update_interval: must be >= 30s")
		}
		updateInterval = d
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	ephemeral := false
	if req.Ephemeral != nil {
		ephemeral = *req.Ephemeral
	}
	ephemeralNodeEvictDelay := defaultSubscriptionEphemeralNodeEvictDelay
	if req.EphemeralNodeEvictDelay != nil {
		d, err := time.ParseDuration(*req.EphemeralNodeEvictDelay)
		if err != nil {
			return nil, invalidArg("ephemeral_node_evict_delay: " + err.Error())
		}
		if d < 0 {
			return nil, invalidArg("ephemeral_node_evict_delay: must be non-negative")
		}
		ephemeralNodeEvictDelay = d
	}

	id := uuid.New().String()
	now := time.Now().UnixNano()

	ms := model.Subscription{
		ID:                        id,
		Name:                      name,
		SourceType:                sourceType,
		URL:                       subURL,
		Content:                   content,
		Sources:                   sources,
		UpdateIntervalNs:          int64(updateInterval),
		Enabled:                   enabled,
		Ephemeral:                 ephemeral,
		EphemeralNodeEvictDelayNs: int64(ephemeralNodeEvictDelay),
		CreatedAtNs:               now,
		UpdatedAtNs:               now,
	}
	if err := s.Engine.UpsertSubscription(ms); err != nil {
		return nil, internal("persist subscription", err)
	}

	sub := subscription.NewSubscription(id, name, subURL, enabled, ephemeral)
	sub.SetFetchConfig(subURL, int64(updateInterval))
	sub.SetSourceType(sourceType)
	sub.SetContent(content)
	sub.SetSources(sources)
	sub.SetEphemeralNodeEvictDelayNs(int64(ephemeralNodeEvictDelay))
	sub.CreatedAtNs = now
	sub.UpdatedAtNs = now
	s.SubMgr.Register(sub)

	r := s.subToResponse(sub)
	return &r, nil
}

// UpdateSubscription applies a constrained partial patch to a subscription.
// This is not RFC 7396 JSON Merge Patch: patch must be a non-empty object and
// null values are rejected.
func (s *ControlPlaneService) UpdateSubscription(id string, patchJSON json.RawMessage) (*SubscriptionResponse, error) {
	patch, verr := parseMergePatch(patchJSON)
	if verr != nil {
		return nil, verr
	}
	if err := patch.validateFields(subscriptionPatchAllowedFields, func(key string) string {
		return fmt.Sprintf("field %q is read-only or unknown", key)
	}); err != nil {
		return nil, err
	}

	sub := s.SubMgr.Lookup(id)
	if sub == nil {
		return nil, notFound("subscription not found")
	}

	// Track what changed for side-effects.
	nameChanged := false
	enabledChanged := false
	sourcesChanged := false
	sourceType := sub.SourceType()

	newName := sub.Name()
	if nameStr, ok, err := patch.optionalNonEmptyString("name"); err != nil {
		return nil, err
	} else if ok {
		newName = nameStr
		if newName != sub.Name() {
			nameChanged = true
		}
	}

	newSources := sub.Sources()
	if rawSources, ok, err := patch.optionalArray("sources"); err != nil {
		return nil, err
	} else if ok {
		if _, conflictURL := patch["url"]; conflictURL {
			return nil, invalidArg("sources: cannot be patched together with url")
		}
		if _, conflictContent := patch["content"]; conflictContent {
			return nil, invalidArg("sources: cannot be patched together with content")
		}
		normalized, sourceErr := normalizeSubscriptionSourcesPatch(rawSources)
		if sourceErr != nil {
			return nil, sourceErr
		}
		newSources = normalized
		sourcesChanged = true
	} else {
		newURL := sub.URL()
		if urlStr, ok, err := patch.optionalString("url"); err != nil {
			return nil, err
		} else if ok {
			if sourceType != subscription.SourceTypeRemote {
				return nil, invalidArg("url: field is not allowed for local subscription")
			}
			if _, verr := parseHTTPAbsoluteURL("url", urlStr); verr != nil {
				return nil, verr
			}
			newURL = urlStr
		}

		newContent := sub.Content()
		if contentStr, ok, err := patch.optionalString("content"); err != nil {
			return nil, err
		} else if ok {
			if sourceType != subscription.SourceTypeLocal {
				return nil, invalidArg("content: field is not allowed for remote subscription")
			}
			if strings.TrimSpace(contentStr) == "" {
				return nil, invalidArg("content: must be a non-empty string")
			}
			newContent = contentStr
		}

		legacySources := buildLegacySubscriptionSource(sourceType, newURL, newContent)
		if !subscriptionSourcesEqual(newSources, legacySources) {
			newSources = legacySources
			sourcesChanged = true
		}
	}
	sourceType, newURL, newContent := primarySourceFields(newSources)

	newInterval := sub.UpdateIntervalNs()
	if d, ok, err := patch.optionalDurationString("update_interval"); err != nil {
		return nil, err
	} else if ok {
		if d < minSubscriptionUpdateInterval {
			return nil, invalidArg("update_interval: must be >= 30s")
		}
		newInterval = int64(d)
	}

	newEnabled := sub.Enabled()
	if b, ok, err := patch.optionalBool("enabled"); err != nil {
		return nil, err
	} else if ok {
		if b != newEnabled {
			enabledChanged = true
		}
		newEnabled = b
	}

	newEphemeral := sub.Ephemeral()
	if b, ok, err := patch.optionalBool("ephemeral"); err != nil {
		return nil, err
	} else if ok {
		newEphemeral = b
	}

	newEphemeralNodeEvictDelay := sub.EphemeralNodeEvictDelayNs()
	if d, ok, err := patch.optionalDurationString("ephemeral_node_evict_delay"); err != nil {
		return nil, err
	} else if ok {
		if d < 0 {
			return nil, invalidArg("ephemeral_node_evict_delay: must be non-negative")
		}
		newEphemeralNodeEvictDelay = int64(d)
	}

	now := time.Now().UnixNano()
	ms := model.Subscription{
		ID:                        id,
		Name:                      newName,
		SourceType:                sourceType,
		URL:                       newURL,
		Content:                   newContent,
		Sources:                   newSources,
		UpdateIntervalNs:          newInterval,
		Enabled:                   newEnabled,
		Ephemeral:                 newEphemeral,
		EphemeralNodeEvictDelayNs: newEphemeralNodeEvictDelay,
		CreatedAtNs:               sub.CreatedAtNs,
		UpdatedAtNs:               now,
	}
	if err := s.Engine.UpsertSubscription(ms); err != nil {
		return nil, internal("persist subscription", err)
	}

	// Apply side-effects via scheduler.
	sub.SetFetchConfig(newURL, newInterval)
	sub.SetContent(newContent)
	sub.SetSourceType(sourceType)
	sub.SetSources(newSources)
	sub.SetEphemeral(newEphemeral)
	sub.SetEphemeralNodeEvictDelayNs(newEphemeralNodeEvictDelay)
	sub.UpdatedAtNs = now

	if nameChanged {
		s.Scheduler.RenameSubscription(sub, newName)
	}
	if enabledChanged {
		s.Scheduler.SetSubscriptionEnabled(sub, newEnabled)
	}
	if sourcesChanged {
		go s.Scheduler.UpdateSubscription(sub)
	}

	r := s.subToResponse(sub)
	return &r, nil
}

func normalizeSubscriptionSourcesPatch(raw []any) ([]model.SubscriptionSource, *ServiceError) {
	inputs := make([]SubscriptionSourceInput, 0, len(raw))
	for index, item := range raw {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return nil, invalidArg(fmt.Sprintf("sources[%d]: must be an object", index))
		}
		input := SubscriptionSourceInput{}
		if value, ok := itemMap["id"]; ok {
			id, ok := value.(string)
			if !ok {
				return nil, invalidArg(fmt.Sprintf("sources[%d].id: must be a string", index))
			}
			input.ID = id
		}
		if value, ok := itemMap["label"]; ok {
			label, ok := value.(string)
			if !ok {
				return nil, invalidArg(fmt.Sprintf("sources[%d].label: must be a string", index))
			}
			input.Label = label
		}
		if value, ok := itemMap["type"]; ok {
			sourceType, ok := value.(string)
			if !ok {
				return nil, invalidArg(fmt.Sprintf("sources[%d].type: must be a string", index))
			}
			input.Type = sourceType
		}
		if value, ok := itemMap["url"]; ok {
			url, ok := value.(string)
			if !ok {
				return nil, invalidArg(fmt.Sprintf("sources[%d].url: must be a string", index))
			}
			input.URL = url
		}
		if value, ok := itemMap["content"]; ok {
			content, ok := value.(string)
			if !ok {
				return nil, invalidArg(fmt.Sprintf("sources[%d].content: must be a string", index))
			}
			input.Content = content
		}
		if value, ok := itemMap["enabled"]; ok {
			enabled, ok := value.(bool)
			if !ok {
				return nil, invalidArg(fmt.Sprintf("sources[%d].enabled: must be a boolean", index))
			}
			input.Enabled = &enabled
		}
		inputs = append(inputs, input)
	}
	return normalizeSubscriptionSourcesInput(inputs)
}

func subscriptionSourcesEqual(left, right []model.SubscriptionSource) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// DeleteSubscription deletes a subscription and evicts its nodes.
func (s *ControlPlaneService) DeleteSubscription(id string) error {
	sub := s.SubMgr.Lookup(id)
	if sub == nil {
		return notFound("subscription not found")
	}

	var (
		managedHashes []node.Hash
		deleteErr     error
	)

	// Keep delete atomic across persistence + in-memory runtime state:
	// if DB delete fails, do not mutate runtime subscription/node state.
	sub.WithOpLock(func() {
		// Re-check under lock in case another goroutine removed it between
		// the initial Lookup and lock acquisition.
		lockedSub := s.SubMgr.Lookup(id)
		if lockedSub == nil {
			deleteErr = notFound("subscription not found")
			return
		}

		lockedSub.ManagedNodes().RangeNodes(func(h node.Hash, _ subscription.ManagedNode) bool {
			managedHashes = append(managedHashes, h)
			return true
		})

		if err := s.Engine.DeleteSubscription(id); err != nil {
			if errors.Is(err, state.ErrNotFound) {
				deleteErr = notFound("subscription not found")
			} else {
				deleteErr = internal("delete subscription", err)
			}
			return
		}

		// Persist succeeded; now apply in-memory cleanup.
		for _, h := range managedHashes {
			s.Pool.RemoveNodeFromSub(h, id)
		}
		s.SubMgr.Unregister(id)
	})

	return deleteErr
}

// RefreshSubscription triggers an immediate subscription refresh (blocks).
func (s *ControlPlaneService) RefreshSubscription(id string) error {
	sub := s.SubMgr.Lookup(id)
	if sub == nil {
		return notFound("subscription not found")
	}
	s.Scheduler.UpdateSubscription(sub)
	return nil
}

func (s *ControlPlaneService) AutoFillSubscriptionUnknownNodes(id string) error {
	_, err := s.fillSubscriptionUnknownNodes(id, false)
	return err
}

func (s *ControlPlaneService) FillSubscriptionUnknownNodes(id string) (*SubscriptionFillUnknownNodesResult, error) {
	return s.fillSubscriptionUnknownNodes(id, true)
}

func (s *ControlPlaneService) fillSubscriptionUnknownNodes(
	id string,
	manual bool,
) (*SubscriptionFillUnknownNodesResult, error) {
	if s == nil || s.SubMgr == nil || s.Pool == nil {
		return nil, internal("subscription unknown-node repair unavailable", nil)
	}

	sub := s.SubMgr.Lookup(id)
	if sub == nil {
		return nil, notFound("subscription not found")
	}

	result := &SubscriptionFillUnknownNodesResult{}
	egressQueue := make([]node.Hash, 0, 32)
	profileQueue := make([]node.Hash, 0, 32)
	endpointCache := make(map[string]endpointSeedResolution)
	var fillErr error

	sub.WithOpLock(func() {
		lockedSub := s.SubMgr.Lookup(id)
		if lockedSub == nil {
			fillErr = notFound("subscription not found")
			return
		}

		managed := lockedSub.ManagedNodes()
		if managed == nil {
			return
		}

		managed.RangeNodes(func(h node.Hash, managedNode subscription.ManagedNode) bool {
			if managedNode.Evicted {
				result.Matched++
				result.Skipped++
				return true
			}

			entry, ok := s.Pool.GetEntry(h)
			if !ok || entry == nil {
				result.Matched++
				result.Skipped++
				return true
			}
			if !entry.HasOutbound() {
				result.Matched++
				result.Skipped++
				return true
			}
			s.queueUnknownNodeRepair(result, h, entry, manual, endpointCache, &egressQueue, &profileQueue)
			return true
		})
	})
	if fillErr != nil {
		return nil, fillErr
	}
	s.flushUnknownNodeRepairQueues(result, manual, egressQueue, profileQueue)

	return result, nil
}

// CleanupSubscriptionCircuitOpenNodes removes problematic nodes from a subscription.
// It marks nodes as evicted (while keeping managed hashes) for nodes currently
// circuit-open, and nodes with no outbound while carrying a non-empty last error.
func (s *ControlPlaneService) CleanupSubscriptionCircuitOpenNodes(id string) (int, error) {
	return s.cleanupSubscriptionCircuitOpenNodesWithHook(id, nil)
}

// cleanupSubscriptionCircuitOpenNodesWithHook performs cleanup with an optional
// hook between first scan and second confirmation scan. The hook is only used
// by tests to simulate TOCTOU recovery.
func (s *ControlPlaneService) cleanupSubscriptionCircuitOpenNodesWithHook(
	id string,
	betweenScans func(),
) (int, error) {
	sub := s.SubMgr.Lookup(id)
	if sub == nil {
		return 0, notFound("subscription not found")
	}

	var (
		cleanedCount int
		evicted      []node.Hash
		cleanupErr   error
	)

	sub.WithOpLock(func() {
		// Re-check under lock in case another goroutine deleted the subscription
		// between lookup and lock acquisition.
		lockedSub := s.SubMgr.Lookup(id)
		if lockedSub == nil {
			cleanupErr = notFound("subscription not found")
			return
		}

		cleanedCount, evicted = topology.CleanupSubscriptionNodesWithConfirmNoLock(
			lockedSub,
			s.Pool,
			shouldCleanupSubscriptionNode,
			betweenScans,
		)
	})
	if cleanupErr != nil {
		return 0, cleanupErr
	}

	if s.Engine != nil {
		for _, h := range evicted {
			s.Engine.MarkSubscriptionNode(id, h.Hex())
		}
	}

	return cleanedCount, nil
}

func shouldCleanupSubscriptionNode(entry *node.NodeEntry) bool {
	if entry == nil {
		return false
	}
	return entry.IsCircuitOpen() || (!entry.HasOutbound() && entry.GetLastError() != "")
}
