import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Info, Plus, RefreshCw, Search, Sparkles } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useNavigate } from "react-router-dom";
import { Badge } from "../../components/ui/Badge";
import { Button } from "../../components/ui/Button";
import { Card } from "../../components/ui/Card";
import { Input } from "../../components/ui/Input";
import { OffsetPagination } from "../../components/ui/OffsetPagination";
import { Select } from "../../components/ui/Select";
import { Switch } from "../../components/ui/Switch";
import { Textarea } from "../../components/ui/Textarea";
import { ToastContainer } from "../../components/ui/Toast";
import { useToast } from "../../hooks/useToast";
import { useI18n } from "../../i18n";
import { formatApiErrorMessage } from "../../lib/error-message";
import { formatGoDuration, formatRelativeTime } from "../../lib/time";
import { listSubscriptions } from "../subscriptions/api";
import { createPlatform, listPlatforms } from "./api";
import {
  allocationPolicies,
  allocationPolicyLabel,
  emptyAccountBehaviorLabel,
  emptyAccountBehaviors,
  missActionLabel,
  missActions,
  proxyAccessModeLabel,
  rotationPolicies,
  rotationPolicyLabel,
} from "./constants";
import {
  defaultPlatformFormValues,
  platformFormSchema,
  platformNameRuleHint,
  toPlatformCreateInput,
  type PlatformFormInput,
  type PlatformFormValues,
} from "./formModel";
import type { Platform } from "./types";

const ZERO_UUID = "00000000-0000-0000-0000-000000000000";
const EMPTY_PLATFORMS: Platform[] = [];
const PAGE_SIZE_OPTIONS = [12, 24, 48, 96] as const;
const NETWORK_TYPE_OPTIONS = [
  { value: "RESIDENTIAL", label: "家宽 / 住宅" },
  { value: "DATACENTER", label: "机房" },
  { value: "MOBILE", label: "移动网络" },
  { value: "UNKNOWN", label: "未知" },
] as const;

