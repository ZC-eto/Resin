export type SubscriptionSource = {
  id: string;
  label: string;
  type: "remote" | "local";
  url: string;
  content: string;
  enabled: boolean;
};

export type Subscription = {
  id: string;
  name: string;
  source_type: "remote" | "local";
  url: string;
  content: string;
  sources: SubscriptionSource[];
  update_interval: string;
  node_count: number;
  healthy_node_count: number;
  residential_node_count: number;
  datacenter_node_count: number;
  mobile_node_count: number;
  unknown_node_count: number;
  pending_egress_node_count: number;
  pending_profile_node_count: number;
  profiled_unknown_node_count: number;
  average_quality_score?: number;
  ephemeral: boolean;
  ephemeral_node_evict_delay: string;
  enabled: boolean;
  created_at: string;
  last_checked?: string;
  last_updated?: string;
  last_error?: string;
};

export type SubscriptionFillUnknownNodesResult = {
  matched: number;
  queued_egress: number;
  queued_profile: number;
  skipped: number;
  failed: number;
};

export type PageResponse<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type SubscriptionCreateInput = {
  name: string;
  sources?: SubscriptionSource[];
  source_type?: "remote" | "local";
  url?: string;
  content?: string;
  update_interval?: string;
  enabled?: boolean;
  ephemeral?: boolean;
  ephemeral_node_evict_delay?: string;
};

export type SubscriptionUpdateInput = {
  name?: string;
  sources?: SubscriptionSource[];
  url?: string;
  content?: string;
  update_interval?: string;
  enabled?: boolean;
  ephemeral?: boolean;
  ephemeral_node_evict_delay?: string;
};
