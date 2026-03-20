export type PlatformMissAction = "TREAT_AS_EMPTY" | "REJECT";
export type PlatformEmptyAccountBehavior = "RANDOM" | "FIXED_HEADER" | "ACCOUNT_HEADER_RULE";
export type PlatformAllocationPolicy = "BALANCED" | "PREFER_LOW_LATENCY" | "PREFER_IDLE_IP";
export type PlatformProxyAccessMode = "STANDARD" | "STICKY";
export type PlatformRotationPolicy = "KEEP" | "TTL";
export type NetworkType = "UNKNOWN" | "RESIDENTIAL" | "DATACENTER" | "MOBILE";

export type Platform = {
  id: string;
  name: string;
  sticky_ttl: string;
  proxy_access_mode: PlatformProxyAccessMode;
  rotation_policy: PlatformRotationPolicy;
  rotation_interval: string;
  regex_filters: string[];
  region_filters: string[];
  subscription_filters: string[];
  network_type_filters: NetworkType[];
  min_quality_score?: number;
  max_reference_latency_ms?: number;
  min_egress_stability_score?: number;
  max_circuit_open_count?: number;
  routable_node_count: number;
  reverse_proxy_miss_action: PlatformMissAction;
  reverse_proxy_empty_account_behavior: PlatformEmptyAccountBehavior;
  reverse_proxy_fixed_account_header: string;
  allocation_policy: PlatformAllocationPolicy;
  updated_at: string;
};

export type PageResponse<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type PlatformCreateInput = {
  name: string;
  sticky_ttl?: string;
  proxy_access_mode?: PlatformProxyAccessMode;
  rotation_policy?: PlatformRotationPolicy;
  rotation_interval?: string;
  regex_filters?: string[];
  region_filters?: string[];
  subscription_filters?: string[];
  network_type_filters?: NetworkType[];
  min_quality_score?: number;
  max_reference_latency_ms?: number;
  min_egress_stability_score?: number;
  max_circuit_open_count?: number;
  reverse_proxy_miss_action?: PlatformMissAction;
  reverse_proxy_empty_account_behavior?: PlatformEmptyAccountBehavior;
  reverse_proxy_fixed_account_header?: string;
  allocation_policy?: PlatformAllocationPolicy;
};

export type PlatformUpdateInput = {
  name?: string;
  sticky_ttl?: string;
  proxy_access_mode?: PlatformProxyAccessMode;
  rotation_policy?: PlatformRotationPolicy;
  rotation_interval?: string;
  regex_filters?: string[];
  region_filters?: string[];
  subscription_filters?: string[];
  network_type_filters?: NetworkType[];
  min_quality_score?: number;
  max_reference_latency_ms?: number;
  min_egress_stability_score?: number;
  max_circuit_open_count?: number;
  reverse_proxy_miss_action?: PlatformMissAction;
  reverse_proxy_empty_account_behavior?: PlatformEmptyAccountBehavior;
  reverse_proxy_fixed_account_header?: string;
  allocation_policy?: PlatformAllocationPolicy;
};

export type PlatformPreviewSummary = {
  matched_nodes: number;
  healthy_nodes: number;
  unique_egress_ips: number;
  unique_healthy_egress_ips: number;
  profiled_nodes: number;
  unprofiled_nodes: number;
  network_type_breakdown: Record<string, number>;
  quality_grade_breakdown: Record<string, number>;
};

export type PlatformPreviewResponse = {
  items: unknown[];
  total: number;
  limit: number;
  offset: number;
  summary: PlatformPreviewSummary;
};
