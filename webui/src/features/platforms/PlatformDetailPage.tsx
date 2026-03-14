import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, Info, Link2, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Badge } from "../../components/ui/Badge";
import { Button } from "../../components/ui/Button";
import { Card } from "../../components/ui/Card";
import { Input } from "../../components/ui/Input";
import { Select } from "../../components/ui/Select";
import { Switch } from "../../components/ui/Switch";
import { Textarea } from "../../components/ui/Textarea";
import { ToastContainer } from "../../components/ui/Toast";
import { useToast } from "../../hooks/useToast";
import { useI18n } from "../../i18n";
import { formatApiErrorMessage } from "../../lib/error-message";
import { formatGoDuration, formatRelativeTime } from "../../lib/time";
import { getEnvConfig } from "../systemConfig/api";
import { clearAllPlatformLeases, deletePlatform, getPlatform, resetPlatform, rotatePlatformLease, updatePlatform } from "./api";
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
  platformToFormValues,
  toPlatformUpdateInput,
  type PlatformFormValues,
} from "./formModel";
import { PlatformMonitorPanel } from "./PlatformMonitorPanel";

type PlatformDetailTab = "monitor" | "config" | "ops";

const ZERO_UUID = "00000000-0000-0000-0000-000000000000";
const DETAIL_TABS: Array<{ key: PlatformDetailTab; label: string; hint: string }> = [
  { key: "monitor", label: "监控", hint: "平台运行态趋势和快照" },
  { key: "config", label: "配置", hint: "过滤规则与分配策略" },
  { key: "ops", label: "运维", hint: "重置、清租约、删除操作" },
];

function normalizeProxyHost(host: string): string {
  const trimmed = host.trim();
  return trimmed || "127.0.0.1";
}

function normalizeProxyPort(port: string): string {
  const trimmed = port.trim();
  return trimmed || "2260";
}

function buildProxyAuthority(host: string, port: string): string {
  return `${normalizeProxyHost(host)}:${normalizeProxyPort(port)}`;
}

function buildForwardProxyURL(platformName: string, token: string, authority: string): string {
  return `http://${platformName}:${token}@${authority}`;
}

function buildStickyProxyURL(platformName: string, account: string, token: string, authority: string): string {
  return `http://${platformName}.${account}:${token}@${authority}`;
}

function buildStickyTemplateURL(platformName: string, token: string, authority: string): string {
  return `http://${platformName}.{account}:${token}@${authority}`;
}

function buildRotateCurlCommand(rotateAPIURL: string, account: string): string {
  const normalizedAccount = account.trim() || "account_001";
  return `curl -X POST "${rotateAPIURL}" -H "Content-Type: application/json" -d "{\\"account\\":\\"${normalizedAccount}\\"}"`;
}

