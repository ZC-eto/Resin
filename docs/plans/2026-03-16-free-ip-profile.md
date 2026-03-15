# Free IP Profiling Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a hot-configurable free online IP profiling flow with persistent egress-IP cache and manual recheck actions in Resin.

**Architecture:** Extend runtime config and system settings UI for IP profiling controls, refactor the profiling service to read runtime settings and a pluggable online provider, persist profile cache by egress IP in cache.db, and expose single/bulk recheck actions through the nodes API/UI.

**Tech Stack:** Go, SQLite, React, TanStack Query/Table, existing Resin control-plane APIs.

---

### Task 1: Extend runtime config for IP profiling controls

**Files:**
- Modify: `internal/config/runtime.go`
- Modify: `internal/service/control_plane_system.go`
- Modify: `internal/config/runtime_test.go`
- Modify: `internal/service/control_plane_test.go`
- Modify: `webui/src/features/systemConfig/types.ts`
- Modify: `webui/src/features/systemConfig/api.ts`

**Step 1: Write/extend failing tests**

- Add runtime-config tests for new JSON fields and defaults.
- Add control-plane validation tests for provider enum, positive RPM, positive cache TTL.

**Step 2: Implement minimal config model**

- Add new runtime fields for local lookup toggle, provider, api key, rpm, cache ttl, background toggle, startup refresh, egress-change refresh.
- Extend allowlist and validation logic.

**Step 3: Update frontend runtime config types**

- Add matching TS fields and default normalization.

**Step 4: Run targeted tests**

- `go test ./internal/config ./internal/service`

### Task 2: Add persistent egress-IP profile cache

**Files:**
- Add: `internal/state/migrations/cache/000004_egress_profile_cache.up.sql`
- Add: `internal/state/migrations/cache/000004_egress_profile_cache.down.sql`
- Modify: `internal/state/repo_cache.go`
- Modify: `internal/model/models.go`
- Modify: `internal/state/repo_cache_test.go`

**Step 1: Write failing repo tests**

- Add tests for upsert/load/delete of egress profile cache rows.

**Step 2: Implement cache repo methods**

- Add SQL and model for egress IP profile cache entries.
- Wire load/save/delete helpers.

**Step 3: Run targeted tests**

- `go test ./internal/state`

### Task 3: Refactor profiling service for hot config and provider abstraction

**Files:**
- Modify: `internal/ipprofile/service.go`
- Add: `internal/ipprofile/provider_proxycheck.go`
- Add: `internal/ipprofile/provider.go`
- Modify: `cmd/resin/main.go`
- Modify: `cmd/resin/app_runtime.go`
- Modify: `internal/service/control_plane_nodes.go`
- Modify: `internal/service/control_plane_nodes_test.go`

**Step 1: Write failing service tests**

- Add tests for local-first/online-fallback.
- Add tests for persistent-cache hit avoiding provider calls.
- Add tests for force refresh bypassing cache.

**Step 2: Implement provider abstraction**

- Keep IPinfo as one provider.
- Add Proxycheck provider with free-tier mapping.

**Step 3: Implement runtime-config-driven behavior**

- Replace fixed startup values with getters backed by runtime config.
- Respect `refresh_existing_on_start` and `refresh_on_egress_change`.

**Step 4: Implement force-refresh and batch enqueue**

- Add service methods to reprofile one or many nodes.

**Step 5: Run targeted tests**

- `go test ./internal/ipprofile ./internal/service ./cmd/resin`

### Task 4: Add node recheck APIs

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handler_node.go`
- Modify: `internal/api/handler_node_test.go`
- Modify: `internal/api/contract_test.go`
- Modify: `webui/src/features/nodes/api.ts`
- Modify: `webui/src/features/nodes/types.ts`

**Step 1: Write failing API tests**

- Single-node reprofile action.
- Batch reprofile action.

**Step 2: Implement handlers**

- Expose POST action for single reprofile and batch reprofile.

**Step 3: Update frontend client**

- Add API helpers and result types.

**Step 4: Run targeted tests**

- `go test ./internal/api`

### Task 5: Extend system settings UI

**Files:**
- Modify: `webui/src/features/systemConfig/SystemConfigPage.tsx`
- Modify: `webui/src/i18n/translations.ts`

**Step 1: Add form fields and descriptions**

- New “节点画像” section with Chinese help text.

**Step 2: Add validation and patch preview**

- Ensure provider/key/rpm/cache TTL fields parse correctly.

**Step 3: Run frontend checks**

- `npm run lint`
- `npm run build`

### Task 6: Extend nodes page for single/bulk recheck

**Files:**
- Modify: `webui/src/features/nodes/NodesPage.tsx`

**Step 1: Add row selection / bulk actions**

- Support selecting multiple nodes from the current page.

**Step 2: Add single-node and drawer recheck actions**

- Button text should make it clear this refreshes network-type profiling.

**Step 3: Add bulk recheck action**

- Selected nodes trigger batch endpoint and refresh queries.

**Step 4: Run frontend checks**

- `npm run lint`
- `npm run build`

### Task 7: Update docs and finalize verification

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Step 1: Document free provider flow**

- Explain local vs online roles and the new settings page.

**Step 2: Run full verification**

- `go test ./...`
- `npm run lint`
- `npm run build`
- `run mojibake scan`

**Step 3: Commit**

- `git add ...`
- `git commit -m "feat(proxy): add free online ip profiling controls"`
