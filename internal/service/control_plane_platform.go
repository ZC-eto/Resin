package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Resinat/Resin/internal/model"
	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/platform"
	"github.com/Resinat/Resin/internal/state"
)

// ------------------------------------------------------------------
// Platform
// ------------------------------------------------------------------

// PlatformResponse is the API response model for a platform.
type PlatformResponse struct {
	ID                               string   `json:"id"`
	Name                             string   `json:"name"`
	StickyTTL                        string   `json:"sticky_ttl"`
	ProxyAccessMode                  string   `json:"proxy_access_mode"`
	RotationPolicy                   string   `json:"rotation_policy"`
	RotationInterval                 string   `json:"rotation_interval"`
	RegexFilters                     []string `json:"regex_filters"`
	RegionFilters                    []string `json:"region_filters"`
	SubscriptionFilters              []string `json:"subscription_filters"`
	NetworkTypeFilters               []string `json:"network_type_filters"`
	MinQualityScore                  *int     `json:"min_quality_score,omitempty"`
	MaxReferenceLatencyMs            *int     `json:"max_reference_latency_ms,omitempty"`
	MinEgressStabilityScore          *int     `json:"min_egress_stability_score,omitempty"`
	MaxCircuitOpenCount              *int     `json:"max_circuit_open_count,omitempty"`
	RoutableNodeCount                int      `json:"routable_node_count"`
	ReverseProxyMissAction           string   `json:"reverse_proxy_miss_action"`
	ReverseProxyEmptyAccountBehavior string   `json:"reverse_proxy_empty_account_behavior"`
	ReverseProxyFixedAccountHeader   string   `json:"reverse_proxy_fixed_account_header"`
	AllocationPolicy                 string   `json:"allocation_policy"`
	UpdatedAt                        string   `json:"updated_at"`
}

func platformToResponse(p model.Platform) PlatformResponse {
	behavior := normalizePlatformEmptyAccountBehavior(p.ReverseProxyEmptyAccountBehavior)
	fixedHeader := normalizeHeaderFieldName(p.ReverseProxyFixedAccountHeader)
	accessMode := platform.NormalizeProxyAccessMode(p.ProxyAccessMode)
	if !accessMode.IsValid() {
		accessMode = platform.ProxyAccessModeStandard
	}
	rotationPolicy := platform.EffectiveRotationPolicy(p.RotationPolicy, p.RotationIntervalNs, p.StickyTTLNs)
	rotationInterval := platform.EffectiveRotationIntervalNs(p.RotationIntervalNs, p.StickyTTLNs)
	stickyTTL := ""
	if rotationPolicy == platform.RotationPolicyTTL && rotationInterval > 0 {
		stickyTTL = time.Duration(rotationInterval).String()
	}
	return PlatformResponse{
		ID:                               p.ID,
		Name:                             p.Name,
		StickyTTL:                        stickyTTL,
		ProxyAccessMode:                  string(accessMode),
		RotationPolicy:                   string(rotationPolicy),
		RotationInterval:                 stickyTTL,
		RegexFilters:                     append([]string(nil), p.RegexFilters...),
		RegionFilters:                    append([]string(nil), p.RegionFilters...),
		SubscriptionFilters:              append([]string(nil), p.SubscriptionFilters...),
		NetworkTypeFilters:               append([]string(nil), p.NetworkTypeFilters...),
		MinQualityScore:                  p.MinQualityScore,
		MaxReferenceLatencyMs:            p.MaxReferenceLatencyMs,
		MinEgressStabilityScore:          p.MinEgressStabilityScore,
		MaxCircuitOpenCount:              p.MaxCircuitOpenCount,
		RoutableNodeCount:                0,
		ReverseProxyMissAction:           p.ReverseProxyMissAction,
		ReverseProxyEmptyAccountBehavior: behavior,
		ReverseProxyFixedAccountHeader:   fixedHeader,
		AllocationPolicy:                 p.AllocationPolicy,
		UpdatedAt:                        time.Unix(0, p.UpdatedAtNs).UTC().Format(time.RFC3339Nano),
	}
}

func (s *ControlPlaneService) withRoutableNodeCount(resp PlatformResponse) PlatformResponse {
	if s == nil || s.Pool == nil {
		return resp
	}
	plat, ok := s.Pool.GetPlatform(resp.ID)
	if !ok || plat == nil {
		return resp
	}
	resp.RoutableNodeCount = plat.View().Size()
	return resp
}

func (s *ControlPlaneService) currentLatencyAuthorities() []string {
	if s == nil || s.RuntimeCfg == nil {
		return nil
	}
	cfg := s.RuntimeCfg.Load()
	if cfg == nil {
		return nil
	}
	return cfg.LatencyAuthorities
}

type platformConfig struct {
	Name                             string
	StickyTTLNs                      int64
	ProxyAccessMode                  string
	RotationPolicy                   string
	RotationIntervalNs               int64
	RegexFilters                     []string
	RegionFilters                    []string
	SubscriptionFilters              []string
	NetworkTypeFilters               []string
	MinQualityScore                  *int
	MaxReferenceLatencyMs            *int
	MinEgressStabilityScore          *int
	MaxCircuitOpenCount              *int
	ReverseProxyMissAction           string
	ReverseProxyEmptyAccountBehavior string
	ReverseProxyFixedAccountHeader   string
	AllocationPolicy                 string
}

func normalizePlatformMissAction(raw string) string {
	normalized := platform.NormalizeReverseProxyMissAction(raw)
	if normalized == "" {
		return ""
	}
	return string(normalized)
}

func normalizePlatformEmptyAccountBehavior(raw string) string {
	if platform.ReverseProxyEmptyAccountBehavior(raw).IsValid() {
		return raw
	}
	return string(platform.ReverseProxyEmptyAccountBehaviorRandom)
}

func normalizeNetworkTypeFilters(values []string) []model.EgressNetworkType {
	result := make([]model.EgressNetworkType, 0, len(values))
	for _, value := range values {
		result = append(result, model.NormalizeEgressNetworkType(strings.ToUpper(strings.TrimSpace(value))))
	}
	return result
}

