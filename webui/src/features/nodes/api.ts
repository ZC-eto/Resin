import { apiRequest } from "../../lib/api-client";
import type {
  NodeExportResponse,
  EgressProbeResult,
  LatencyProbeResult,
  NodeReprofileBatchResult,
  NodeListQuery,
  NodeSummary,
  PageResponse,
} from "./types";

const basePath = "/api/v1/nodes";

type ApiNodeSummary = Omit<NodeSummary, "tags"> & {
  tags?: NodeSummary["tags"] | null;
  last_error?: string | null;
  circuit_open_since?: string | null;
  egress_ip?: string | null;
  egress_asn?: number | null;
  egress_asn_name?: string | null;
  egress_asn_type?: string | null;
  egress_provider?: string | null;
  egress_profile_source?: string | null;
  egress_profile_updated_at?: string | null;
  last_egress_ip_change_at?: string | null;
  reference_latency_ms?: number | null;
  region?: string | null;
  last_egress_update?: string | null;
  last_latency_probe_attempt?: string | null;
  last_authority_latency_probe_attempt?: string | null;
  last_egress_update_attempt?: string | null;
  profile_state?: string | null;
};

function normalizeNode(raw: ApiNodeSummary): NodeSummary {
  const { reference_latency_ms, ...rest } = raw;
  const normalized: NodeSummary = {
    ...rest,
    tags: Array.isArray(raw.tags) ? raw.tags : [],
    last_error: raw.last_error || "",
    circuit_open_since: raw.circuit_open_since || "",
    egress_ip: raw.egress_ip || "",
    profile_state: raw.profile_state || "",
    egress_network_type: raw.egress_network_type || "UNKNOWN",
    egress_asn: raw.egress_asn ?? undefined,
    egress_asn_name: raw.egress_asn_name || "",
    egress_asn_type: raw.egress_asn_type || "",
    egress_provider: raw.egress_provider || "",
    egress_profile_source: raw.egress_profile_source || "",
    egress_profile_updated_at: raw.egress_profile_updated_at || "",
    quality_score: typeof raw.quality_score === "number" ? raw.quality_score : 0,
    quality_grade: raw.quality_grade || "D",
    egress_stability_score: typeof raw.egress_stability_score === "number" ? raw.egress_stability_score : 0,
    egress_probe_success_count_total:
      typeof raw.egress_probe_success_count_total === "number" ? raw.egress_probe_success_count_total : 0,
    egress_probe_failure_count_total:
      typeof raw.egress_probe_failure_count_total === "number" ? raw.egress_probe_failure_count_total : 0,
    egress_ip_change_count_total:
      typeof raw.egress_ip_change_count_total === "number" ? raw.egress_ip_change_count_total : 0,
    circuit_open_count_total:
      typeof raw.circuit_open_count_total === "number" ? raw.circuit_open_count_total : 0,
    last_egress_ip_change_at: raw.last_egress_ip_change_at || "",
    region: raw.region || "",
    last_egress_update: raw.last_egress_update || "",
    last_latency_probe_attempt: raw.last_latency_probe_attempt || "",
    last_authority_latency_probe_attempt: raw.last_authority_latency_probe_attempt || "",
    last_egress_update_attempt: raw.last_egress_update_attempt || "",
  };

  // Backend uses `omitempty`; field missing means "no reference latency".
  if (typeof reference_latency_ms === "number") {
    normalized.reference_latency_ms = reference_latency_ms;
  }

  return normalized;
}

export async function listNodes(filters: NodeListQuery): Promise<PageResponse<NodeSummary>> {
  const query = new URLSearchParams({
    limit: String(filters.limit ?? 50),
    offset: String(filters.offset ?? 0),
    sort_by: filters.sort_by || "tag",
    sort_order: filters.sort_order || "asc",
  });

  const appendIfNotEmpty = (key: string, value?: string) => {
    if (!value) {
      return;
    }
    const trimmed = value.trim();
    if (!trimmed) {
      return;
    }
    query.set(key, trimmed);
  };

  appendIfNotEmpty("platform_id", filters.platform_id);
  appendIfNotEmpty("subscription_id", filters.subscription_id);
  appendIfNotEmpty("tag_keyword", filters.tag_keyword);
  appendIfNotEmpty("region", filters.region?.toLowerCase());
  appendIfNotEmpty("network_type", filters.network_type);
  appendIfNotEmpty("egress_ip", filters.egress_ip);
  appendIfNotEmpty("probed_since", filters.probed_since);

  if (filters.circuit_open !== undefined) {
    query.set("circuit_open", String(filters.circuit_open));
  }
  if (filters.has_outbound !== undefined) {
    query.set("has_outbound", String(filters.has_outbound));
  }
  if (filters.profiled !== undefined) {
    query.set("profiled", String(filters.profiled));
  }
  if (filters.min_quality_score !== undefined) {
    query.set("min_quality_score", String(filters.min_quality_score));
  }
  if (filters.max_reference_latency_ms !== undefined) {
    query.set("max_reference_latency_ms", String(filters.max_reference_latency_ms));
  }
  if (filters.min_egress_stability_score !== undefined) {
    query.set("min_egress_stability_score", String(filters.min_egress_stability_score));
  }
  if (filters.max_circuit_open_count !== undefined) {
    query.set("max_circuit_open_count", String(filters.max_circuit_open_count));
  }

  const data = await apiRequest<PageResponse<ApiNodeSummary>>(`${basePath}?${query.toString()}`);
  return {
    ...data,
    items: data.items.map(normalizeNode),
  };
}

export async function getNode(hash: string): Promise<NodeSummary> {
  const data = await apiRequest<ApiNodeSummary>(`${basePath}/${hash}`);
  return normalizeNode(data);
}

export async function exportNode(hash: string): Promise<NodeExportResponse> {
  return apiRequest<NodeExportResponse>(`${basePath}/${hash}/export`);
}

export async function probeEgress(hash: string): Promise<EgressProbeResult> {
  return apiRequest<EgressProbeResult>(`${basePath}/${hash}/actions/probe-egress`, {
    method: "POST",
  });
}

export async function probeLatency(hash: string): Promise<LatencyProbeResult> {
  return apiRequest<LatencyProbeResult>(`${basePath}/${hash}/actions/probe-latency`, {
    method: "POST",
  });
}

export async function reprofileNode(hash: string): Promise<NodeSummary> {
  const data = await apiRequest<ApiNodeSummary>(`${basePath}/${hash}/actions/reprofile`, {
    method: "POST",
  });
  return normalizeNode(data);
}

export async function reprofileNodes(hashes: string[]): Promise<NodeReprofileBatchResult> {
  return apiRequest<NodeReprofileBatchResult>(`${basePath}/actions/reprofile`, {
    method: "POST",
    body: { hashes },
  });
}
