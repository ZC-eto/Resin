package platform

import "strings"

type RotationPolicy string

const (
	RotationPolicyKeep RotationPolicy = "KEEP"
	RotationPolicyTTL  RotationPolicy = "TTL"
)

func NormalizeRotationPolicy(raw string) RotationPolicy {
	switch RotationPolicy(strings.ToUpper(strings.TrimSpace(raw))) {
	case RotationPolicyKeep:
		return RotationPolicyKeep
	case RotationPolicyTTL:
		return RotationPolicyTTL
	default:
		return ""
	}
}

func (p RotationPolicy) IsValid() bool {
	return p == RotationPolicyKeep || p == RotationPolicyTTL
}

func EffectiveRotationPolicy(raw string, rotationIntervalNs, stickyTTLNs int64) RotationPolicy {
	normalized := NormalizeRotationPolicy(raw)
	if normalized == RotationPolicyTTL && rotationIntervalNs <= 0 && stickyTTLNs <= 0 {
		return RotationPolicyKeep
	}
	if normalized.IsValid() {
		return normalized
	}
	if rotationIntervalNs > 0 || stickyTTLNs > 0 {
		return RotationPolicyTTL
	}
	return RotationPolicyKeep
}

func EffectiveRotationIntervalNs(rotationIntervalNs, stickyTTLNs int64) int64 {
	if rotationIntervalNs > 0 {
		return rotationIntervalNs
	}
	if stickyTTLNs > 0 {
		return stickyTTLNs
	}
	return 0
}
