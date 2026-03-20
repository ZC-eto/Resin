// Package model defines domain structs shared across the persistence layer.
package model

import "encoding/json"

type EgressNetworkType string

const (
	EgressNetworkTypeUnknown     EgressNetworkType = "UNKNOWN"
	EgressNetworkTypeResidential EgressNetworkType = "RESIDENTIAL"
	EgressNetworkTypeDatacenter  EgressNetworkType = "DATACENTER"
	EgressNetworkTypeMobile      EgressNetworkType = "MOBILE"
)

func NormalizeEgressNetworkType(raw string) EgressNetworkType {
	switch EgressNetworkType(raw) {
	case EgressNetworkTypeResidential, EgressNetworkTypeDatacenter, EgressNetworkTypeMobile:
		return EgressNetworkType(raw)
	default:
		return EgressNetworkTypeUnknown
	}
}

type EgressProfileSource string

const (
	EgressProfileSourceUnknown         EgressProfileSource = ""
	EgressProfileSourceLocal           EgressProfileSource = "LOCAL"
	EgressProfileSourceOnline          EgressProfileSource = "ONLINE"
	EgressProfileSourceLocalPlusOnline EgressProfileSource = "LOCAL_PLUS_ONLINE"
)

func NormalizeEgressProfileSource(raw string) EgressProfileSource {
	switch EgressProfileSource(raw) {
	case EgressProfileSourceLocal, EgressProfileSourceOnline, EgressProfileSourceLocalPlusOnline:
		return EgressProfileSource(raw)
	default:
		return EgressProfileSourceUnknown
	}
}

type QualityGrade string

const (
	QualityGradeA QualityGrade = "A"
	QualityGradeB QualityGrade = "B"
	QualityGradeC QualityGrade = "C"
	QualityGradeD QualityGrade = "D"
)

func NormalizeQualityGrade(raw string) QualityGrade {
	switch QualityGrade(raw) {
	case QualityGradeA, QualityGradeB, QualityGradeC, QualityGradeD:
		return QualityGrade(raw)
	default:
		return QualityGradeD
	}
}

// Platform represents a routing platform.
type Platform struct {
	ID                               string `json:"id"`
	Name                             string `json:"name"`
	StickyTTLNs                      int64  `json:"sticky_ttl_ns"`
	ProxyAccessMode                  string `json:"proxy_access_mode"`
	RotationPolicy                   string `json:"rotation_policy"`
	RotationIntervalNs               int64  `json:"rotation_interval_ns"`
	RegexFilters                     []string
	RegionFilters                    []string
	SubscriptionFilters              []string
	NetworkTypeFilters               []string
	MinQualityScore                  *int   `json:"min_quality_score,omitempty"`
	MaxReferenceLatencyMs            *int   `json:"max_reference_latency_ms,omitempty"`
	MinEgressStabilityScore          *int   `json:"min_egress_stability_score,omitempty"`
	MaxCircuitOpenCount              *int   `json:"max_circuit_open_count,omitempty"`
	ReverseProxyMissAction           string `json:"reverse_proxy_miss_action"`
	ReverseProxyEmptyAccountBehavior string `json:"reverse_proxy_empty_account_behavior"`
	ReverseProxyFixedAccountHeader   string `json:"reverse_proxy_fixed_account_header"`
	AllocationPolicy                 string `json:"allocation_policy"`
	UpdatedAtNs                      int64  `json:"updated_at_ns"`
}

// Subscription represents a node subscription source.
type SubscriptionSource struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

// Subscription represents a node subscription source.
type Subscription struct {
	ID                        string               `json:"id"`
	Name                      string               `json:"name"`
	SourceType                string               `json:"source_type"`
	URL                       string               `json:"url"`
	Content                   string               `json:"content"`
	Sources                   []SubscriptionSource `json:"sources"`
	UpdateIntervalNs          int64                `json:"update_interval_ns"`
	Enabled                   bool                 `json:"enabled"`
	Ephemeral                 bool                 `json:"ephemeral"`
	EphemeralNodeEvictDelayNs int64                `json:"ephemeral_node_evict_delay_ns"`
	CreatedAtNs               int64                `json:"created_at_ns"`
	UpdatedAtNs               int64                `json:"updated_at_ns"`
}

// AccountHeaderRule defines header extraction rules for reverse proxy account matching.
type AccountHeaderRule struct {
	URLPrefix   string `json:"url_prefix"`
	Headers     []string
	UpdatedAtNs int64 `json:"updated_at_ns"`
}