func validateNonNegativeOptionalInt(field string, value *int) *ServiceError {
	if value == nil {
		return nil
	}
	if *value < 0 {
		return invalidArg(field + ": must be >= 0")
	}
	return nil
}

func (s *ControlPlaneService) defaultPlatformConfig(name string) platformConfig {
	return platformConfig{
		Name:                   name,
		StickyTTLNs:            int64(s.EnvCfg.DefaultPlatformStickyTTL),
		ProxyAccessMode:        string(platform.ProxyAccessModeStandard),
		RotationPolicy:         string(platform.RotationPolicyTTL),
		RotationIntervalNs:     int64(s.EnvCfg.DefaultPlatformStickyTTL),
		RegexFilters:           append([]string(nil), s.EnvCfg.DefaultPlatformRegexFilters...),
		RegionFilters:          append([]string(nil), s.EnvCfg.DefaultPlatformRegionFilters...),
		SubscriptionFilters:    []string{},
		NetworkTypeFilters:     []string{},
		ReverseProxyMissAction: s.EnvCfg.DefaultPlatformReverseProxyMissAction,
		ReverseProxyEmptyAccountBehavior: normalizePlatformEmptyAccountBehavior(
			s.EnvCfg.DefaultPlatformReverseProxyEmptyAccountBehavior,
		),
		ReverseProxyFixedAccountHeader: normalizeHeaderFieldName(
			s.EnvCfg.DefaultPlatformReverseProxyFixedAccountHeader,
		),
		AllocationPolicy: s.EnvCfg.DefaultPlatformAllocationPolicy,
	}
}

func platformConfigFromModel(mp model.Platform) platformConfig {
	accessMode := platform.NormalizeProxyAccessMode(mp.ProxyAccessMode)
	if !accessMode.IsValid() {
		accessMode = platform.ProxyAccessModeStandard
	}
	return platformConfig{
		Name:                             mp.Name,
		StickyTTLNs:                      mp.StickyTTLNs,
		ProxyAccessMode:                  string(accessMode),
		RotationPolicy:                   string(platform.EffectiveRotationPolicy(mp.RotationPolicy, mp.RotationIntervalNs, mp.StickyTTLNs)),
		RotationIntervalNs:               platform.EffectiveRotationIntervalNs(mp.RotationIntervalNs, mp.StickyTTLNs),
		RegexFilters:                     append([]string(nil), mp.RegexFilters...),
		RegionFilters:                    append([]string(nil), mp.RegionFilters...),
		SubscriptionFilters:              append([]string(nil), mp.SubscriptionFilters...),
		NetworkTypeFilters:               append([]string(nil), mp.NetworkTypeFilters...),
		MinQualityScore:                  mp.MinQualityScore,
		MaxReferenceLatencyMs:            mp.MaxReferenceLatencyMs,
		MinEgressStabilityScore:          mp.MinEgressStabilityScore,
		MaxCircuitOpenCount:              mp.MaxCircuitOpenCount,
		ReverseProxyMissAction:           mp.ReverseProxyMissAction,
		ReverseProxyEmptyAccountBehavior: normalizePlatformEmptyAccountBehavior(mp.ReverseProxyEmptyAccountBehavior),
		ReverseProxyFixedAccountHeader:   normalizeHeaderFieldName(mp.ReverseProxyFixedAccountHeader),
		AllocationPolicy:                 mp.AllocationPolicy,
	}
}

func (cfg platformConfig) toModel(id string, updatedAtNs int64) model.Platform {
	return model.Platform{
		ID:                               id,
		Name:                             cfg.Name,
		StickyTTLNs:                      cfg.RotationIntervalNs,
		ProxyAccessMode:                  string(platform.NormalizeProxyAccessMode(cfg.ProxyAccessMode)),
		RotationPolicy:                   string(platform.EffectiveRotationPolicy(cfg.RotationPolicy, cfg.RotationIntervalNs, cfg.StickyTTLNs)),
		RotationIntervalNs:               cfg.RotationIntervalNs,
		RegexFilters:                     append([]string(nil), cfg.RegexFilters...),
		RegionFilters:                    append([]string(nil), cfg.RegionFilters...),
		SubscriptionFilters:              append([]string(nil), cfg.SubscriptionFilters...),
		NetworkTypeFilters:               append([]string(nil), cfg.NetworkTypeFilters...),
		MinQualityScore:                  cfg.MinQualityScore,
		MaxReferenceLatencyMs:            cfg.MaxReferenceLatencyMs,
		MinEgressStabilityScore:          cfg.MinEgressStabilityScore,
		MaxCircuitOpenCount:              cfg.MaxCircuitOpenCount,
		ReverseProxyMissAction:           cfg.ReverseProxyMissAction,
		ReverseProxyEmptyAccountBehavior: cfg.ReverseProxyEmptyAccountBehavior,
		ReverseProxyFixedAccountHeader:   cfg.ReverseProxyFixedAccountHeader,
		AllocationPolicy:                 cfg.AllocationPolicy,
		UpdatedAtNs:                      updatedAtNs,
	}
}

func (cfg platformConfig) toRuntime(id string) (*platform.Platform, error) {
	compiledRegexFilters, err := platform.CompileRegexFilters(cfg.RegexFilters)
	if err != nil {
		return nil, err
	}
	plat := platform.NewConfiguredPlatform(
		id,
		cfg.Name,
		compiledRegexFilters,
		cfg.RegionFilters,
		cfg.StickyTTLNs,
		cfg.RotationPolicy,
		cfg.RotationIntervalNs,
		cfg.ReverseProxyMissAction,
		cfg.ReverseProxyEmptyAccountBehavior,
		cfg.ReverseProxyFixedAccountHeader,
		cfg.AllocationPolicy,
	)
	plat.SubscriptionFilters = append([]string(nil), cfg.SubscriptionFilters...)
	plat.NetworkTypeFilters = normalizeNetworkTypeFilters(cfg.NetworkTypeFilters)
	plat.MinQualityScore = cfg.MinQualityScore
	plat.MaxReferenceLatencyMs = cfg.MaxReferenceLatencyMs
	plat.MinEgressStabilityScore = cfg.MinEgressStabilityScore
	plat.MaxCircuitOpenCount = cfg.MaxCircuitOpenCount
	return plat, nil
}

