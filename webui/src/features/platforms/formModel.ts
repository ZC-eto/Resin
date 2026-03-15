import { z } from "zod";
import { allocationPolicies, emptyAccountBehaviors, missActions, proxyAccessModes, rotationPolicies } from "./constants";
import { parseHeaderLines, parseLinesToList } from "./formParsers";
import type { NetworkType, Platform, PlatformCreateInput, PlatformUpdateInput } from "./types";

const platformNameForbiddenChars = ".:|/\\@?#%~";
const platformNameForbiddenSpacing = " \t\r\n";
const platformNameReserved = "api";

function containsAny(source: string, chars: string): boolean {
  for (const ch of chars) {
    if (source.includes(ch)) {
      return true;
    }
  }
  return false;
}

export const platformNameRuleHint = "平台名不能包含 .:|/\\@?#%~、空格、Tab、换行、回车，也不能为保留字。";

export const platformFormSchema = z.object({
  name: z.string().trim()
    .min(1, "平台名称不能为空")
    .refine((value) => !containsAny(value, platformNameForbiddenChars), {
      message: "平台名称不能包含字符 .:|/\\@?#%~",
    })
    .refine((value) => !containsAny(value, platformNameForbiddenSpacing), {
      message: "平台名称不能包含空格、Tab、换行、回车",
    })
    .refine((value) => value.toLowerCase() !== platformNameReserved, {
      message: "平台名称不能为保留字",
    }),
  proxy_access_mode: z.enum(proxyAccessModes),
  rotation_policy: z.enum(rotationPolicies),
  rotation_interval: z.string().optional(),
  regex_filters_text: z.string().optional(),
  region_filters_text: z.string().optional(),
  subscription_filters: z.array(z.string()).default([]),
  network_type_filters: z.array(z.enum(["UNKNOWN", "RESIDENTIAL", "DATACENTER", "MOBILE"])).default([]),
  min_quality_score: z.string().optional(),
  max_reference_latency_ms: z.string().optional(),
  min_egress_stability_score: z.string().optional(),
  max_circuit_open_count: z.string().optional(),
  reverse_proxy_miss_action: z.enum(missActions),
  reverse_proxy_empty_account_behavior: z.enum(emptyAccountBehaviors),
  reverse_proxy_fixed_account_header: z.string().optional(),
  allocation_policy: z.enum(allocationPolicies),
}).superRefine((value, ctx) => {
  if (value.rotation_policy === "TTL" && !(value.rotation_interval?.trim())) {
    ctx.addIssue({
      code: "custom",
      path: ["rotation_interval"],
      message: "按时间轮换时必须填写轮换周期",
    });
  }
  if (
    value.reverse_proxy_empty_account_behavior === "FIXED_HEADER" &&
    parseHeaderLines(value.reverse_proxy_fixed_account_header).length === 0
  ) {
    ctx.addIssue({
      code: "custom",
      path: ["reverse_proxy_fixed_account_header"],
      message: "用于提取 Account 的 Headers 不能为空",
    });
  }
});

export type PlatformFormInput = z.input<typeof platformFormSchema>;
export type PlatformFormValues = z.output<typeof platformFormSchema>;

export const defaultPlatformFormValues: PlatformFormInput = {
  name: "",
  proxy_access_mode: "STANDARD",
  rotation_policy: "KEEP",
  rotation_interval: "",
  regex_filters_text: "",
  region_filters_text: "",
  subscription_filters: [],
  network_type_filters: [],
  min_quality_score: "",
  max_reference_latency_ms: "",
  min_egress_stability_score: "",
  max_circuit_open_count: "",
  reverse_proxy_miss_action: "TREAT_AS_EMPTY",
  reverse_proxy_empty_account_behavior: "RANDOM",
  reverse_proxy_fixed_account_header: "Authorization",
  allocation_policy: "BALANCED",
};

export function platformToFormValues(platform: Platform): PlatformFormInput {
  const regexFilters = Array.isArray(platform.regex_filters) ? platform.regex_filters : [];
  const regionFilters = Array.isArray(platform.region_filters) ? platform.region_filters : [];

  return {
    name: platform.name,
    proxy_access_mode: platform.proxy_access_mode,
    rotation_policy: platform.rotation_policy,
    rotation_interval: platform.rotation_interval || platform.sticky_ttl,
    regex_filters_text: regexFilters.join("\n"),
    region_filters_text: regionFilters.join("\n"),
    subscription_filters: Array.isArray(platform.subscription_filters) ? platform.subscription_filters : [],
    network_type_filters: Array.isArray(platform.network_type_filters) ? platform.network_type_filters : [],
    min_quality_score: platform.min_quality_score !== undefined ? String(platform.min_quality_score) : "",
    max_reference_latency_ms: platform.max_reference_latency_ms !== undefined ? String(platform.max_reference_latency_ms) : "",
    min_egress_stability_score: platform.min_egress_stability_score !== undefined ? String(platform.min_egress_stability_score) : "",
    max_circuit_open_count: platform.max_circuit_open_count !== undefined ? String(platform.max_circuit_open_count) : "",
    reverse_proxy_miss_action: platform.reverse_proxy_miss_action,
    reverse_proxy_empty_account_behavior: platform.reverse_proxy_empty_account_behavior,
    reverse_proxy_fixed_account_header: platform.reverse_proxy_fixed_account_header,
    allocation_policy: platform.allocation_policy,
  };
}

function toPlatformPayloadBase(values: PlatformFormValues) {
  return {
    name: values.name.trim(),
    proxy_access_mode: values.proxy_access_mode,
    rotation_policy: values.rotation_policy,
    regex_filters: parseLinesToList(values.regex_filters_text),
    region_filters: parseLinesToList(values.region_filters_text, (value) => value.toLowerCase()),
    subscription_filters: values.subscription_filters,
    network_type_filters: values.network_type_filters as NetworkType[],
    min_quality_score: parseOptionalNumber(values.min_quality_score),
    max_reference_latency_ms: parseOptionalNumber(values.max_reference_latency_ms),
    min_egress_stability_score: parseOptionalNumber(values.min_egress_stability_score),
    max_circuit_open_count: parseOptionalNumber(values.max_circuit_open_count),
    reverse_proxy_miss_action: values.reverse_proxy_miss_action,
    reverse_proxy_empty_account_behavior: values.reverse_proxy_empty_account_behavior,
    reverse_proxy_fixed_account_header: parseHeaderLines(values.reverse_proxy_fixed_account_header).join("\n"),
    allocation_policy: values.allocation_policy,
  };
}

export function toPlatformCreateInput(values: PlatformFormValues): PlatformCreateInput {
  return {
    ...toPlatformPayloadBase(values),
    rotation_interval: values.rotation_policy === "TTL" ? values.rotation_interval?.trim() || undefined : undefined,
  };
}

export function toPlatformUpdateInput(values: PlatformFormValues): PlatformUpdateInput {
  return {
    ...toPlatformPayloadBase(values),
    rotation_interval: values.rotation_policy === "TTL" ? values.rotation_interval?.trim() || "" : undefined,
  };
}

function parseOptionalNumber(value?: string): number | undefined {
  const normalized = value?.trim();
  if (!normalized) {
    return undefined;
  }
  const parsed = Number(normalized);
  if (!Number.isFinite(parsed)) {
    return undefined;
  }
  return parsed;
}