export function PlatformDetailPage() {
  const { t } = useI18n();
  const { platformId = "" } = useParams();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<PlatformDetailTab>("monitor");
  const [exportHost, setExportHost] = useState("");
  const [exportPort, setExportPort] = useState("");
  const [exportToken, setExportToken] = useState("my-token");
  const [exportAccount, setExportAccount] = useState("account_001");
  const [rotateAccount, setRotateAccount] = useState("");
  const { toasts, showToast, dismissToast } = useToast();
  const queryClient = useQueryClient();
  const formatPlatformMutationError = (error: unknown) => {
    const base = formatApiErrorMessage(error, t);
    if (base.includes("name:")) {
      return `${base}；${t(platformNameRuleHint)}`;
    }
    return base;
  };

  const platformQuery = useQuery({
    queryKey: ["platform", platformId],
    queryFn: () => getPlatform(platformId),
    enabled: Boolean(platformId),
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  });
  const envConfigQuery = useQuery({
    queryKey: ["system-env-config", "proxy-export"],
    queryFn: getEnvConfig,
  });

  const platform = platformQuery.data ?? null;

  const editForm = useForm<PlatformFormValues>({
    resolver: zodResolver(platformFormSchema),
    defaultValues: defaultPlatformFormValues,
  });
  const detailEmptyAccountBehavior = editForm.watch("reverse_proxy_empty_account_behavior");
  const detailProxyAccessMode = editForm.watch("proxy_access_mode");
  const detailRotationPolicy = editForm.watch("rotation_policy");

  useEffect(() => {
    if (!platform) {
      return;
    }
    editForm.reset(platformToFormValues(platform));
  }, [platform, editForm]);

  useEffect(() => {
    const nextHost = typeof window !== "undefined" ? window.location.hostname : "";
    if (!exportHost && nextHost) {
      setExportHost(nextHost);
    }
  }, [exportHost]);

  useEffect(() => {
    const nextPort = envConfigQuery.data?.resin_port ? String(envConfigQuery.data.resin_port) : "";
    if (!exportPort && nextPort) {
      setExportPort(nextPort);
    }
  }, [envConfigQuery.data, exportPort]);

  useEffect(() => {
    if (!rotateAccount && exportAccount) {
      setRotateAccount(exportAccount);
    }
  }, [exportAccount, rotateAccount]);

  const invalidatePlatform = async (id: string) => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["platforms"] }),
      queryClient.invalidateQueries({ queryKey: ["platform", id] }),
    ]);
  };

  const updateMutation = useMutation({
    mutationFn: async (formData: PlatformFormValues) => {
      if (!platform) {
        throw new Error("平台不存在或已被删除");
      }

      return updatePlatform(platform.id, toPlatformUpdateInput(formData));
    },
    onSuccess: async (updated) => {
      await invalidatePlatform(updated.id);
      editForm.reset(platformToFormValues(updated));
      showToast("success", t("平台 {{name}} 已更新", { name: updated.name }));
    },
    onError: (error) => {
      showToast("error", formatPlatformMutationError(error));
    },
  });

  const resetMutation = useMutation({
    mutationFn: async () => {
      if (!platform) {
        throw new Error("平台不存在或已被删除");
      }
      return resetPlatform(platform.id);
    },
    onSuccess: async (updated) => {
      await invalidatePlatform(updated.id);
      editForm.reset(platformToFormValues(updated));
      showToast("success", t("平台 {{name}} 已重置为默认配置", { name: updated.name }));
    },
    onError: (error) => {
      showToast("error", formatPlatformMutationError(error));
    },
  });

  const clearLeasesMutation = useMutation({
    mutationFn: async () => {
      if (!platform) {
        throw new Error("平台不存在或已被删除");
      }
      await clearAllPlatformLeases(platform.id);
      return platform;
    },
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({ queryKey: ["platform-monitor"] });
      showToast("success", t("平台 {{name}} 的所有租约已清除", { name: updated.name }));
    },
    onError: (error) => {
      showToast("error", formatApiErrorMessage(error, t));
    },
  });

  const rotateLeaseMutation = useMutation({
    mutationFn: async () => {
      if (!platform) {
        throw new Error("平台不存在或已被删除");
      }
      await rotatePlatformLease(platform.id, rotateAccount.trim());
      return platform;
    },
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({ queryKey: ["platform-monitor"] });
      showToast("success", t("平台 {{name}} 的账号 {{account}} 已触发换 IP", { name: updated.name, account: rotateAccount.trim() }));
    },
    onError: (error) => {
      showToast("error", formatApiErrorMessage(error, t));
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      if (!platform) {
        throw new Error("平台不存在或已被删除");
      }
      await deletePlatform(platform.id);
      return platform;
    },
    onSuccess: async (deleted) => {
      await queryClient.invalidateQueries({ queryKey: ["platforms"] });
      showToast("success", t("平台 {{name}} 已删除", { name: deleted.name }));
      navigate("/platforms", { replace: true });
    },
    onError: (error) => {
      showToast("error", formatApiErrorMessage(error, t));
    },
  });

  const onEditSubmit = editForm.handleSubmit(async (values) => {
    await updateMutation.mutateAsync(values);
  });

  const handleDelete = async () => {
    if (!platform) {
      return;
    }
    if (platform.id === ZERO_UUID) {
      return;
    }
    const confirmed = window.confirm(t("确认删除平台 {{name}}？该操作不可撤销。", { name: platform.name }));
    if (!confirmed) {
      return;
    }
    await deleteMutation.mutateAsync();
  };

  const handleClearAllLeases = async () => {
    if (!platform) {
      return;
    }
    const confirmed = window.confirm(t("确认清除平台 {{name}} 的所有租约？", { name: platform.name }));
    if (!confirmed) {
      return;
    }
    await clearLeasesMutation.mutateAsync();
  };

  const copyText = async (value: string, successMessage: string) => {
    try {
      if (!navigator?.clipboard?.writeText) {
        throw new Error("clipboard unavailable");
      }
      await navigator.clipboard.writeText(value);
      showToast("success", successMessage);
    } catch {
      showToast("error", t("当前浏览器无法直接复制，请手动复制文本。"));
    }
  };

  const handleRotateLease = async () => {
    if (!platform) {
      return;
    }
    if (!rotateAccount.trim()) {
      showToast("error", t("请输入需要切换的账号"));
      return;
    }
    await rotateLeaseMutation.mutateAsync();
  };

  const rotationInterval = platform
    ? platform.rotation_policy === "KEEP"
      ? t("不自动轮换")
      : formatGoDuration(platform.rotation_interval || platform.sticky_ttl, t("默认"))
    : t("默认");
  const regionCount = platform?.region_filters.length ?? 0;
  const regexCount = platform?.regex_filters.length ?? 0;
  const deleteDisabled = !platform || platform.id === ZERO_UUID || deleteMutation.isPending;
  const authority = buildProxyAuthority(exportHost, exportPort);
  const plainProxyURL = platform ? buildForwardProxyURL(platform.name, exportToken, authority) : "";
  const stickyProxyURL = platform ? buildStickyProxyURL(platform.name, exportAccount, exportToken, authority) : "";
  const stickyTemplateURL = platform ? buildStickyTemplateURL(platform.name, exportToken, authority) : "";
  const rotateAPIURL =
    platform && exportToken
      ? `http://${authority}/${exportToken}/api/v1/${platform.name}/actions/rotate-lease`
      : "";
  const rotateCurlCommand = rotateAPIURL ? buildRotateCurlCommand(rotateAPIURL, exportAccount) : "";

  return (
    <section className="platform-page platform-detail-page">
      <header className="module-header">
        <div>
          <h2>{t("平台详情")}</h2>
          <p className="module-description">{t("调整当前平台策略，并执行维护操作。")}</p>
        </div>
        <div className="platform-detail-toolbar">
          <Button variant="secondary" size="sm" onClick={() => navigate("/platforms")}>
            <ArrowLeft size={16} />
            {t("返回列表")}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => platformQuery.refetch()} disabled={!platformId || platformQuery.isFetching}>
            <RefreshCw size={16} className={platformQuery.isFetching ? "spin" : undefined} />
            {t("刷新")}
          </Button>
        </div>
      </header>

      <ToastContainer toasts={toasts} onDismiss={dismissToast} />

      {!platformId ? (
        <div className="callout callout-error">
          <AlertTriangle size={14} />
          <span>{t("平台 ID 缺失，无法加载详情。")}</span>
        </div>
      ) : null}

      {platformQuery.isError && !platform ? (
        <div className="callout callout-error">
          <AlertTriangle size={14} />
          <span>{formatApiErrorMessage(platformQuery.error, t)}</span>
        </div>
      ) : null}

      {platformQuery.isLoading && !platform ? (
        <Card className="platform-cards-container">
          <p className="muted">{t("正在加载平台详情...")}</p>
        </Card>
      ) : null}

      {platform ? (
        <>
          <Card className="platform-directory-card platform-detail-header-card">
            <div className="platform-detail-header-main">
              <div>
                <h3>{platform.name}</h3>
                <p>{platform.id}</p>
              </div>
              <div className="platform-detail-header-meta">
                <Badge variant={platform.id === ZERO_UUID ? "warning" : "success"}>
                  {platform.id === ZERO_UUID ? t("内置平台") : t("自定义平台")}
                </Badge>
                <span>{t("更新于 {{time}}", { time: formatRelativeTime(platform.updated_at) })}</span>
              </div>
            </div>
            <div className="platform-detail-header-footer">
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
                  <span>{t("策略")}</span>
                  <strong>{t(allocationPolicyLabel[platform.allocation_policy])}</strong>
                </span>
                <span className="platform-fact">
                  <span>{t("接入模式")}</span>
                  <strong>{t(proxyAccessModeLabel[platform.proxy_access_mode])}</strong>
                </span>
                <span className="platform-fact">
                  <span>{t("未命中策略")}</span>
                  <strong>{t(missActionLabel[platform.reverse_proxy_miss_action])}</strong>
                </span>
                <span className="platform-fact">
                  <span>{t("空账号行为")}</span>
                  <strong>{t(emptyAccountBehaviorLabel[platform.reverse_proxy_empty_account_behavior])}</strong>
                </span>
              </div>
              <Link to={`/nodes?platform_id=${encodeURIComponent(platform.id)}`} className="platform-detail-node-link">
                <Link2 size={14} />
                <span>{t("可路由节点")}</span>
              </Link>
            </div>
          </Card>

          <Card className="platform-cards-container platform-detail-main-card">
            <div className="platform-detail-tabs" role="tablist" aria-label={t("平台详情板块")}>
              {DETAIL_TABS.map((tab) => {
                const selected = activeTab === tab.key;
                return (
                  <button
                    key={tab.key}
                    id={`platform-tab-${tab.key}`}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    aria-controls={`platform-tabpanel-${tab.key}`}
                    className={`platform-detail-tab ${selected ? "platform-detail-tab-active" : ""}`}
                    title={t(tab.hint)}
                    onClick={() => setActiveTab(tab.key)}
                  >
                    <span>{t(tab.label)}</span>
                  </button>
                );
              })}
            </div>

            {activeTab === "monitor" ? (
              <div
                id="platform-tabpanel-monitor"
                role="tabpanel"
                aria-labelledby="platform-tab-monitor"
                className="platform-detail-panel"
              >
                <PlatformMonitorPanel platform={platform} />
              </div>
            ) : null}

            {activeTab === "config" ? (
              <section
                id="platform-tabpanel-config"
                role="tabpanel"
                aria-labelledby="platform-tab-config"
                className="platform-detail-tabpanel"
              >
                <div className="platform-drawer-section-head">
                  <h4>{t("平台配置")}</h4>
                  <p>{t("修改过滤策略与路由策略后点击保存。")}</p>
                </div>

                <form className="form-grid platform-config-form" onSubmit={onEditSubmit}>
                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-name">
                      {t("名称")}
                    </label>
                    <Input id="detail-edit-name" invalid={Boolean(editForm.formState.errors.name)} {...editForm.register("name")} />
                    {editForm.formState.errors.name?.message ? (
                      <p className="field-error">{t(editForm.formState.errors.name.message)}</p>
                    ) : null}
                    <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                      {t(platformNameRuleHint)}
                    </p>
                  </div>

                  <div className="field-group">
                    <input type="hidden" {...editForm.register("proxy_access_mode")} />
                    <label className="field-label" htmlFor="detail-edit-proxy-access-mode" style={{ visibility: "hidden" }}>
                      {t("粘性代理模式")}
                    </label>
                    <div className="subscription-switch-item platform-mode-switch-item">
                      <label className="subscription-switch-label platform-switch-label" htmlFor="detail-edit-proxy-access-mode">
                        <span className="platform-switch-title">{t("粘性代理模式")}</span>
                        <span className="muted platform-switch-hint">
                          {detailProxyAccessMode === "STICKY"
                            ? t("当前平台默认导出粘性代理地址，适合固定账号长期复用。")
                            : t("当前平台默认导出普通代理地址，适合外部服务直接填入。")}
                        </span>
                      </label>
                      <Switch
                        id="detail-edit-proxy-access-mode"
                        checked={detailProxyAccessMode === "STICKY"}
                        onChange={(event) => {
                          editForm.setValue("proxy_access_mode", event.target.checked ? "STICKY" : "STANDARD", {
                            shouldDirty: true,
                          });
                        }}
                      />
                    </div>
                  </div>

                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-rotation-policy">
                      {t("轮换策略")}
                    </label>
                    <Select id="detail-edit-rotation-policy" {...editForm.register("rotation_policy")}>
                      {rotationPolicies.map((item) => (
                        <option key={item} value={item}>
                          {t(rotationPolicyLabel[item])}
                        </option>
                      ))}
                    </Select>
                    <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                      {detailRotationPolicy === "KEEP"
                        ? t("保持当前出口，直到故障或手动切换。")
                        : t("到达轮换周期后，下次请求会重新分配出口。")}
                    </p>
                  </div>

                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-rotation-interval">
                      {t("轮换周期")}
                    </label>
                    <Input
                      id="detail-edit-rotation-interval"
                      placeholder={t("例如 2m / 30m / 2h")}
                      disabled={detailRotationPolicy !== "TTL"}
                      invalid={Boolean(editForm.formState.errors.rotation_interval)}
                      {...editForm.register("rotation_interval")}
                    />
                    {editForm.formState.errors.rotation_interval?.message ? (
                      <p className="field-error">{t(editForm.formState.errors.rotation_interval.message)}</p>
                    ) : null}
                  </div>

                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-miss-action">
                      {t("反向代理账号解析出错策略")}
                    </label>
                    <Select id="detail-edit-miss-action" {...editForm.register("reverse_proxy_miss_action")}>
                      {missActions.map((item) => (
                        <option key={item} value={item}>
                          {t(missActionLabel[item])}
                        </option>
                      ))}
                    </Select>
                  </div>

                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-policy">
                      {t("节点分配策略")}
                    </label>
                    <Select id="detail-edit-policy" {...editForm.register("allocation_policy")}>
                      {allocationPolicies.map((item) => (
                        <option key={item} value={item}>
                          {t(allocationPolicyLabel[item])}
                        </option>
                      ))}
                    </Select>
                  </div>

                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-empty-account-behavior">
                      {t("反向代理账号为空行为")}
                    </label>
                    <Select id="detail-edit-empty-account-behavior" {...editForm.register("reverse_proxy_empty_account_behavior")}>
                      {emptyAccountBehaviors.map((item) => (
                        <option key={item} value={item}>
                          {t(emptyAccountBehaviorLabel[item])}
                        </option>
                      ))}
                    </Select>
                  </div>

                  <div
                    className={`account-headers-collapse ${detailEmptyAccountBehavior === "FIXED_HEADER" ? "account-headers-collapse-open" : ""}`}
                    aria-hidden={detailEmptyAccountBehavior !== "FIXED_HEADER"}
                  >
                    <div className="field-group">
                      <label className="field-label" htmlFor="detail-edit-fixed-account-header">
                        {t("用于提取 Account 的 Headers（每行一个）")}
                      </label>
                      <Textarea
                        id="detail-edit-fixed-account-header"
                        rows={4}
                        placeholder={t("每行一个，例如 Authorization 或 X-Account-Id")}
                        {...editForm.register("reverse_proxy_fixed_account_header")}
                      />
                      {editForm.formState.errors.reverse_proxy_fixed_account_header?.message ? (
                        <p className="field-error">{t(editForm.formState.errors.reverse_proxy_fixed_account_header.message)}</p>
                      ) : null}
                    </div>
                  </div>

                  <div className="field-group">
                    <label className="field-label field-label-with-info" htmlFor="detail-edit-regex">
                      <span>{t("节点名正则过滤规则")}</span>
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
                      id="detail-edit-regex"
                      rows={6}
                      placeholder={t("每行一条，例如 .*专线.* 或 <订阅名>/.*")}
                      {...editForm.register("regex_filters_text")}
                    />
                    <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>
                      {t("技巧：<订阅名>/.* 可筛选来自该订阅的节点。")}
                    </p>
                  </div>

                  <div className="field-group">
                    <label className="field-label" htmlFor="detail-edit-region">
                      {t("地区过滤规则")}
                    </label>
                    <Textarea
                      id="detail-edit-region"
                      rows={6}
                      placeholder={t("每行一条，如 hk / us")}
                      {...editForm.register("region_filters_text")}
                    />
                  </div>

                  <div className="platform-config-actions">
                    <Button type="submit" disabled={updateMutation.isPending}>
                      {updateMutation.isPending ? t("保存中...") : t("保存配置")}
                    </Button>
                  </div>
                </form>

                <div className="platform-export-panel">
                  <div className="platform-drawer-section-head">
                    <h4>{t("接入模板")}</h4>
                    <p>{t("直接复制到浏览器、脚本或外部服务里使用。")}</p>
                  </div>

                  <div className="platform-export-grid">
                    <div className="field-group">
                      <label className="field-label" htmlFor="platform-export-host">
                        {t("代理域名或地址")}
                      </label>
                      <Input id="platform-export-host" value={exportHost} onChange={(event) => setExportHost(event.target.value)} />
                    </div>
                    <div className="field-group">
                      <label className="field-label" htmlFor="platform-export-port">
                        {t("代理端口")}
                      </label>
                      <Input id="platform-export-port" value={exportPort} onChange={(event) => setExportPort(event.target.value)} />
                    </div>
                    <div className="field-group">
                      <label className="field-label" htmlFor="platform-export-token">
                        {t("代理 Token")}
                      </label>
                      <Input id="platform-export-token" value={exportToken} onChange={(event) => setExportToken(event.target.value)} />
                    </div>
                    <div className="field-group">
                      <label className="field-label" htmlFor="platform-export-account">
                        {t("示例账号")}
                      </label>
                      <Input id="platform-export-account" value={exportAccount} onChange={(event) => setExportAccount(event.target.value)} />
                    </div>
                  </div>

                  <div className="platform-export-list">
                    <div className="platform-export-item">
                      <div className="platform-op-copy">
                        <h5>{t("普通代理地址")}</h5>
                        <p className="platform-op-hint">{t("适合只能填写一个 HTTP 代理地址的浏览器或第三方软件。")}</p>
                      </div>
                      <Input readOnly value={plainProxyURL} />
                      <Button variant="secondary" onClick={() => void copyText(plainProxyURL, t("普通代理地址已复制"))}>
                        {t("复制")}
                      </Button>
                    </div>

                    <div className="platform-export-item">
                      <div className="platform-op-copy">
                        <h5>{t("粘性代理地址")}</h5>
                        <p className="platform-op-hint">{t("固定账号时优先复用原出口 IP，适合登录、养号、长会话。")}</p>
                      </div>
                      <Input readOnly value={stickyProxyURL} />
                      <Button variant="secondary" onClick={() => void copyText(stickyProxyURL, t("粘性代理地址已复制"))}>
                        {t("复制")}
                      </Button>
                    </div>

                    <div className="platform-export-item">
                      <div className="platform-op-copy">
                        <h5>{t("多账号模板")}</h5>
                        <p className="platform-op-hint">{t("把 {account} 替换成你的业务账号名，每个账号都会独立保持粘性。")}</p>
                      </div>
                      <Input readOnly value={stickyTemplateURL} />
                      <Button variant="secondary" onClick={() => void copyText(stickyTemplateURL, t("多账号模板已复制"))}>
                        {t("复制")}
                      </Button>
                    </div>

                    <div className="platform-export-item">
                      <div className="platform-op-copy">
                        <h5>{t("Rotate API 地址")}</h5>
                        <p className="platform-op-hint">{t("当当前账号 IP 被风控时，可单独调用这个接口让下次请求换出口。")}</p>
                      </div>
                      <Input readOnly value={rotateAPIURL} />
                      <Button variant="secondary" onClick={() => void copyText(rotateAPIURL, t("Rotate API 地址已复制"))}>
                        {t("复制")}
                      </Button>
                    </div>
                  </div>

                  <div className="platform-usage-notes">
                    <div className="platform-usage-note">
                      <h5>{t("当前项目推荐接法")}</h5>
                      <ul className="platform-usage-list">
                        <li>{t("普通浏览器或指纹浏览器：直接复制“普通代理地址”填入 HTTP 代理。")}</li>
                        <li>{t("需要按账号长期保持出口：复制“多账号模板”，把 {account} 替换成业务账号。")}</li>
                        <li>{t("Gemini Business2API：Auth 和 Chat 平台都建议开启“粘性代理模式”并把“轮换策略”设为“保持原出口”。")}</li>
                        <li>{t("Gemini Business2API 里无需单独再填 Rotate API，只要代理地址使用 http://平台.{account}:Token@主机:端口 这种标准格式，程序就能自动推导 Rotate 接口。")}</li>
                      </ul>
                    </div>

                    <div className="platform-usage-note">
                      <h5>{t("手动 Rotate 请求示例")}</h5>
                      <p className="platform-op-hint">
                        {t("如果业务端识别到“当前账号这个 IP 已被风控”，可以主动调用下面这条命令。")}
                      </p>
                      <Textarea readOnly rows={3} value={rotateCurlCommand} />
                      <div className="platform-usage-actions">
                        <Button variant="secondary" onClick={() => void copyText(rotateCurlCommand, t("Rotate 命令已复制"))}>
                          {t("复制命令")}
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>
              </section>
            ) : null}

            {activeTab === "ops" ? (
              <section
                id="platform-tabpanel-ops"
                role="tabpanel"
                aria-labelledby="platform-tab-ops"
                className="platform-detail-tabpanel platform-ops-section"
              >
                <div className="platform-drawer-section-head">
                  <h4>{t("运维操作")}</h4>
                  <p>{t("以下操作会直接作用于当前平台，请谨慎执行。")}</p>
                </div>

                <div className="platform-ops-list">
                  <div className="platform-op-item">
                    <div className="platform-op-copy">
                      <h5>{t("切换指定账号 IP")}</h5>
                      <p className="platform-op-hint">{t("输入账号后执行 rotate，下次该账号请求会重新分配出口。")}</p>
                    </div>
                    <div className="platform-rotate-inline">
                      <Input
                        value={rotateAccount}
                        onChange={(event) => setRotateAccount(event.target.value)}
                        placeholder={t("输入账号，例如 account_001")}
                      />
                      <Button variant="secondary" onClick={() => void handleRotateLease()} disabled={rotateLeaseMutation.isPending}>
                        {rotateLeaseMutation.isPending ? t("切换中...") : t("切换 IP")}
                      </Button>
                    </div>
                  </div>

                  <div className="platform-op-item">
                    <div className="platform-op-copy">
                      <h5>{t("重置为默认配置")}</h5>
                      <p className="platform-op-hint">{t("恢复默认设置，并覆盖当前修改。")}</p>
                    </div>
                    <Button variant="secondary" onClick={() => void resetMutation.mutateAsync()} disabled={resetMutation.isPending}>
                      {resetMutation.isPending ? t("重置中...") : t("重置为默认配置")}
                    </Button>
                  </div>

                  <div className="platform-op-item">
                    <div className="platform-op-copy">
                      <h5>{t("清除所有租约")}</h5>
                      <p className="platform-op-hint">{t("立即清除当前平台的全部租约，下次请求将重新分配出口。")}</p>
                    </div>
                    <Button variant="danger" onClick={() => void handleClearAllLeases()} disabled={clearLeasesMutation.isPending}>
                      {clearLeasesMutation.isPending ? t("清除中...") : t("清除所有租约")}
                    </Button>
                  </div>

                  <div className="platform-op-item">
                    <div className="platform-op-copy">
                      <h5>{t("删除平台")}</h5>
                      <p className="platform-op-hint">{t("永久删除当前平台及其配置，操作不可撤销。")}</p>
                    </div>
                    <Button variant="danger" onClick={() => void handleDelete()} disabled={deleteDisabled}>
                      {deleteMutation.isPending ? t("删除中...") : t("删除平台")}
                    </Button>
                  </div>
                </div>
              </section>
            ) : null}
          </Card>
        </>
      ) : null}
    </section>
  );
}