func validatePlatformMissAction(raw string) *ServiceError {
	if normalizePlatformMissAction(raw) != "" {
		return nil
	}
	return invalidArg(fmt.Sprintf(
		"reverse_proxy_miss_action: must be %s or %s",
		platform.ReverseProxyMissActionTreatAsEmpty,
		platform.ReverseProxyMissActionReject,
	))
}

func validatePlatformProxyAccessMode(raw string) *ServiceError {
	if platform.NormalizeProxyAccessMode(raw).IsValid() {
		return nil
	}
	return invalidArg(fmt.Sprintf(
		"proxy_access_mode: must be %s or %s",
		platform.ProxyAccessModeStandard,
		platform.ProxyAccessModeSticky,
	))
}

func validatePlatformEmptyAccountBehavior(raw string) *ServiceError {
	if platform.ReverseProxyEmptyAccountBehavior(raw).IsValid() {
		return nil
	}
	return invalidArg(fmt.Sprintf(
		"reverse_proxy_empty_account_behavior: must be %s, %s, or %s",
		platform.ReverseProxyEmptyAccountBehaviorRandom,
		platform.ReverseProxyEmptyAccountBehaviorFixedHeader,
		platform.ReverseProxyEmptyAccountBehaviorAccountHeaderRule,
	))
}

func normalizeHeaderFieldName(raw string) string {
	normalized, _, err := platform.NormalizeFixedAccountHeaders(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return normalized
}

func validatePlatformEmptyAccountConfig(cfg *platformConfig) *ServiceError {
	if cfg == nil {
		return invalidArg("platform config is required")
	}
	if err := validatePlatformEmptyAccountBehavior(cfg.ReverseProxyEmptyAccountBehavior); err != nil {
		return err
	}
	normalizedFixedHeaders, fixedHeaders, err := platform.NormalizeFixedAccountHeaders(cfg.ReverseProxyFixedAccountHeader)
	if err != nil {
		return invalidArg("reverse_proxy_fixed_account_header: " + err.Error())
	}
	cfg.ReverseProxyFixedAccountHeader = normalizedFixedHeaders
	if cfg.ReverseProxyEmptyAccountBehavior == string(platform.ReverseProxyEmptyAccountBehaviorFixedHeader) &&
		len(fixedHeaders) == 0 {
		return invalidArg(
			"reverse_proxy_fixed_account_header: required when reverse_proxy_empty_account_behavior is FIXED_HEADER",
		)
	}
	return nil
}

func validatePlatformAllocationPolicy(raw string) *ServiceError {
	if platform.AllocationPolicy(raw).IsValid() {
		return nil
	}
	return invalidArg(fmt.Sprintf(
		"allocation_policy: must be %s, %s, or %s",
		platform.AllocationPolicyBalanced,
		platform.AllocationPolicyPreferLowLatency,
		platform.AllocationPolicyPreferIdleIP,
	))
}

func setPlatformStickyTTL(cfg *platformConfig, d time.Duration) *ServiceError {
	if d <= 0 {
		return invalidArg("sticky_ttl: must be > 0")
	}
	cfg.StickyTTLNs = int64(d)
	cfg.RotationPolicy = string(platform.RotationPolicyTTL)
	cfg.RotationIntervalNs = int64(d)
	return nil
}

func setPlatformProxyAccessMode(cfg *platformConfig, accessMode string) *ServiceError {
	if err := validatePlatformProxyAccessMode(accessMode); err != nil {
		return err
	}
	cfg.ProxyAccessMode = string(platform.NormalizeProxyAccessMode(accessMode))
	return nil
}

func validatePlatformRotationPolicy(raw string) *ServiceError {
	if platform.NormalizeRotationPolicy(raw).IsValid() {
		return nil
	}
	return invalidArg(fmt.Sprintf(
		"rotation_policy: must be %s or %s",
		platform.RotationPolicyKeep,
		platform.RotationPolicyTTL,
	))
}

func setPlatformRotationPolicy(cfg *platformConfig, raw string) *ServiceError {
	if err := validatePlatformRotationPolicy(raw); err != nil {
		return err
	}
	cfg.RotationPolicy = string(platform.NormalizeRotationPolicy(raw))
	if cfg.RotationPolicy == string(platform.RotationPolicyKeep) {
		cfg.RotationIntervalNs = 0
		cfg.StickyTTLNs = 0
	}
	return nil
}

func setPlatformRotationInterval(cfg *platformConfig, d time.Duration) *ServiceError {
	if d == 0 && platform.NormalizeRotationPolicy(cfg.RotationPolicy) == platform.RotationPolicyKeep {
		cfg.RotationIntervalNs = 0
		cfg.StickyTTLNs = 0
		return nil
	}
	if d <= 0 {
		return invalidArg("rotation_interval: must be > 0")
	}
	cfg.RotationPolicy = string(platform.RotationPolicyTTL)
	cfg.RotationIntervalNs = int64(d)
	cfg.StickyTTLNs = int64(d)
	return nil
}

func setPlatformMissAction(cfg *platformConfig, missAction string) *ServiceError {
	if err := validatePlatformMissAction(missAction); err != nil {
		return err
	}
	cfg.ReverseProxyMissAction = normalizePlatformMissAction(missAction)
	return nil
}

func setPlatformEmptyAccountBehavior(cfg *platformConfig, behavior string) *ServiceError {
	if err := validatePlatformEmptyAccountBehavior(behavior); err != nil {
		return err
	}
	cfg.ReverseProxyEmptyAccountBehavior = behavior
	return nil
}

func setPlatformAllocationPolicy(cfg *platformConfig, policy string) *ServiceError {
	if err := validatePlatformAllocationPolicy(policy); err != nil {
		return err
	}
	cfg.AllocationPolicy = policy
	return nil
}

func validatePlatformConfig(cfg *platformConfig, validateRegionFilters bool) *ServiceError {
	if validateRegionFilters {
		if err := platform.ValidateRegionFilters(cfg.RegionFilters); err != nil {
			return invalidArg(err.Error())
		}
	}
	if err := platform.ValidateNetworkTypeFilters(cfg.NetworkTypeFilters); err != nil {
		return invalidArg(err.Error())
	}
	if err := validateNonNegativeOptionalInt("min_quality_score", cfg.MinQualityScore); err != nil {
		return err
	}
	if err := validateNonNegativeOptionalInt("max_reference_latency_ms", cfg.MaxReferenceLatencyMs); err != nil {
		return err
	}
	if err := validateNonNegativeOptionalInt("min_egress_stability_score", cfg.MinEgressStabilityScore); err != nil {
		return err
	}
	if err := validateNonNegativeOptionalInt("max_circuit_open_count", cfg.MaxCircuitOpenCount); err != nil {
		return err
	}
	if cfg.MinQualityScore != nil && *cfg.MinQualityScore == 0 {
		cfg.MinQualityScore = nil
	}
	if cfg.MaxReferenceLatencyMs != nil && *cfg.MaxReferenceLatencyMs == 0 {
		cfg.MaxReferenceLatencyMs = nil
	}
	if cfg.MinEgressStabilityScore != nil && *cfg.MinEgressStabilityScore == 0 {
		cfg.MinEgressStabilityScore = nil
	}
	if cfg.MaxCircuitOpenCount != nil && *cfg.MaxCircuitOpenCount == 0 {
		cfg.MaxCircuitOpenCount = nil
	}
	if err := validatePlatformEmptyAccountConfig(cfg); err != nil {
		return err
	}
	if err := validatePlatformProxyAccessMode(cfg.ProxyAccessMode); err != nil {
		return err
	}
	if err := validatePlatformRotationPolicy(cfg.RotationPolicy); err != nil {
		return err
	}
	if platform.NormalizeRotationPolicy(cfg.RotationPolicy) == platform.RotationPolicyTTL && cfg.RotationIntervalNs <= 0 {
		return invalidArg("rotation_interval: must be > 0 when rotation_policy is TTL")
	}
	if platform.NormalizeRotationPolicy(cfg.RotationPolicy) == platform.RotationPolicyKeep && cfg.RotationIntervalNs != 0 {
		return invalidArg("rotation_interval: must be empty when rotation_policy is KEEP")
	}
	return nil
}

func (s *ControlPlaneService) compileAndUpsertPlatform(id string, cfg platformConfig) (model.Platform, *platform.Platform, *ServiceError) {
	if err := platform.ValidatePlatformName(cfg.Name); err != nil {
		return model.Platform{}, nil, invalidArg("name: " + err.Error())
	}

	plat, err := cfg.toRuntime(id)
	if err != nil {
		return model.Platform{}, nil, invalidArg(err.Error())
	}
	mp := cfg.toModel(id, time.Now().UnixNano())
	if err := s.Engine.UpsertPlatform(mp); err != nil {
		if errors.Is(err, state.ErrConflict) {
			return model.Platform{}, nil, conflict("platform name already exists")
		}
		if strings.HasPrefix(err.Error(), "platform name: ") {
			return model.Platform{}, nil, invalidArg("name: " + strings.TrimPrefix(err.Error(), "platform name: "))
		}
		return model.Platform{}, nil, internal("persist platform", err)
	}
	return mp, plat, nil
}

// ListPlatforms returns all platforms from the database.
func (s *ControlPlaneService) ListPlatforms() ([]PlatformResponse, error) {
	platforms, err := s.Engine.ListPlatforms()
	if err != nil {
		return nil, internal("list platforms", err)
	}
	resp := make([]PlatformResponse, len(platforms))
	for i, p := range platforms {
		resp[i] = s.withRoutableNodeCount(platformToResponse(p))
	}
	return resp, nil
}

func (s *ControlPlaneService) getPlatformModel(id string) (*model.Platform, error) {
	p, err := s.Engine.GetPlatform(id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, notFound("platform not found")
		}
		return nil, internal("get platform", err)
	}
	return p, nil
}

