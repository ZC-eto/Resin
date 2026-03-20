package platform

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Resinat/Resin/internal/model"
)

func isLowerAlpha2(s string) bool {
	if len(s) != 2 {
		return false
	}
	return s[0] >= 'a' && s[0] <= 'z' && s[1] >= 'a' && s[1] <= 'z'
}

// ValidateRegionFilters validates region filters against lowercase ISO alpha-2 format.
func ValidateRegionFilters(regionFilters []string) error {
	for i, r := range regionFilters {
		if !isLowerAlpha2(r) {
			return fmt.Errorf("region_filters[%d]: must be a 2-letter lowercase ISO 3166-1 alpha-2 code (e.g. us, jp)", i)
		}
	}
	return nil
}

// CompileRegexFilters compiles regex filters in order.
func CompileRegexFilters(regexFilters []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(regexFilters))
	for i, re := range regexFilters {
		c, err := regexp.Compile(re)
		if err != nil {
			return nil, fmt.Errorf("regex_filters[%d]: invalid regex: %v", i, err)
		}
		compiled = append(compiled, c)
	}
	return compiled, nil
}

// ValidateNetworkTypeFilters validates configured network types.
func ValidateNetworkTypeFilters(filters []string) error {
	for i, raw := range filters {
		normalized := model.NormalizeEgressNetworkType(strings.ToUpper(strings.TrimSpace(raw)))
		if normalized == model.EgressNetworkTypeUnknown && strings.ToUpper(strings.TrimSpace(raw)) != string(model.EgressNetworkTypeUnknown) {
			return fmt.Errorf(
				"network_type_filters[%d]: must be one of %s, %s, %s, %s",
				i,
				model.EgressNetworkTypeResidential,
				model.EgressNetworkTypeDatacenter,
				model.EgressNetworkTypeMobile,
				model.EgressNetworkTypeUnknown,
			)
		}
	}
	return nil
}

// NewConfiguredPlatform builds a runtime platform with non-filter settings applied.
func NewConfiguredPlatform(
	id, name string,
	regexFilters []*regexp.Regexp,
	regionFilters []string,
	stickyTTLNs int64,
	rotationPolicy string,
	rotationIntervalNs int64,
	missAction string,
	emptyAccountBehavior string,
	fixedAccountHeader string,
	allocationPolicy string,
) *Platform {
	normalizedFixedHeaders, fixedHeaders, err := NormalizeFixedAccountHeaders(fixedAccountHeader)
	if err != nil {
		normalizedFixedHeaders = strings.TrimSpace(fixedAccountHeader)
		fixedHeaders = nil
	}
	plat := NewPlatform(id, name, regexFilters, regionFilters)
	plat.StickyTTLNs = stickyTTLNs
	plat.RotationPolicy = EffectiveRotationPolicy(rotationPolicy, rotationIntervalNs, stickyTTLNs)
	plat.RotationIntervalNs = EffectiveRotationIntervalNs(rotationIntervalNs, stickyTTLNs)
	plat.ReverseProxyMissAction = missAction
	plat.ReverseProxyEmptyAccountBehavior = emptyAccountBehavior
	plat.ReverseProxyFixedAccountHeader = normalizedFixedHeaders
	plat.ReverseProxyFixedAccountHeaders = append([]string(nil), fixedHeaders...)
	plat.AllocationPolicy = ParseAllocationPolicy(allocationPolicy)
	return plat
}

// CompileModelRegexFilters compiles regex filters from persisted model values.
func CompileModelRegexFilters(platformID string, regexFilters []string) ([]*regexp.Regexp, error) {
	compiled, err := CompileRegexFilters(regexFilters)
	if err != nil {
		return nil, fmt.Errorf("decode platform %s regex_filters: %w", platformID, err)
	}
	return compiled, nil
}

// BuildFromModel builds a runtime platform from a persisted model.Platform.
func BuildFromModel(mp model.Platform) (*Platform, error) {
	regexFilters, err := CompileModelRegexFilters(mp.ID, mp.RegexFilters)
	if err != nil {
		return nil, err
	}
	if err := ValidateRegionFilters(mp.RegionFilters); err != nil {
		return nil, err
	}
	if err := ValidateNetworkTypeFilters(mp.NetworkTypeFilters); err != nil {
		return nil, err
	}
	emptyAccountBehavior := mp.ReverseProxyEmptyAccountBehavior
	if !ReverseProxyEmptyAccountBehavior(emptyAccountBehavior).IsValid() {
		emptyAccountBehavior = string(ReverseProxyEmptyAccountBehaviorRandom)
	}
	missAction := NormalizeReverseProxyMissAction(mp.ReverseProxyMissAction)
	if missAction == "" {
		return nil, fmt.Errorf(
			"decode platform %s reverse_proxy_miss_action: invalid value %q",
			mp.ID,
			mp.ReverseProxyMissAction,
		)
	}
	fixedHeader, _, err := NormalizeFixedAccountHeaders(mp.ReverseProxyFixedAccountHeader)
	if err != nil {
		return nil, fmt.Errorf("decode platform %s reverse_proxy_fixed_account_header: %w", mp.ID, err)
	}
	if emptyAccountBehavior == string(ReverseProxyEmptyAccountBehaviorFixedHeader) && fixedHeader == "" {
		return nil, fmt.Errorf(
			"decode platform %s reverse_proxy_fixed_account_header: required when reverse_proxy_empty_account_behavior is %s",
			mp.ID,
			ReverseProxyEmptyAccountBehaviorFixedHeader,
		)
	}

	networkTypes := make([]model.EgressNetworkType, 0, len(mp.NetworkTypeFilters))
	for _, raw := range mp.NetworkTypeFilters {
		networkTypes = append(networkTypes, model.NormalizeEgressNetworkType(strings.ToUpper(strings.TrimSpace(raw))))
	}

	plat := NewConfiguredPlatform(
		mp.ID,
		mp.Name,
		regexFilters,
		append([]string(nil), mp.RegionFilters...),
		mp.StickyTTLNs,
		mp.RotationPolicy,
		mp.RotationIntervalNs,
		string(missAction),
		emptyAccountBehavior,
		fixedHeader,
		mp.AllocationPolicy,
	)
	plat.SubscriptionFilters = append([]string(nil), mp.SubscriptionFilters...)
	plat.NetworkTypeFilters = networkTypes
	plat.MinQualityScore = mp.MinQualityScore
	plat.MaxReferenceLatencyMs = mp.MaxReferenceLatencyMs
	plat.MinEgressStabilityScore = mp.MinEgressStabilityScore
	plat.MaxCircuitOpenCount = mp.MaxCircuitOpenCount
	return plat, nil
}
