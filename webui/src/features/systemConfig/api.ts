import { apiRequest } from "../../lib/api-client";
import type { EnvConfig, RuntimeConfig, RuntimeConfigPatch, SystemTaskStatus } from "./types";

const path = "/api/v1/system/config";

const DEFAULT_CONFIG: RuntimeConfig = {
  user_agent: "",
  request_log_enabled: true,
  reverse_proxy_log_detail_enabled: false,
  reverse_proxy_log_req_headers_max_bytes: 0,
  reverse_proxy_log_req_body_max_bytes: 0,
  reverse_proxy_log_resp_headers_max_bytes: 0,
  reverse_proxy_log_resp_body_max_bytes: 0,
  max_consecutive_failures: 0,
  max_latency_test_interval: "",
  max_authority_latency_test_interval: "",
  max_egress_test_interval: "",
  latency_test_url: "",
  latency_authorities: [],
  ip_profile_local_lookup_enabled: true,
  ip_profile_online_provider: "DISABLED",
  ip_profile_online_api_key: "",
  ip_profile_online_requests_per_minute: 120,
  ip_profile_cache_ttl: "720h",
  ip_profile_background_enabled: true,
  ip_profile_refresh_on_egress_change: true,
  stale_node_cleanup_enabled: false,
  stale_node_cleanup_window: "168h0m0s",
  p2c_latency_window: "",
  latency_decay_window: "",
  cache_flush_interval: "",
  cache_flush_dirty_threshold: 0,
};

function asNumber(raw: unknown, fallback: number): number {
  const value = Number(raw);
  if (!Number.isFinite(value)) {
    return fallback;
  }
  return value;
}

function asString(raw: unknown, fallback: string): string {
  if (typeof raw !== "string") {
    return fallback;
  }
  return raw;
}

function normalizeRuntimeConfig(raw: Partial<RuntimeConfig> | null | undefined): RuntimeConfig {
  if (!raw) {
    return DEFAULT_CONFIG;
  }

  return {
    user_agent: asString(raw.user_agent, DEFAULT_CONFIG.user_agent),
    request_log_enabled: Boolean(raw.request_log_enabled),
    reverse_proxy_log_detail_enabled: Boolean(raw.reverse_proxy_log_detail_enabled),
    reverse_proxy_log_req_headers_max_bytes: asNumber(
      raw.reverse_proxy_log_req_headers_max_bytes,
      DEFAULT_CONFIG.reverse_proxy_log_req_headers_max_bytes,
    ),
    reverse_proxy_log_req_body_max_bytes: asNumber(
      raw.reverse_proxy_log_req_body_max_bytes,
      DEFAULT_CONFIG.reverse_proxy_log_req_body_max_bytes,
    ),
    reverse_proxy_log_resp_headers_max_bytes: asNumber(
      raw.reverse_proxy_log_resp_headers_max_bytes,
      DEFAULT_CONFIG.reverse_proxy_log_resp_headers_max_bytes,
    ),
    reverse_proxy_log_resp_body_max_bytes: asNumber(
      raw.reverse_proxy_log_resp_body_max_bytes,
      DEFAULT_CONFIG.reverse_proxy_log_resp_body_max_bytes,
    ),
    max_consecutive_failures: asNumber(raw.max_consecutive_failures, DEFAULT_CONFIG.max_consecutive_failures),
    max_latency_test_interval: asString(raw.max_latency_test_interval, DEFAULT_CONFIG.max_latency_test_interval),
    max_authority_latency_test_interval: asString(
      raw.max_authority_latency_test_interval,
      DEFAULT_CONFIG.max_authority_latency_test_interval,
    ),
    max_egress_test_interval: asString(raw.max_egress_test_interval, DEFAULT_CONFIG.max_egress_test_interval),
    latency_test_url: asString(raw.latency_test_url, DEFAULT_CONFIG.latency_test_url),
    latency_authorities: Array.isArray(raw.latency_authorities)
      ? raw.latency_authorities.filter((item): item is string => typeof item === "string")
      : DEFAULT_CONFIG.latency_authorities,
    ip_profile_local_lookup_enabled: Boolean(raw.ip_profile_local_lookup_enabled),
    ip_profile_online_provider: asString(raw.ip_profile_online_provider, DEFAULT_CONFIG.ip_profile_online_provider),
    ip_profile_online_api_key: asString(raw.ip_profile_online_api_key, DEFAULT_CONFIG.ip_profile_online_api_key),
    ip_profile_online_requests_per_minute: asNumber(
      raw.ip_profile_online_requests_per_minute,
      DEFAULT_CONFIG.ip_profile_online_requests_per_minute,
    ),
    ip_profile_cache_ttl: asString(raw.ip_profile_cache_ttl, DEFAULT_CONFIG.ip_profile_cache_ttl),
    ip_profile_background_enabled: Boolean(raw.ip_profile_background_enabled),
    ip_profile_refresh_on_egress_change: Boolean(raw.ip_profile_refresh_on_egress_change),
    stale_node_cleanup_enabled: Boolean(raw.stale_node_cleanup_enabled),
    stale_node_cleanup_window: asString(raw.stale_node_cleanup_window, DEFAULT_CONFIG.stale_node_cleanup_window),
    p2c_latency_window: asString(raw.p2c_latency_window, DEFAULT_CONFIG.p2c_latency_window),
    latency_decay_window: asString(raw.latency_decay_window, DEFAULT_CONFIG.latency_decay_window),
    cache_flush_interval: asString(raw.cache_flush_interval, DEFAULT_CONFIG.cache_flush_interval),
    cache_flush_dirty_threshold: asNumber(
      raw.cache_flush_dirty_threshold,
      DEFAULT_CONFIG.cache_flush_dirty_threshold,
    ),
  };
}

export async function getSystemConfig(): Promise<RuntimeConfig> {
  const data = await apiRequest<RuntimeConfig>(path);
  return normalizeRuntimeConfig(data);
}

export async function getDefaultSystemConfig(): Promise<RuntimeConfig> {
  const data = await apiRequest<RuntimeConfig>(path + "/default");
  return normalizeRuntimeConfig(data);
}

export async function patchSystemConfig(patch: RuntimeConfigPatch): Promise<RuntimeConfig> {
  const data = await apiRequest<RuntimeConfig>(path, {
    method: "PATCH",
    body: patch,
  });
  return normalizeRuntimeConfig(data);
}

export async function getEnvConfig(): Promise<EnvConfig> {
  return await apiRequest<EnvConfig>(path + "/env");
}

export async function reprofileKnownNodes(): Promise<{ requested: number; accepted: number; failed: string[] }> {
  return await apiRequest(path.replace("/config", "/actions/reprofile-known-nodes"), {
    method: "POST",
  });
}

export async function getSystemTaskStatus(): Promise<SystemTaskStatus> {
  return await apiRequest<SystemTaskStatus>(path.replace("/config", "/tasks/status"));
}