// GetPlatform returns a single platform by ID.
func (s *ControlPlaneService) GetPlatform(id string) (*PlatformResponse, error) {
	mp, err := s.getPlatformModel(id)
	if err != nil {
		return nil, err
	}
	r := s.withRoutableNodeCount(platformToResponse(*mp))
	return &r, nil
}

// CreatePlatformRequest holds create platform parameters.
type CreatePlatformRequest struct {
	Name                             *string  `json:"name"`
	StickyTTL                        *string  `json:"sticky_ttl"`
	ProxyAccessMode                  *string  `json:"proxy_access_mode"`
	RotationPolicy                   *string  `json:"rotation_policy"`
	RotationInterval                 *string  `json:"rotation_interval"`
	RegexFilters                     []string `json:"regex_filters"`
	RegionFilters                    []string `json:"region_filters"`
	SubscriptionFilters              []string `json:"subscription_filters"`
	NetworkTypeFilters               []string `json:"network_type_filters"`
	MinQualityScore                  *int     `json:"min_quality_score"`
	MaxReferenceLatencyMs            *int     `json:"max_reference_latency_ms"`
	MinEgressStabilityScore          *int     `json:"min_egress_stability_score"`
	MaxCircuitOpenCount              *int     `json:"max_circuit_open_count"`
	ReverseProxyMissAction           *string  `json:"reverse_proxy_miss_action"`
	ReverseProxyEmptyAccountBehavior *string  `json:"reverse_proxy_empty_account_behavior"`
	ReverseProxyFixedAccountHeader   *string  `json:"reverse_proxy_fixed_account_header"`
	AllocationPolicy                 *string  `json:"allocation_policy"`
}

