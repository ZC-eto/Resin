import { z } from "zod";
import { allocationPolicies, emptyAccountBehaviors, missActions, proxyAccessModes, rotationPolicies } from "./constants";
import { parseHeaderLines, parseLinesToList } from "./formParsers";
import type { Platform, PlatformCreateInput, PlatformUpdateInput } from "./types";

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

export type PlatformFormValues = z.infer<typeof platformFormSchema>;

export const defaultPlatformFormValues: PlatformFormValues = {
  name: "",
  proxy_access_mode: "STANDARD",
  rotation_policy: "KEEP",
  rotation_interval: "",
  regex_filters_text: "",
  region_filters_text: "",
  reverse_proxy_miss_action: "TREAT_AS_EMPTY",
  reverse_proxy_empty_account_behavior: "RANDOM",
  reverse_proxy_fixed_account_header: "Authorization",
  allocation_policy: "BALANCED",
};

export function platformToFormValues(platform: Platform): PlatformFormValues {
  const regexFilters = Array.isArray(platform.regex_filters) ? platform.regex_filters : [];
  const regionFilters = Array.isArray(platform.region_filters) ? platform.region_filters : [];

  return {
    name: platform.name,
    proxy_access_mode: platform.proxy_access_mode,
    rotation_policy: platform.rotation_policy,
    rotation_interval: platform.rotation_interval || platform.sticky_ttl,
    regex_filters_text: regexFilters.join("\n"),
    region_filters_text: regionFilters.join("\n"),
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
    rotation_interval: values.rotation_policy === "TTL" ? values.rotation_interval?.trim() || "" : "",
  };
}
