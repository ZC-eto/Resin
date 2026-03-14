# Sticky Routing Controls Phase 1 Design

**Scope**

Phase 1 adds two conservative capabilities:

1. Document and test standard HTTP proxy URL access using `http://user:pass@host:port` with Resin V1 forward-proxy credentials.
2. Add a request-scoped "rotate current sticky lease" control for authenticated proxy traffic.

**Why This Scope**

The current Resin routing model is stable because it revolves around `Platform + Account + sticky lease`.
This phase avoids changing the platform model, subscription model, or authentication model.
It only adds a small request-time control on top of the existing sticky lease system.

**Accepted Behaviors**

- Forward proxy clients may use standard proxy URLs whose user/password map to Resin's existing `Proxy-Authorization: Basic` parsing.
- Resin will not add a new encryption layer in phase 1.
- A request can ask Resin to rotate the current sticky lease before routing.
- Rotation is best-effort and only applies when an `Account` is available.
- Rotation prefers a different egress IP from the current lease.
- If no alternative egress IP exists, Resin keeps the current lease and continues serving the request.

**Rejected Behaviors**

- No request-scoped temporary region override.
- No new signed-token or encrypted proxy access protocol.
- No background scheduler that proactively rotates leases.
- No SOCKS inbound implementation in this phase.

**Security Notes**

- `http://user:pass@host:port` is only a client-side transport form for HTTP Basic auth; it is not encrypted by itself.
- When Resin is exposed across an untrusted network, operators should protect transport with a private network, TLS terminator, SSH tunnel, or equivalent trusted channel.
- The request-scoped control header must never leak to upstream targets.

**Implementation Shape**

- Routing layer:
  - add a request option for forced rotation
  - add a low-frequency alternate-route selector that excludes the current egress IP
- Proxy layer:
  - read `X-Resin-Rotate: true|1|yes|on`
  - apply forced rotation only to the current request
  - strip `X-Resin-Rotate` before forwarding upstream
- Docs and tests:
  - add README examples for V1 proxy URL usage
  - add security guidance for Basic auth over untrusted networks
  - add routing, proxy e2e, and header-stripping coverage