// CreatePlatform creates a new platform.
func (s *ControlPlaneService) CreatePlatform(req CreatePlatformRequest) (*PlatformResponse, error) {
	// Validate name.
	if req.Name == nil {
		return nil, invalidArg("name is required")
	}
	name := platform.NormalizePlatformName(*req.Name)
	if name == "" {
		return nil, invalidArg("name is required")
	}
	if err := platform.ValidatePlatformName(name); err != nil {
		return nil, invalidArg("name: " + err.Error())
	}
	if name == platform.DefaultPlatformName {
		return nil, conflict("cannot use reserved name 'Default'")
	}

	// Apply defaults from env and overlay request fields.
	cfg := s.defaultPlatformConfig(name)
	if req.StickyTTL != nil {
		d, err := time.ParseDuration(*req.StickyTTL)
		if err != nil {
			return nil, invalidArg("sticky_ttl: " + err.Error())
		}
		if err := setPlatformStickyTTL(&cfg, d); err != nil {
			return nil, err
		}
	}
	if req.ProxyAccessMode != nil {
		if err := setPlatformProxyAccessMode(&cfg, *req.ProxyAccessMode); err != nil {
			return nil, err
		}
	}
	if req.RotationPolicy != nil {
		if err := setPlatformRotationPolicy(&cfg, *req.RotationPolicy); err != nil {
			return nil, err
		}
	}
	if req.RotationInterval != nil {
		d, err := time.ParseDuration(*req.RotationInterval)
		if err != nil {
			return nil, invalidArg("rotation_interval: " + err.Error())
		}
		if err := setPlatformRotationInterval(&cfg, d); err != nil {
			return nil, err
		}
	}
	if req.RegexFilters != nil {
		cfg.RegexFilters = req.RegexFilters
	}
	if req.RegionFilters != nil {
		cfg.RegionFilters = req.RegionFilters
	}
	if req.SubscriptionFilters != nil {
		cfg.SubscriptionFilters = req.SubscriptionFilters
	}
	if req.NetworkTypeFilters != nil {
		cfg.NetworkTypeFilters = req.NetworkTypeFilters
	}
	cfg.MinQualityScore = req.MinQualityScore
	cfg.MaxReferenceLatencyMs = req.MaxReferenceLatencyMs
	cfg.MinEgressStabilityScore = req.MinEgressStabilityScore
	cfg.MaxCircuitOpenCount = req.MaxCircuitOpenCount
	if req.ReverseProxyMissAction != nil {
		if err := setPlatformMissAction(&cfg, *req.ReverseProxyMissAction); err != nil {
			return nil, err
		}
	}
	if req.ReverseProxyEmptyAccountBehavior != nil {
		if err := setPlatformEmptyAccountBehavior(&cfg, *req.ReverseProxyEmptyAccountBehavior); err != nil {
			return nil, err
		}
	}
	if req.ReverseProxyFixedAccountHeader != nil {
		cfg.ReverseProxyFixedAccountHeader = *req.ReverseProxyFixedAccountHeader
	}
	if req.AllocationPolicy != nil {
		if err := setPlatformAllocationPolicy(&cfg, *req.AllocationPolicy); err != nil {
			return nil, err
		}
	}
	if err := validatePlatformConfig(&cfg, true); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	mp, plat, svcErr := s.compileAndUpsertPlatform(id, cfg)
	if svcErr != nil {
		return nil, svcErr
	}

	// Register in topology pool.
	// Build the routable view before publish so concurrent readers don't observe
	// a newly created platform with an empty view.
	s.Pool.RebuildPlatform(plat)
	s.Pool.RegisterPlatform(plat)

	r := s.withRoutableNodeCount(platformToResponse(mp))
	return &r, nil
}

// UpdatePlatform applies a constrained partial patch to a platform.
// This is not RFC 7396 JSON Merge Patch: patch must be a non-empty object and
// null values are rejected.
func (s *ControlPlaneService) UpdatePlatform(id string, patchJSON json.RawMessage) (*PlatformResponse, error) {
	patch, verr := parseMergePatch(patchJSON)
	if verr != nil {
		return nil, verr
	}
	if err := patch.validateFields(platformPatchAllowedFields, func(key string) string {
		return fmt.Sprintf("field %q is read-only or unknown", key)
	}); err != nil {
		return nil, err
	}

	// Load current.
	current, err := s.getPlatformModel(id)
	if err != nil {
		return nil, err
	}
	if current.ID == platform.DefaultPlatformID {
		if nameVal, ok := patch["name"]; ok {
			nameStr, _ := nameVal.(string)
			if nameStr != platform.DefaultPlatformName {
				return nil, conflict("cannot rename Default platform")
			}
		}
	}

	cfg := platformConfigFromModel(*current)

	// Apply patch to current config.
	if nameStr, ok, err := patch.optionalNonEmptyString("name"); err != nil {
		return nil, err
	} else if ok {
		cfg.Name = platform.NormalizePlatformName(nameStr)
		if err := platform.ValidatePlatformName(cfg.Name); err != nil {
			return nil, invalidArg("name: " + err.Error())
		}
		if cfg.Name == platform.DefaultPlatformName && current.ID != platform.DefaultPlatformID {
			return nil, conflict("cannot use reserved name 'Default'")
		}
	}

	if d, ok, err := patch.optionalDurationString("sticky_ttl"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformStickyTTL(&cfg, d); err != nil {
			return nil, err
		}
	}
	if accessMode, ok, err := patch.optionalString("proxy_access_mode"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformProxyAccessMode(&cfg, accessMode); err != nil {
			return nil, err
		}
	}
	if rotationPolicy, ok, err := patch.optionalString("rotation_policy"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformRotationPolicy(&cfg, rotationPolicy); err != nil {
			return nil, err
		}
	}
	if d, ok, err := patch.optionalDurationString("rotation_interval"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformRotationInterval(&cfg, d); err != nil {
			return nil, err
		}
	}

	if filters, ok, err := patch.optionalStringSlice("regex_filters"); err != nil {
		return nil, err
	} else if ok {
		cfg.RegexFilters = filters
	}

	regionFiltersPatched := false
	if filters, ok, err := patch.optionalStringSlice("region_filters"); err != nil {
		return nil, err
	} else if ok {
		regionFiltersPatched = true
		cfg.RegionFilters = filters
	}
	if filters, ok, err := patch.optionalStringSlice("subscription_filters"); err != nil {
		return nil, err
	} else if ok {
		cfg.SubscriptionFilters = filters
	}
	if filters, ok, err := patch.optionalStringSlice("network_type_filters"); err != nil {
		return nil, err
	} else if ok {
		cfg.NetworkTypeFilters = filters
	}
	if value, ok, err := patch.optionalInt("min_quality_score"); err != nil {
		return nil, err
	} else if ok {
		cfg.MinQualityScore = &value
	}
	if value, ok, err := patch.optionalInt("max_reference_latency_ms"); err != nil {
		return nil, err
	} else if ok {
		cfg.MaxReferenceLatencyMs = &value
	}
	if value, ok, err := patch.optionalInt("min_egress_stability_score"); err != nil {
		return nil, err
	} else if ok {
		cfg.MinEgressStabilityScore = &value
	}
	if value, ok, err := patch.optionalInt("max_circuit_open_count"); err != nil {
		return nil, err
	} else if ok {
		cfg.MaxCircuitOpenCount = &value
	}

	if ma, ok, err := patch.optionalString("reverse_proxy_miss_action"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformMissAction(&cfg, ma); err != nil {
			return nil, err
		}
	}
	if behavior, ok, err := patch.optionalString("reverse_proxy_empty_account_behavior"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformEmptyAccountBehavior(&cfg, behavior); err != nil {
			return nil, err
		}
	}
	if fixedHeader, ok, err := patch.optionalString("reverse_proxy_fixed_account_header"); err != nil {
		return nil, err
	} else if ok {
		cfg.ReverseProxyFixedAccountHeader = fixedHeader
	}

	if ap, ok, err := patch.optionalString("allocation_policy"); err != nil {
		return nil, err
	} else if ok {
		if err := setPlatformAllocationPolicy(&cfg, ap); err != nil {
			return nil, err
		}
	}
	if err := validatePlatformConfig(&cfg, regionFiltersPatched); err != nil {
		return nil, err
	}

	mp, plat, svcErr := s.compileAndUpsertPlatform(id, cfg)
	if svcErr != nil {
		return nil, svcErr
	}

	// Replace in topology pool.
	if err := s.Pool.ReplacePlatform(plat); err != nil {
		return nil, internal("replace platform in pool", err)
	}

	r := s.withRoutableNodeCount(platformToResponse(mp))
	return &r, nil
}

