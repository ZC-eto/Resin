package platform

import "testing"

func TestEffectiveRotationPolicy_LegacyZeroTTLDefaultsToKeep(t *testing.T) {
	if got := EffectiveRotationPolicy(string(RotationPolicyTTL), 0, 0); got != RotationPolicyKeep {
		t.Fatalf("EffectiveRotationPolicy(TTL, 0, 0) = %q, want %q", got, RotationPolicyKeep)
	}
}

func TestEffectiveRotationPolicy_TTLRemainsTTLWhenIntervalExists(t *testing.T) {
	if got := EffectiveRotationPolicy(string(RotationPolicyTTL), 1, 0); got != RotationPolicyTTL {
		t.Fatalf("EffectiveRotationPolicy(TTL, 1, 0) = %q, want %q", got, RotationPolicyTTL)
	}
}
