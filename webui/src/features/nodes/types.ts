export type NodeTag = {
  subscription_id: string;
  subscription_name: string;
  tag: string;
};

export type NodeSummary = {
  node_hash: string;
  created_at: string;
  has_outbound: boolean;
  last_error?: string;
  circuit_open_since?: string;
  failure_count: number;
  egress_ip?: string;
  reference_latency_ms?: number;
  region?: string;
  profile_state?: string;
  egress_network_type: "UNKNOWN" | "RESIDENTIAL" | "DATACENTER" | "MOBILE";
  egress_asn?: number;
  egress_asn_name?: string;
  egress_asn_type?: string;
  egress_provider?: string;
  egress_profile_source?: string;
  egress_profile_updated_at?: string;
  quality_score: number;
  quality_grade: string;
  egress_stability_score: number;
  egress_probe_success_count_total: number;
  egress_probe_failure_count_total: number;
  egress_ip_change_count_total: number;
  last_egress_ip_change_at?: string;
  circuit_open_count_total: number;
  last_egress_update?: string;
  last_latency_probe_attempt?: string;
  last_authority_latency_probe_attempt?: string;
  last_egress_update_attempt?: string;
  tags: NodeTag[];
};

export type PageResponse<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
  unique_egress_ips: number;
  unique_healthy_egress_ips: number;
};

export type NodeSortBy = "tag" | "created_at" | "failure_count" | "region" | "quality_score" | "reference_latency_ms";
export type SortOrder = "asc" | "desc";

export type NodeListFilters = {
  platform_id?: string;
  subscription_id?: string;
  tag_keyword?: string;
  region?: string;
  network_type?: string;
  egress_ip?: string;
  probed_since?: string;
  circuit_open?: boolean;
  has_outbound?: boolean;
  profiled?: boolean;
  min_quality_score?: number;
  max_reference_latency_ms?: number;
  min_egress_stability_score?: number;
  max_circuit_open_count?: number;
};

export type NodeListQuery = NodeListFilters & {
  sort_by?: NodeSortBy;
  sort_order?: SortOrder;
  limit?: number;
  offset?: number;
};

export type EgressProbeResult = {
  egress_ip: string;
  region?: string;
  latency_ewma_ms: number;
};

export type LatencyProbeResult = {
  latency_ewma_ms: number;
};

export type NodeReprofileBatchResult = {
  requested: number;
  accepted: number;
  failed: string[];
};