export function PlatformPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState<number>(24);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const { toasts, showToast, dismissToast } = useToast();

  const queryClient = useQueryClient();
  const formatPlatformMutationError = (error: unknown) => {
    const base = formatApiErrorMessage(error, t);
    if (base.includes("name:")) {
      return `${base}；${t(platformNameRuleHint)}`;
    }
    return base;
  };

  const platformsQuery = useQuery({
    queryKey: ["platforms", "page", page, pageSize, search],
    queryFn: () =>
      listPlatforms({
        limit: pageSize,
        offset: page * pageSize,
        keyword: search,
      }),
    refetchInterval: 30_000,
    placeholderData: (prev) => prev,
  });
  const subscriptionsQuery = useQuery({
    queryKey: ["subscriptions", "all", "platform-create"],
    queryFn: async () => {
      const data = await listSubscriptions({ limit: 100000, offset: 0 });
      return data.items;
    },
    staleTime: 60_000,
  });

  const platforms = platformsQuery.data?.items ?? EMPTY_PLATFORMS;
  const subscriptions = subscriptionsQuery.data ?? [];

  const totalPlatforms = platformsQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(totalPlatforms / pageSize));
  const currentPage = Math.min(page, totalPages - 1);

  const createForm = useForm<PlatformFormInput, unknown, PlatformFormValues>({
    resolver: zodResolver(platformFormSchema),
    defaultValues: defaultPlatformFormValues,
  });
  const createEmptyAccountBehavior = createForm.watch("reverse_proxy_empty_account_behavior");
  const createProxyAccessMode = createForm.watch("proxy_access_mode");
  const createRotationPolicy = createForm.watch("rotation_policy");
  const createSubscriptionFilters = createForm.watch("subscription_filters") ?? [];
  const createNetworkTypeFilters = createForm.watch("network_type_filters") ?? [];

  const createMutation = useMutation({
    mutationFn: createPlatform,
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ["platforms"] });
      setCreateModalOpen(false);
      createForm.reset();
      showToast("success", t("平台 {{name}} 创建成功", { name: created.name }));
      navigate(`/platforms/${created.id}`);
    },
    onError: (error) => {
      showToast("error", formatPlatformMutationError(error));
    },
  });

  const onCreateSubmit = createForm.handleSubmit(async (values) => {
    await createMutation.mutateAsync(toPlatformCreateInput(values));
  });

  const changePageSize = (next: number) => {
    setPageSize(next);
    setPage(0);
  };

  return (
    <section className="platform-page">
      <header className="module-header">
        <div>
          <h2>{t("平台管理")}</h2>
          <p className="module-description">{t("集中维护平台策略与节点分配规则。")}</p>
        </div>
      </header>

      <ToastContainer toasts={toasts} onDismiss={dismissToast} />

      <Card className="platform-list-card platform-directory-card">
        <div className="list-card-header">
          <div>
            <h3>{t("平台列表")}</h3>
            <p>{t("共 {{count}} 个平台", { count: totalPlatforms })}</p>
          </div>
          <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
            <label className="search-box" htmlFor="platform-search" style={{ maxWidth: 200, margin: 0, gap: 6 }}>
              <Search size={16} />
              <Input
                id="platform-search"
                placeholder={t("搜索平台")}
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value);
                  setPage(0);
                }}
                style={{ padding: "6px 10px", borderRadius: 8 }}
              />
            </label>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setCreateModalOpen(true)}
            >
              <Plus size={16} />
              {t("新建")}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => platformsQuery.refetch()}
              disabled={platformsQuery.isFetching}
            >
              <RefreshCw size={16} className={platformsQuery.isFetching ? "spin" : undefined} />
              {t("刷新")}
            </Button>
          </div>
        </div>
      </Card>

      <Card className="platform-cards-container">
        {platformsQuery.isLoading ? <p className="muted">{t("正在加载平台数据...")}</p> : null}

        {platformsQuery.isError ? (
          <div className="callout callout-error">
            <AlertTriangle size={14} />
            <span>{formatApiErrorMessage(platformsQuery.error, t)}</span>
          </div>
        ) : null}

        {!platformsQuery.isLoading && !platforms.length ? (
          <div className="empty-box">
            <Sparkles size={16} />
            <p>{t("没有匹配的平台")}</p>
          </div>
        ) : null}

        <div className="platform-card-grid">
          {platforms.map((platform) => {
            const regionCount = platform.region_filters.length;
            const regexCount = platform.regex_filters.length;
            const rotationInterval =
              platform.rotation_policy === "KEEP"
                ? t("不自动轮换")
                : formatGoDuration(platform.rotation_interval || platform.sticky_ttl, t("默认"));

            return (
              <button
                key={platform.id}
                type="button"
                className="platform-tile"
                onClick={() => navigate(`/platforms/${platform.id}`)}
              >
                <div className="platform-tile-head">
                  <p>{platform.name}</p>
                  <Badge variant={platform.id === ZERO_UUID ? "warning" : "success"}>
                    {platform.id === ZERO_UUID ? t("内置平台") : t("自定义平台")}
                  </Badge>
                </div>
                <div className="platform-tile-facts">
                  <span className="platform-fact">
                    <span>{t("区域")}</span>
                    <strong>{regionCount}</strong>
                  </span>
                  <span className="platform-fact">
                    <span>{t("正则")}</span>
                    <strong>{regexCount}</strong>
                  </span>
                  <span className="platform-fact">
                    <span>{t("轮换策略")}</span>
                    <strong>{t(rotationPolicyLabel[platform.rotation_policy])}</strong>
                  </span>
                  <span className="platform-fact">
                    <span>{t("轮换周期")}</span>
                    <strong>{rotationInterval}</strong>
                  </span>
                  <span className="platform-fact">
                    <span>{t("接入模式")}</span>
                    <strong>{t(proxyAccessModeLabel[platform.proxy_access_mode])}</strong>
                  </span>
                </div>
                <div className="platform-tile-foot">
                  <span className="platform-tile-meta">
                    {t("{{count}} 个可用节点", { count: platform.routable_node_count })}
                  </span>
                  <span className="platform-tile-meta platform-tile-updated">
                    {t("更新于 {{time}}", { time: formatRelativeTime(platform.updated_at) })}
                  </span>
                </div>
              </button>
            );
          })}
        </div>

        <OffsetPagination
          page={currentPage}
          totalPages={totalPages}
          totalItems={totalPlatforms}
          pageSize={pageSize}
          pageSizeOptions={PAGE_SIZE_OPTIONS}
          onPageChange={setPage}
          onPageSizeChange={changePageSize}
        />
      </Card>

      {createModalOpen ? (
        <div className="modal-overlay" role="dialog" aria-modal="true">
          <Card className="modal-card">
            <div className="modal-header">
              <h3>{t("新建平台")}</h3>
              <Button variant="ghost" size="sm" onClick={() => setCreateModalOpen(false)}>
                {t("关闭")}
              </Button>
            </div>

            <form className="form-grid" onSubmit={onCreateSubmit}>
              <div className="field-group">
                <label className="field-label" htmlFor="create-name">
                  {t("名称")}
                </label>
                <Input id="create-name" invalid={Boolean(createForm.formState.errors.name)} {...createForm.register("name")} />
                {createForm.formState.errors.name?.message ? (
                  <p className="field-error">{t(createForm.formState.errors.name.message)}</p>
                ) : null}
                <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                  {t(platformNameRuleHint)}
                </p>
              </div>

              <div className="field-group">
                <input type="hidden" {...createForm.register("proxy_access_mode")} />
                <label className="field-label" htmlFor="create-proxy-access-mode" style={{ visibility: "hidden" }}>
                  {t("粘性代理模式")}
                </label>
                <div className="subscription-switch-item">
                  <label className="subscription-switch-label" htmlFor="create-proxy-access-mode">
                    <span>{t("粘性代理模式")}</span>
                    <span className="muted">{t("开启后默认导出 Platform.Account 的粘性代理地址。")}</span>
                  </label>
                  <Switch
                    id="create-proxy-access-mode"
                    checked={createProxyAccessMode === "STICKY"}
                    onChange={(event) => {
                      createForm.setValue("proxy_access_mode", event.target.checked ? "STICKY" : "STANDARD", {
                        shouldDirty: true,
                      });
                    }}
                  />
                </div>
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-rotation-policy">
                  {t("轮换策略")}
                </label>
                <Select id="create-rotation-policy" {...createForm.register("rotation_policy")}>
                  {rotationPolicies.map((item) => (
                    <option key={item} value={item}>
                      {t(rotationPolicyLabel[item])}
                    </option>
                  ))}
                </Select>
                <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                  {createRotationPolicy === "KEEP"
                    ? t("保持当前出口，直到故障或手动切换。")
                    : t("到达轮换周期后，下次请求会重新分配出口。")}
                </p>
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-rotation-interval">
                  {t("轮换周期")}
                </label>
                <Input
                  id="create-rotation-interval"
                  placeholder={t("例如 2m / 30m / 2h")}
                  disabled={createRotationPolicy !== "TTL"}
                  invalid={Boolean(createForm.formState.errors.rotation_interval)}
                  {...createForm.register("rotation_interval")}
                />
                {createForm.formState.errors.rotation_interval?.message ? (
                  <p className="field-error">{t(createForm.formState.errors.rotation_interval.message)}</p>
                ) : null}
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-miss-action">
                  {t("反向代理账号解析出错策略")}
                </label>
                <Select id="create-miss-action" {...createForm.register("reverse_proxy_miss_action")}>
                  {missActions.map((item) => (
                    <option key={item} value={item}>
                      {t(missActionLabel[item])}
                    </option>
                  ))}
                </Select>
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-policy">
                  {t("节点分配策略")}
                </label>
                <Select id="create-policy" {...createForm.register("allocation_policy")}>
                  {allocationPolicies.map((item) => (
                    <option key={item} value={item}>
                      {t(allocationPolicyLabel[item])}
                    </option>
                  ))}
                </Select>
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-empty-account-behavior">
                  {t("反向代理账号为空行为")}
                </label>
                <Select id="create-empty-account-behavior" {...createForm.register("reverse_proxy_empty_account_behavior")}>
                  {emptyAccountBehaviors.map((item) => (
                    <option key={item} value={item}>
                      {t(emptyAccountBehaviorLabel[item])}
                    </option>
                  ))}
                </Select>
              </div>

              <div
                className={`account-headers-collapse ${createEmptyAccountBehavior === "FIXED_HEADER" ? "account-headers-collapse-open" : ""}`}
                aria-hidden={createEmptyAccountBehavior !== "FIXED_HEADER"}
              >
                <div className="field-group">
                  <label className="field-label" htmlFor="create-fixed-account-header">
                    {t("用于提取 Account 的 Headers（每行一个）")}
                  </label>
                  <Textarea
                    id="create-fixed-account-header"
                    rows={3}
                    placeholder={t("每行一个，例如 Authorization 或 X-Account-Id")}
                    {...createForm.register("reverse_proxy_fixed_account_header")}
                  />
                  {createForm.formState.errors.reverse_proxy_fixed_account_header?.message ? (
                    <p className="field-error">{t(createForm.formState.errors.reverse_proxy_fixed_account_header.message)}</p>
                  ) : null}
                </div>
              </div>

              <div className="field-group">
                <label className="field-label field-label-with-info" htmlFor="create-regex">
                  <span>{t("节点名正则过滤规则（可选）")}</span>
                  <span
                    className="subscription-info-icon"
                    title={t("满足所有正则表达式的节点才会被选择")}
                    aria-label={t("满足所有正则表达式的节点才会被选择")}
                    tabIndex={0}
                  >
                    <Info size={13} />
                  </span>
                </label>
                <Textarea
                  id="create-regex"
                  rows={4}
                  placeholder={t("每行一条，例如 .*专线.* 或 <订阅名>/.*")}
                  {...createForm.register("regex_filters_text")}
                />
                <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                  {t("技巧：<订阅名>/.* 可筛选来自该订阅的节点。")}
                </p>
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-region">
                  {t("地区过滤规则（可选）")}
                </label>
                <Textarea id="create-region" rows={4} placeholder={t("每行一条，如 hk / us")} {...createForm.register("region_filters_text")} />
              </div>

              <div className="field-group">
                <label className="field-label">{t("订阅来源过滤（可选）")}</label>
                <div className="syscfg-checkbox-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
                  {subscriptions.map((subscription) => {
                    const checked = createSubscriptionFilters.includes(subscription.id);
                    return (
                      <label key={subscription.id} style={{ display: "flex", gap: 8, alignItems: "center" }}>
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={(event) => {
                            const next = event.target.checked
                              ? [...createSubscriptionFilters, subscription.id]
                              : createSubscriptionFilters.filter((value) => value !== subscription.id);
                            createForm.setValue("subscription_filters", next, { shouldDirty: true });
                          }}
                        />
                        <span>{subscription.name}</span>
                      </label>
                    );
                  })}
                </div>
                <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                  {t("不勾选表示允许所有订阅来源。")}
                </p>
              </div>

              <div className="field-group">
                <label className="field-label">{t("网络类型过滤（可选）")}</label>
                <div className="syscfg-checkbox-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
                  {NETWORK_TYPE_OPTIONS.map((option) => {
                    const checked = createNetworkTypeFilters.includes(option.value);
                    return (
                      <label key={option.value} style={{ display: "flex", gap: 8, alignItems: "center" }}>
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={(event) => {
                            const next = event.target.checked
                              ? [...createNetworkTypeFilters, option.value]
                              : createNetworkTypeFilters.filter((value) => value !== option.value);
                            createForm.setValue("network_type_filters", next, { shouldDirty: true });
                          }}
                        />
                        <span>{t(option.label)}</span>
                      </label>
                    );
                  })}
                </div>
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-min-quality-score">
                  {t("最低质量分（可选）")}
                </label>
                <Input id="create-min-quality-score" type="number" min={0} max={100} {...createForm.register("min_quality_score")} />
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-max-reference-latency">
                  {t("最大参考延迟 (ms)")}</label>
                <Input id="create-max-reference-latency" type="number" min={0} {...createForm.register("max_reference_latency_ms")} />
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-min-stability-score">
                  {t("最低出口稳定性分")}
                </label>
                <Input id="create-min-stability-score" type="number" min={0} max={20} {...createForm.register("min_egress_stability_score")} />
              </div>

              <div className="field-group">
                <label className="field-label" htmlFor="create-max-circuit-open-count">
                  {t("最大累计熔断次数")}
                </label>
                <Input id="create-max-circuit-open-count" type="number" min={0} {...createForm.register("max_circuit_open_count")} />
              </div>

              <div className="detail-actions">
                <Button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? t("创建中...") : t("确认创建")}
                </Button>
                <Button variant="secondary" onClick={() => setCreateModalOpen(false)}>
                  {t("取消")}
                </Button>
              </div>
            </form>
          </Card>
        </div>
      ) : null}
    </section>
  );
}
