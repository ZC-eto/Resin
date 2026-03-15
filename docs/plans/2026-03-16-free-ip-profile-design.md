# 免费画像与热配置设计

## 背景

当前 Resin 的节点画像链路依赖启动时环境变量，在线补充默认仅支持 IPinfo。这样会带来三个问题：

1. 在线 provider 切换需要重部署，不适合频繁调优。
2. 当前本地画像只能较稳地识别机房、匿名与移动网络，对住宅识别能力不足。
3. 现有按出口 IP 的缓存仅在内存中保留一段时间，重启后会丢失，容易重复消耗在线查询额度。

用户确认的新方向是：

- 不使用付费 IPinfo，优先接入免费在线 provider。
- 配置入口放到系统设置页，并补中文说明。
- 增加单个节点与批量节点的“重新检测”入口。
- 结果尽量按出口 IP 持久化，减少重复查询。

## 方案选择

### 方案 A：继续只用本地 MMDB

- 优点：无额外外网请求，部署简单。
- 缺点：无法很好识别住宅节点，核心目标达不成。

### 方案 B：纯免费在线 provider

- 优点：住宅/机房区分会明显好于当前本地库。
- 缺点：每个出口 IP 都要走远程；如果没有持久化缓存，重启和重复检测会浪费免费额度。

### 方案 C：本地预判 + 免费在线补充 + 持久化缓存

- 优点：本地先过滤明显机房/移动，只对未知节点走免费在线，最符合当前诉求。
- 缺点：实现面最大，需要调整画像服务和设置页。

本次采用 **方案 C**。

## 设计

### 1. 运行时画像设置

把以下项加入运行时可热更新设置，由系统设置页维护：

- `ip_profile_local_lookup_enabled`
- `ip_profile_online_provider`
- `ip_profile_online_api_key`
- `ip_profile_online_requests_per_minute`
- `ip_profile_cache_ttl`
- `ip_profile_background_enabled`
- `ip_profile_refresh_existing_on_start`
- `ip_profile_refresh_on_egress_change`

其中：

- provider 首版支持 `DISABLED`、`PROXYCHECK`、`IPINFO`
- 默认 provider 改为 `DISABLED`
- 线上若需要 IPinfo，仍可兼容旧环境变量作为启动默认值

### 2. provider 结构

抽象在线画像 provider：

- `IPINFO`：保留现有逻辑，避免回归
- `PROXYCHECK`：新增免费 provider 实现

优先级：

1. 本地 MMDB 预判
2. 若结果仍为未知且 provider 已启用，则请求在线 provider

### 3. 按出口 IP 的持久化缓存

新增 cache.db 表，按 `egress_ip` 存储：

- 网络类型
- ASN / ASNName / ASNType
- Provider
- 画像来源
- 最近检测时间

策略：

- 查询时先读持久化缓存
- 命中且未过期则直接复用
- 未命中或强制重检时才真正请求远程 provider

### 4. 重检入口

后端新增：

- 单节点重检：强制重新探测出口 IP 并刷新画像
- 批量重检：按节点 hash 数组批量执行

前端新增：

- 节点详情页按钮：重新检测网络类型
- 节点列表批量操作：对选中节点批量重新检测

### 5. 系统设置页

在“探测与路由”后新增“节点画像”分组，展示：

- 是否启用本地画像
- 在线 provider 选择
- API key / token
- 在线请求速率
- 缓存 TTL
- 启动时回填
- 出口变化时自动刷新

所有字段带中文说明，强调：

- `DISABLED` 不会请求在线 provider
- `PROXYCHECK` 适合免费方案
- API key 为管理员私有配置
- 速率过高会更快用掉免费额度

## 风险

- API key 若直接放入运行时配置接口，会被管理员读取。当前系统配置接口本身就是管理员访问，但仍应避免在列表/日志中泄露。
- 批量重检需要限制并发，避免一次性把免费额度打光。
- 增加持久化缓存后，需要兼顾旧数据兼容和迁移安全。

## 测试重点

- 运行时配置 patch 能正确保存并即时生效。
- `PROXYCHECK` provider 能把住宅/机房/移动映射为内部枚举。
- 持久化缓存命中时不重复调用在线 provider。
- 单节点与批量重检能刷新画像且不会影响其他节点状态。
- UI 设置页能保存、回滚、显示说明；节点页能执行重检。
