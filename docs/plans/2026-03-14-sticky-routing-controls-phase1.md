# Sticky Routing Controls Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add safe request-scoped sticky lease rotation and document/test standard HTTP proxy URL access for Resin V1.

**Architecture:** Keep the existing `Platform + Account + sticky lease` model. Implement forced rotation as a routing option that prefers a different egress IP for the current request, then expose it through a stripped internal request header in the proxy layer. Do not add new auth schemes or background workers.

**Tech Stack:** Go, net/http, httptest, existing Resin routing/proxy packages

---

### Task 1: Add routing support for forced sticky rotation

**Files:**
- Modify: `E:/Tools/proxy/Resin/internal/routing/router.go`
- Test: `E:/Tools/proxy/Resin/internal/routing/routing_test.go`

**Step 1: Write failing tests**

- Add a test that seeds a lease on one node, enables force rotation, and expects the router to choose a different egress IP when an alternative exists.
- Add a test that confirms force rotation falls back to the existing lease when no alternate egress IP exists.

**Step 2: Run targeted tests to verify failure**

Run: `go test ./internal/routing -run 'TestStickyLease_ForceRotate'`

**Step 3: Write minimal implementation**

- Introduce request options for routing.
- Add a force-rotation path before normal sticky-hit handling.
- Implement an alternate candidate selector that excludes the current egress IP.

**Step 4: Run targeted tests to verify pass**

Run: `go test ./internal/routing -run 'TestStickyLease_ForceRotate'`

### Task 2: Expose force rotation through proxy request control headers

**Files:**
- Modify: `E:/Tools/proxy/Resin/internal/proxy/forward.go`
- Modify: `E:/Tools/proxy/Resin/internal/proxy/reverse.go`
- Modify: `E:/Tools/proxy/Resin/internal/proxy/errors.go`
- Test: `E:/Tools/proxy/Resin/internal/proxy/e2e_test.go`
- Test: `E:/Tools/proxy/Resin/internal/proxy/proxy_test.go`

**Step 1: Write failing tests**

- Add an e2e test showing `X-Resin-Rotate` is accepted and stripped.
- Add a unit test for accepted rotate header values.

**Step 2: Run targeted tests to verify failure**

Run: `go test ./internal/proxy -run 'Test.*Rotate'`

**Step 3: Write minimal implementation**

- Parse `X-Resin-Rotate` as a best-effort internal control.
- Pass force-rotation options into routing.
- Strip `X-Resin-Rotate` from outbound forward and reverse traffic.

**Step 4: Run targeted tests to verify pass**

Run: `go test ./internal/proxy -run 'Test.*Rotate'`

### Task 3: Document HTTP proxy URL usage and transport security guidance

**Files:**
- Modify: `E:/Tools/proxy/Resin/README.md`
- Modify: `E:/Tools/proxy/Resin/README.zh-CN.md`
- Test: `E:/Tools/proxy/Resin/internal/proxy/e2e_subscription_test.go`

**Step 1: Add documentation examples**

- Add V1 forward proxy examples using `http://Platform.Account:TOKEN@host:port`.
- Add a note explaining that the format is convenient but not encrypted on plain HTTP transport.

**Step 2: Add e2e coverage**

- Add a real proxy-client test that uses `url.UserPassword("Platform.Account", "TOKEN")`.

**Step 3: Run focused tests**

Run: `go test ./internal/proxy -run 'TestForwardProxy_E2E.*V1'`

### Task 4: Run regression checks

**Files:**
- No source changes

**Step 1: Run affected package tests**

Run: `go test ./internal/routing ./internal/proxy ./internal/api ./internal/service`

**Step 2: Run full mojibake safety scan on changed text files**

Run a UTF-8-safe search for common mojibake markers in modified markdown files.

**Step 3: Review worktree**

Run: `git diff --stat`
