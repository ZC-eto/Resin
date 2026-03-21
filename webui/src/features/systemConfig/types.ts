export type RuntimeConfig = {
  user_agent: string;
  request_log_enabled: boolean;
  reverse_proxy_log_detail_enabled: boolean;
  reverse_proxy_log_req_headers_max_bytes: number;
  reverse_proxy_log_req_body_max_bytes: number;
  reverse_proxy_log_resp_headers_max_bytes: number;
  reverse_proxy_log_resp_body_max_bytes: number;
  max_consecutive_failures: number;
  max_latency_test_interval: string;
  max_authority_latency_test_interval: string;
  max_egress_test_interval: string;
  latency_test_url: string;
  latency_authorities: string[];
  ip_profile_local_lookup_enabled: boolean;
  ip_profile_online_provider: string;
  ip_profile_online_api_key: string;
  ip_profile_online_requests_per_minute: number;
  ip_profile_cache_ttl: string;
  ip_profile_background_enabled: boolean;
  ip_profile_refresh_on_egress_change: boolean;
  stale_node_cleanup_enabled: boolean;
  stale_node_cleanup_window: string;
  p2c_latency_window: string;
  latency_decay_window: string;
  cache_flush_interval: string;
  cache_flush_dirty_threshold: number;
};

export type EnvConfig = {
  cache_dir: string;
  state_dir: string;
  log_dir: string;
  listen_address: string;
  resin_port: number;
  api_max_body_bytes: number;
  max_latency_table_entries: number;
  probe_concurrency: number;
  geoip_update_schedule: string;
  default_platform_sticky_ttl: string;
  default_platform_regex_filters: string[] | null;
  default_platform_region_filters: string[] | null;
  default_platform_reverse_proxy_miss_action: string;
  default_platform_reverse_proxy_empty_account_behavior: string;
  default_platform_reverse_proxy_fixed_account_header: string;
  default_platform_allocation_policy: string;
  probe_timeout: string;
  resource_fetch_timeout: string;
  proxy_transport_max_idle_conns: number;
  proxy_transport_max_idle_conns_per_host: number;
  proxy_transport_idle_conn_timeout: string;
  request_log_queue_size: number;
  request_log_queue_flush_batch_size: number;
  request_log_queue_flush_interval: string;
  request_log_db_max_mb: number;
  request_log_db_retain_count: number;
  metric_throughput_interval_seconds: number;
  metric_throughput_retention_seconds: number;
  metric_bucket_seconds: number;
  metric_connections_interval_seconds: number;
  metric_connections_retention_seconds: number;
  metric_leases_interval_seconds: number;
  metric_leases_retention_seconds: number;
  metric_latency_bin_width_ms: number;
  metric_latency_bin_overflow_ms: number;
  admin_token_set: boolean;
  proxy_token_set: boolean;
  admin_token_weak: boolean;
  proxy_token_weak: boolean;
};

export type RuntimeConfigPatch = Partial<RuntimeConfig>;

export type ProbeRuntimeStatus = {
  in_flight_egress: number;
  in_flight_latency: number;
  due_egress_nodes: number;
  due_latency_nodes: number;
  unknown_egress_nodes: number;
  unknown_circuit_open_nodes: number;
  known_egress_nodes: number;
  last_egress_scan_at_ns: number;
  last_latency_scan_at_ns: number;
};

export type IPProfileRuntimeStatus = {
  background_enabled: boolean;
  queue_total: number;
  queue_healthy: number;
  queue_unhealthy: number;
  queue_manual: number;
  running_total: number;
  running_healthy: number;
  pending_known_nodes: number;
  pending_healthy_nodes: number;
  pending_unhealthy_nodes: number;
  last_started_at_ns: number;
  last_finished_at_ns: number;
  last_error?: string;
};

export type SystemTaskStatus = {
  probe: ProbeRuntimeStatus;
  ip_profile: IPProfileRuntimeStatus;
  stale_cleanup: {
    enabled: boolean;
    tracked_candidates: number;
    deleted_last_run: number;
    last_run_at_ns: number;
  };
};

export type UnknownNodesFillResult = {
  matched: number;
  seeded_egress: number;
  queued_egress: number;
  queued_profile: number;
  skipped: number;
  failed: number;
};