// DeletePlatform deletes a platform.
func (s *ControlPlaneService) DeletePlatform(id string) error {
	if id == platform.DefaultPlatformID {
		return conflict("cannot delete Default platform")
	}

	if err := s.Engine.DeletePlatform(id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return notFound("platform not found")
		}
		return internal("delete platform", err)
	}
	s.Pool.UnregisterPlatform(id)
	return nil
}

// ResetPlatformToDefault resets a platform to env defaults.
func (s *ControlPlaneService) ResetPlatformToDefault(id string) (*PlatformResponse, error) {
	name, err := s.Engine.GetPlatformName(id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, notFound("platform not found")
		}
		return nil, internal("get platform", err)
	}

	cfg := s.defaultPlatformConfig(name)
	mp, plat, svcErr := s.compileAndUpsertPlatform(id, cfg)
	if svcErr != nil {
		return nil, svcErr
	}

	if err := s.Pool.ReplacePlatform(plat); err != nil {
		return nil, internal("replace platform in pool", err)
	}

	r := s.withRoutableNodeCount(platformToResponse(mp))
	return &r, nil
}

// RebuildPlatformView triggers a full rebuild of the platform's routable view.
func (s *ControlPlaneService) RebuildPlatformView(id string) error {
	plat, ok := s.Pool.GetPlatform(id)
	if !ok {
		return notFound("platform not found")
	}
	s.Pool.RebuildPlatform(plat)
	return nil
}

// PreviewFilterRequest holds preview filter parameters.
type PreviewFilterRequest struct {
	PlatformID   *string             `json:"platform_id"`
	PlatformSpec *PlatformSpecFilter `json:"platform_spec"`
}

type PlatformSpecFilter struct {
	RegexFilters            []string `json:"regex_filters"`
	RegionFilters           []string `json:"region_filters"`
	SubscriptionFilters     []string `json:"subscription_filters"`
	NetworkTypeFilters      []string `json:"network_type_filters"`
	MinQualityScore         *int     `json:"min_quality_score"`
	MaxReferenceLatencyMs   *int     `json:"max_reference_latency_ms"`
	MinEgressStabilityScore *int     `json:"min_egress_stability_score"`
	MaxCircuitOpenCount     *int     `json:"max_circuit_open_count"`
}

type PreviewFilterSummary struct {
	MatchedNodes            int            `json:"matched_nodes"`
	HealthyNodes            int            `json:"healthy_nodes"`
	UniqueEgressIPs         int            `json:"unique_egress_ips"`
	UniqueHealthyEgressIPs  int            `json:"unique_healthy_egress_ips"`
	ProfiledNodes           int            `json:"profiled_nodes"`
	UnprofiledNodes         int            `json:"unprofiled_nodes"`
	NetworkTypeBreakdown    map[string]int `json:"network_type_breakdown"`
	QualityGradeBreakdown   map[string]int `json:"quality_grade_breakdown"`
}

type PreviewFilterResponse struct {
	Items   []NodeSummary        `json:"items"`
	Summary PreviewFilterSummary `json:"summary"`
}