// NodeStatic holds the immutable portion of a node's data.
type NodeStatic struct {
	Hash        string          `json:"hash"`
	RawOptions  json.RawMessage `json:"raw_options_json"`
	CreatedAtNs int64           `json:"created_at_ns"`
}

// NodeDynamic holds the mutable runtime state of a node.
type NodeDynamic struct {
	Hash                               string `json:"hash"`
	FailureCount                       int    `json:"failure_count"`
	CircuitOpenSince                   int64  `json:"circuit_open_since"`
	EgressIP                           string `json:"egress_ip"`
	EgressRegion                       string `json:"egress_region"`
	EgressUpdatedAtNs                  int64  `json:"egress_updated_at_ns"`
	LastLatencyProbeAttemptNs          int64  `json:"last_latency_probe_attempt_ns"`
	LastAuthorityLatencyProbeAttemptNs int64  `json:"last_authority_latency_probe_attempt_ns"`
	LastEgressUpdateAttemptNs          int64  `json:"last_egress_update_attempt_ns"`
	EgressNetworkType                  string `json:"egress_network_type"`
	EgressASN                          int64  `json:"egress_asn"`
	EgressASNName                      string `json:"egress_asn_name"`
	EgressASNType                      string `json:"egress_asn_type"`
	EgressProvider                     string `json:"egress_provider"`
	EgressProfileSource                string `json:"egress_profile_source"`
	EgressProfileUpdatedAtNs           int64  `json:"egress_profile_updated_at_ns"`
	QualityScore                       int    `json:"quality_score"`
	QualityGrade                       string `json:"quality_grade"`
	EgressProbeSuccessCountTotal       int64  `json:"egress_probe_success_count_total"`
	EgressProbeFailureCountTotal       int64  `json:"egress_probe_failure_count_total"`
	EgressIPChangeCountTotal           int64  `json:"egress_ip_change_count_total"`
	LastEgressIPChangeAtNs             int64  `json:"last_egress_ip_change_at_ns"`
	CircuitOpenCountTotal              int64  `json:"circuit_open_count_total"`
	StaleCleanupWindowStartedAtNs      int64  `json:"stale_cleanup_window_started_at_ns"`
	StaleCleanupLastObservedProbeAtNs  int64  `json:"stale_cleanup_last_observed_probe_at_ns"`
	StaleCleanupFailedProbeCount       int64  `json:"stale_cleanup_failed_probe_count"`
}

// EgressProfileCacheEntry stores per-egress-IP profile snapshots.
type EgressProfileCacheEntry struct {
	EgressIP                 string `json:"egress_ip"`
	EgressNetworkType        string `json:"egress_network_type"`
	EgressASN                int64  `json:"egress_asn"`
	EgressASNName            string `json:"egress_asn_name"`
	EgressASNType            string `json:"egress_asn_type"`
	EgressProvider           string `json:"egress_provider"`
	EgressProfileSource      string `json:"egress_profile_source"`
	EgressProfileUpdatedAtNs int64  `json:"egress_profile_updated_at_ns"`
}

// NodeLatency holds per-domain latency statistics for a node.
type NodeLatency struct {
	NodeHash      string `json:"node_hash"`
	Domain        string `json:"domain"`
	EwmaNs        int64  `json:"ewma_ns"`
	LastUpdatedNs int64  `json:"last_updated_ns"`
}

// NodeLatencyKey is the composite primary key for node_latency.
type NodeLatencyKey struct {
	NodeHash string
	Domain   string
}

// Lease represents a sticky routing lease.
type Lease struct {
	PlatformID     string `json:"platform_id"`
	Account        string `json:"account"`
	NodeHash       string `json:"node_hash"`
	EgressIP       string `json:"egress_ip"`
	CreatedAtNs    int64  `json:"created_at_ns"`
	ExpiryNs       int64  `json:"expiry_ns"`
	LastAccessedNs int64  `json:"last_accessed_ns"`
}

// LeaseKey is the composite primary key for leases.
type LeaseKey struct {
	PlatformID string
	Account    string
}

// SubscriptionNode links a subscription to a node with tags.
type SubscriptionNode struct {
	SubscriptionID string `json:"subscription_id"`
	NodeHash       string `json:"node_hash"`
	Tags           []string
	Evicted        bool `json:"evicted"`
}

// SubscriptionNodeKey is the composite primary key for subscription_nodes.
type SubscriptionNodeKey struct {
	SubscriptionID string
	NodeHash       string
}
