import type {
  PlatformAllocationPolicy,
  PlatformEmptyAccountBehavior,
  PlatformMissAction,
  PlatformProxyAccessMode,
  PlatformRotationPolicy,
} from "./types";

export const allocationPolicies: PlatformAllocationPolicy[] = [
  "BALANCED",
  "PREFER_LOW_LATENCY",
  "PREFER_IDLE_IP",
];

export const missActions: PlatformMissAction[] = ["TREAT_AS_EMPTY", "REJECT"];

export const emptyAccountBehaviors: PlatformEmptyAccountBehavior[] = [
  "RANDOM",
  "FIXED_HEADER",
  "ACCOUNT_HEADER_RULE",
];

export const proxyAccessModes: PlatformProxyAccessMode[] = ["STANDARD", "STICKY"];
export const rotationPolicies: PlatformRotationPolicy[] = ["KEEP", "TTL"];

export const allocationPolicyLabel: Record<PlatformAllocationPolicy, string> = {
  BALANCED: "均衡",
  PREFER_LOW_LATENCY: "优先低延迟",
  PREFER_IDLE_IP: "优先空闲出口 IP",
};

export const missActionLabel: Record<PlatformMissAction, string> = {
  TREAT_AS_EMPTY: "按空账号处理",
  REJECT: "拒绝代理请求",
};

export const emptyAccountBehaviorLabel: Record<PlatformEmptyAccountBehavior, string> = {
  RANDOM: "随机路由",
  FIXED_HEADER: "提取指定请求头作为 Account",
  ACCOUNT_HEADER_RULE: "按照全局请求头规则提取 Account",
};

export const proxyAccessModeLabel: Record<PlatformProxyAccessMode, string> = {
  STANDARD: "普通代理",
  STICKY: "粘性代理",
};

export const rotationPolicyLabel: Record<PlatformRotationPolicy, string> = {
  KEEP: "保持原出口",
  TTL: "按时间轮换",
};