// NodeSummary is the API response for a node.
type NodeSummary struct {
	NodeHash                         string    `json:"node_hash"`
	CreatedAt                        string    `json:"created_at"`
	HasOutbound                      bool      `json:"has_outbound"`
	LastError                        string    `json:"last_error,omitempty"`
	CircuitOpenSince                 *string   `json:"circuit_open_since"`
	FailureCount                     int       `json:"failure_count"`
	EgressIP                         string    `json:"egress_ip,omitempty"`
	Region                           string    `json:"region,omitempty"`
	LastEgressUpdate                 string    `json:"last_egress_update,omitempty"`
	LastLatencyProbeAttempt          string    `json:"last_latency_probe_attempt,omitempty"`
	LastAuthorityLatencyProbeAttempt string    `json:"last_authority_latency_probe_attempt,omitempty"`
	ReferenceLatencyMs               *float64  `json:"reference_latency_ms,omitempty"`
	LastEgressUpdateAttempt          string    `json:"last_egress_update_attempt,omitempty"`
	EgressNetworkType                string    `json:"egress_network_type"`
	EgressASN                        *int64    `json:"egress_asn,omitempty"`
	EgressASNName                    string    `json:"egress_asn_name,omitempty"`
	EgressASNType                    string    `json:"egress_asn_type,omitempty"`
	EgressProvider                   string    `json:"egress_provider,omitempty"`
	EgressProfileSource              string    `json:"egress_profile_source,omitempty"`
	EgressProfileUpdatedAt           string    `json:"egress_profile_updated_at,omitempty"`
	QualityScore                     int       `json:"quality_score"`
	QualityGrade                     string    `json:"quality_grade"`
	EgressStabilityScore             int       `json:"egress_stability_score"`
	EgressProbeSuccessCountTotal     int64     `json:"egress_probe_success_count_total"`
	EgressProbeFailureCountTotal     int64     `json:"egress_probe_failure_count_total"`
	EgressIPChangeCountTotal         int64     `json:"egress_ip_change_count_total"`
	LastEgressIPChangeAt             string    `json:"last_egress_ip_change_at,omitempty"`
	CircuitOpenCountTotal            int64     `json:"circuit_open_count_total"`
	Tags                             []NodeTag `json:"tags"`
}

// IsHealthy follows the unified health rule used across the backend.
func (n NodeSummary) IsHealthy() bool {
	return n.HasOutbound && n.CircuitOpenSince == nil
}

type NodeTag struct {
	SubscriptionID          string `json:"subscription_id"`
	SubscriptionName        string `json:"subscription_name"`
	Tag                     string `json:"tag"`
	SubscriptionCreatedAtNs int64  `json:"-"`
}

func (s *ControlPlaneService) nodeEntryToSummary(h node.Hash, entry *node.NodeEntry) NodeSummary {
	ns := NodeSummary{
		NodeHash:     h.Hex(),
		CreatedAt:    entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		HasOutbound:  entry.HasOutbound(),
		LastError:    entry.GetLastError(),
		FailureCount: int(entry.FailureCount.Load()),
	}

	if cos := entry.CircuitOpenSince.Load(); cos > 0 {
		t := time.Unix(0, cos).UTC().Format(time.RFC3339Nano)
		ns.CircuitOpenSince = &t
	}

	egressIP := entry.GetEgressIP()
	if egressIP.IsValid() {
		ns.EgressIP = egressIP.String()
		ns.Region = entry.GetRegion(nil)
		if s.GeoIP != nil {
			ns.Region = entry.GetRegion(s.GeoIP.Lookup)
		}
	}

	if leu := entry.LastEgressUpdate.Load(); leu > 0 {
		ns.LastEgressUpdate = time.Unix(0, leu).UTC().Format(time.RFC3339Nano)
	}
	if lastAny := entry.LastLatencyProbeAttempt.Load(); lastAny > 0 {
		ns.LastLatencyProbeAttempt = time.Unix(0, lastAny).UTC().Format(time.RFC3339Nano)
	}
	if lastAuthority := entry.LastAuthorityLatencyProbeAttempt.Load(); lastAuthority > 0 {
		ns.LastAuthorityLatencyProbeAttempt = time.Unix(0, lastAuthority).UTC().Format(time.RFC3339Nano)
	}
	if avgMs, ok := entry.ReferenceLatencyMs(s.currentLatencyAuthorities()); ok {
		ns.ReferenceLatencyMs = &avgMs
	}
	if lastEgressAttempt := entry.LastEgressUpdateAttempt.Load(); lastEgressAttempt > 0 {
		ns.LastEgressUpdateAttempt = time.Unix(0, lastEgressAttempt).UTC().Format(time.RFC3339Nano)
	}
	ns.EgressNetworkType = string(entry.GetEgressNetworkType())
	if asn := entry.GetEgressASN(); asn > 0 {
		ns.EgressASN = &asn
	}
	ns.EgressASNName = entry.GetEgressASNName()
	ns.EgressASNType = entry.GetEgressASNType()
	ns.EgressProvider = entry.GetEgressProvider()
	ns.EgressProfileSource = string(entry.GetEgressProfileSource())
	if updatedAt := entry.LastEgressProfileUpdated.Load(); updatedAt > 0 {
		ns.EgressProfileUpdatedAt = time.Unix(0, updatedAt).UTC().Format(time.RFC3339Nano)
	}
	ns.QualityScore = int(entry.QualityScore.Load())
	ns.QualityGrade = string(entry.GetQualityGrade())
	ns.EgressStabilityScore = entry.EgressStabilityScore()
	ns.EgressProbeSuccessCountTotal = entry.EgressProbeSuccessCountTotal.Load()
	ns.EgressProbeFailureCountTotal = entry.EgressProbeFailureCountTotal.Load()
	ns.EgressIPChangeCountTotal = entry.EgressIPChangeCountTotal.Load()
	ns.CircuitOpenCountTotal = entry.CircuitOpenCountTotal.Load()
	if changedAt := entry.LastEgressIPChangeAt.Load(); changedAt > 0 {
		ns.LastEgressIPChangeAt = time.Unix(0, changedAt).UTC().Format(time.RFC3339Nano)
	}

	// Build tags.
	subIDs := entry.SubscriptionIDs()
	for _, subID := range subIDs {
		sub := s.SubMgr.Lookup(subID)
		if sub == nil {
			continue
		}
		managed, ok := sub.ManagedNodes().LoadNode(h)
		if !ok {
			continue
		}
		tags := managed.Tags
		for _, tag := range tags {
			ns.Tags = append(ns.Tags, NodeTag{
				SubscriptionID:          subID,
				SubscriptionName:        sub.Name(),
				Tag:                     sub.Name() + "/" + tag,
				SubscriptionCreatedAtNs: sub.CreatedAtNs,
			})
		}
	}
	if ns.Tags == nil {
		ns.Tags = []NodeTag{}
	}
	return ns
}

