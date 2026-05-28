# Phase 2: Authelia OAuth Protection - Context

**Gathered:** 2026-05-28
**Status:** Ready for planning

<domain>
## Phase Boundary

Secure the MCP server with JWT bearer token validation backed by Authelia. Delivers: `internal/auth/` middleware package, JWKS fetching with exponential-backoff retry, `/.well-known/oauth-protected-resource` endpoint, mux-based HTTP routing, and config validation for auth env vars. Only authenticated clients can call tools after this phase.

</domain>

<decisions>
## Implementation Decisions

### Config Validation
- **D-01:** `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, and `MCP_PUBLIC_URL` are all **required** — extend the existing `log.Fatal` fail-fast pattern from `GEMINI_API_KEY` in `internal/config/config.go`.
- **D-02:** All three auth vars must be validated in `config.Load()` alongside the existing Gemini key check.

### JWKS Startup Behavior
- **D-03:** On startup, attempt JWKS fetch from Authelia. If it fails, **warn and continue** — do not fatal.
- **D-04:** Retry with **exponential backoff** (cap ~30s between attempts), retrying **indefinitely** until JWKS loads successfully.
- **D-05:** While JWKS is not yet loaded, all `/mcp` requests return **503 Service Unavailable** (not 401). This signals "auth not ready" distinctly from "bad token".
- **D-06:** Rationale: container environments where Authelia may start after the MCP server — the server should recover automatically without a restart.

### HTTP Routing Structure
- **D-07:** Use **mux-based routing** in `main.go`. Mount `/mcp` and `/mcp/` through auth middleware; mount `/.well-known/` directly to the HTTP server (no auth). Routing table is the source of truth for which paths are protected.
- **D-08:** Do NOT put path-based bypass logic inside the middleware — keep the middleware agnostic of application routing.
- **D-09:** Use mcp-go's built-in `server.WithProtectedResourceMetadata(server.ProtectedResourceMetadataConfig{...})` on `NewStreamableHTTPServer` to serve the PRM endpoint. No hand-rolled handler.

### Claude's Discretion
- Exact exponential backoff parameters (initial delay, jitter, cap) — use standard Go patterns (~1s initial, ~30s cap, optional jitter).
- Structured logging format for JWKS retry attempts and validation failures.
- Whether to expose a readiness signal (log line) when JWKS first loads successfully.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Implementation Guide
- `.planning/references/mcp-go-authelia.md` — Complete wiring guide: JWT middleware code, `WithProtectedResourceMetadata` config, mux setup, Authelia gotchas (opaque tokens, `iss` claim, `aud` = `client_id`). The reference middleware (§7.1) is the baseline — adapt for the 503/backoff behavior decided here.

### Requirements
- `.planning/REQUIREMENTS.md` — AUTH-01 through AUTH-05 and SRV-03 are the requirements this phase covers.

### Existing Code
- `internal/config/config.go` — Add fail-fast checks for `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, `MCP_PUBLIC_URL` here. Follow the existing `GEMINI_API_KEY` pattern.
- `main.go` — Replace the single `httpHandler` with a `http.ServeMux`. The Phase 2 comment already marks the injection point.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config/Config` struct: already has `AutheliaBaseURL`, `AutheliaClientID`, `PublicBaseURL` fields — no new fields needed, just add validation.
- `main.go` existing `http.Server` setup: timeouts already configured — keep them, just replace `Handler`.

### Established Patterns
- Fail-fast config validation: `if cfg.GeminiAPIKey == "" { log.Fatal(...) }` — replicate for auth vars.
- `internal/` package structure: new `internal/auth/` package follows the existing `internal/config/` and `internal/tools/` pattern.
- `tools.Register(mcpServer, cfg)` signature: auth middleware will need `cfg` too — keep the same dependency injection pattern.

### Integration Points
- `main.go:buildHandler` (to be extracted) wires: `config.Load()` → `auth.NewValidator()` → `server.NewStreamableHTTPServer(WithProtectedResourceMetadata)` → `http.ServeMux`.
- JWKS URL derived from config: `cfg.AutheliaBaseURL + "/jwks.json"`.
- Issuer = `cfg.AutheliaBaseURL`; Audience = `cfg.AutheliaClientID` (Authelia default: `aud` = `client_id`).

</code_context>

<specifics>
## Specific Ideas

- 503 response during JWKS load should include a `Retry-After` header if possible (optional, nice-to-have).
- Log line format for JWKS retry: include attempt number and next retry delay for operability.

</specifics>

<deferred>
## Deferred Ideas

- Rate limiting per JWT `sub` claim — noted in ROADMAP.md backlog (v2+).
- Scope validation in tool handlers (checking `scp` claim) — production hardening, not required for v1.
- Structured request logging (prompt, latency) — ROADMAP.md backlog.
- Docker/deployment config — ROADMAP.md backlog.

</deferred>

---

*Phase: 2-Authelia OAuth Protection*
*Context gathered: 2026-05-28*