// PreviewFilter returns nodes matching the given filter spec.
func (s *ControlPlaneService) PreviewFilter(req PreviewFilterRequest) (*PreviewFilterResponse, error) {
	hasPlatformID := req.PlatformID != nil && *req.PlatformID != ""
	hasPlatformSpec := req.PlatformSpec != nil

	if hasPlatformID == hasPlatformSpec {
		return nil, invalidArg("exactly one of platform_id or platform_spec is required")
	}

	var regexFilters []*regexp.Regexp
	var regionFilters []string
	var subscriptionFilters []string
	var networkTypeFilters []string
	var minQualityScore *int
	var maxReferenceLatencyMs *int
	var minEgressStabilityScore *int
	var maxCircuitOpenCount *int

	if hasPlatformID {
		plat, ok := s.Pool.GetPlatform(*req.PlatformID)
		if !ok {
			return nil, notFound("platform not found")
		}
		regexFilters = plat.RegexFilters
		regionFilters = plat.RegionFilters
		subscriptionFilters = append([]string(nil), plat.SubscriptionFilters...)
		for _, value := range plat.NetworkTypeFilters {
			networkTypeFilters = append(networkTypeFilters, string(value))
		}
		minQualityScore = plat.MinQualityScore
		maxReferenceLatencyMs = plat.MaxReferenceLatencyMs
		minEgressStabilityScore = plat.MinEgressStabilityScore
		maxCircuitOpenCount = plat.MaxCircuitOpenCount
	} else {
		compiled, err := platform.CompileRegexFilters(req.PlatformSpec.RegexFilters)
		if err != nil {
			return nil, invalidArg(err.Error())
		}
		regexFilters = compiled
		regionFilters = req.PlatformSpec.RegionFilters
		if err := platform.ValidateRegionFilters(regionFilters); err != nil {
			return nil, invalidArg(err.Error())
		}
		subscriptionFilters = req.PlatformSpec.SubscriptionFilters
		networkTypeFilters = req.PlatformSpec.NetworkTypeFilters
		if err := platform.ValidateNetworkTypeFilters(networkTypeFilters); err != nil {
			return nil, invalidArg(err.Error())
		}
		minQualityScore = req.PlatformSpec.MinQualityScore
		maxReferenceLatencyMs = req.PlatformSpec.MaxReferenceLatencyMs
		minEgressStabilityScore = req.PlatformSpec.MinEgressStabilityScore
		maxCircuitOpenCount = req.PlatformSpec.MaxCircuitOpenCount
	}

	var subLookup node.SubLookupFunc
	if s.Pool != nil {
		subLookup = s.Pool.MakeSubLookup()
	}
	allowedSubIDs := make(map[string]struct{}, len(subscriptionFilters))
	for _, subID := range subscriptionFilters {
		allowedSubIDs[subID] = struct{}{}
	}
	var regionFilterSet map[string]struct{}
	if len(regionFilters) > 0 {
		regionFilterSet = make(map[string]struct{}, len(regionFilters))
		for _, rf := range regionFilters {
			regionFilterSet[rf] = struct{}{}
		}
	}

	networkTypeSet := make(map[model.EgressNetworkType]struct{}, len(networkTypeFilters))
	for _, raw := range networkTypeFilters {
		networkTypeSet[model.NormalizeEgressNetworkType(strings.ToUpper(strings.TrimSpace(raw)))] = struct{}{}
	}
	response := &PreviewFilterResponse{
		Items: []NodeSummary{},
		Summary: PreviewFilterSummary{
			NetworkTypeBreakdown:  map[string]int{},
			QualityGradeBreakdown: map[string]int{},
		},
	}
	seenIPs := make(map[string]struct{})
	seenHealthyIPs := make(map[string]struct{})
	s.Pool.Range(func(h node.Hash, entry *node.NodeEntry) bool {
		if !entry.MatchesSubscriptionFilter(subLookup, allowedSubIDs) {
			return true
		}
		if !entry.MatchRegexsInSubscriptions(regexFilters, subLookup, allowedSubIDs) {
			return true
		}
		if len(regionFilterSet) > 0 {
			region := entry.GetRegion(nil)
			if s.GeoIP != nil {
				region = entry.GetRegion(s.GeoIP.Lookup)
			}
			if region == "" {
				return true
			}
			if _, ok := regionFilterSet[region]; !ok {
				return true
			}
		}
		if len(networkTypeSet) > 0 {
			if _, ok := networkTypeSet[entry.GetEgressNetworkType()]; !ok {
				return true
			}
		}
		if minQualityScore != nil && int(entry.QualityScore.Load()) < *minQualityScore {
			return true
		}
		if maxReferenceLatencyMs != nil {
			if latencyMs, ok := entry.ReferenceLatencyMs(s.currentLatencyAuthorities()); !ok || latencyMs > float64(*maxReferenceLatencyMs) {
				return true
			}
		}
		if minEgressStabilityScore != nil && entry.EgressStabilityScore() < *minEgressStabilityScore {
			return true
		}
		if maxCircuitOpenCount != nil && int(entry.CircuitOpenCountTotal.Load()) > *maxCircuitOpenCount {
			return true
		}
		summary := s.nodeEntryToSummary(h, entry)
		response.Items = append(response.Items, summary)
		response.Summary.MatchedNodes++
		response.Summary.NetworkTypeBreakdown[summary.EgressNetworkType]++
		response.Summary.QualityGradeBreakdown[summary.QualityGrade]++
		if summary.IsHealthy() {
			response.Summary.HealthyNodes++
		}
		if summary.EgressIP != "" {
			seenIPs[summary.EgressIP] = struct{}{}
			if summary.IsHealthy() {
				seenHealthyIPs[summary.EgressIP] = struct{}{}
			}
		}
		if entry.HasProfile() {
			response.Summary.ProfiledNodes++
		} else {
			response.Summary.UnprofiledNodes++
		}
		return true
	})
	response.Summary.UniqueEgressIPs = len(seenIPs)
	response.Summary.UniqueHealthyEgressIPs = len(seenHealthyIPs)
	return response, nil
}
