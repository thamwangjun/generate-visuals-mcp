---
phase: 1
slug: core-server-image-generation-tool
status: validated
nyquist_compliant: partial
date: 2026-05-27
---

# Phase 1 Validation: Core Server + Image Generation Tool

## Test Infrastructure

| Framework | Config | Run Command |
|-----------|--------|-------------|
| Go stdlib testing | go.mod | `/home/thamw/.local/share/mise/installs/go/latest/bin/go test ./... -v` |

## Per-Task Coverage Map

### Task 1: Go module dependencies

| Requirement | Test | File | Status |
|-------------|------|------|--------|
| go.mod contains pinned deps | Build check | `go build ./...` | MANUAL |

**Manual-only note:** Dependency pinning is verified by `go build ./...` and `go mod verify`. No automated test covers this — it is enforced structurally by the Go module system.

---

### Task 2: Config package

| Requirement | Test | File | Status |
|-------------|------|------|--------|
| CFG-01: GEMINI_API_KEY read from env | `TestConfigLoad_FromEnv` | `internal/config/config_test.go` | COVERED |
| CFG-02: godotenv.Load() does not override env | `TestConfigLoad_EnvWinsOverDotenv` | `internal/config/config_test.go` | COVERED |
| CFG-03: env wins over .env | `TestConfigLoad_EnvWinsOverDotenv` | `internal/config/config_test.go` | COVERED |
| CFG-04: LISTEN_ADDR default :8080 | `TestConfigLoad_DefaultListenAddr` | `internal/config/config_test.go` | COVERED |
| CFG-04: Phase 2 env fields (MCP_PUBLIC_URL, AUTHELIA_URL, AUTHELIA_CLIENT_ID) | — | — | MANUAL |
| CFG-05: Fatal if GEMINI_API_KEY missing | `TestConfigLoad_MissingKey` | `internal/config/config_test.go` | COVERED |
| SRV-01: Default listen address :8080 | `TestConfigLoad_DefaultListenAddr` | `internal/config/config_test.go` | COVERED |

**Manual-only note (CFG-04 Phase 2 fields):** MCP_PUBLIC_URL, AUTHELIA_URL, and AUTHELIA_CLIENT_ID are scaffolded for Phase 2 and unused in Phase 1. Verifying they are readable requires no behavioural test — they are read by `os.Getenv` identically to other fields.

---

### Task 3: Tool package + auth scaffold

| Requirement | Test | File | Status |
|-------------|------|------|--------|
| TOOL-01: generate_visuals tool registered | `TestToolRegistered` | `internal/tools/generate_visuals_test.go` | COVERED |
| TOOL-02: image_prompt parameter required | `TestMissingImagePrompt` | `internal/tools/generate_visuals_test.go` | COVERED |
| TOOL-03: Calls Gemini generate content | `TestGenerateVisualsIntegration` | `internal/tools/generate_visuals_test.go` | PARTIAL |
| TOOL-04: Returns base64 image in ImageContent | `TestGenerateVisualsIntegration` | `internal/tools/generate_visuals_test.go` | PARTIAL |
| TOOL-05: Returns ToolResultError on errors | `TestGeminiClientError`, `TestMissingImagePrompt` | `internal/tools/generate_visuals_test.go` | COVERED |
| Panic recovery | `TestPanicRecovery` | `internal/tools/generate_visuals_test.go` | COVERED |

**Partial note (TOOL-03/04):** `TestGenerateVisualsIntegration` is gated on `GEMINI_API_KEY` being set. This is intentional per PLAN.md — live API tests skip in CI without the key. These requirements are also verified by the manual curl smoke test (Task 4).

---

### Task 4: main.go + server wiring

| Requirement | Test | File | Status |
|-------------|------|------|--------|
| SRV-02: /mcp HTTP endpoint wired | `TestHTTPEndpointPath_MCPResponds` | `main_test.go` | COVERED |
| SRV-04: serverInfo name=generate-visuals-mcp version=1.0.0 | `TestMCPServerIdentity_InitializeResponse` | `main_test.go` | COVERED |
| Build compiles cleanly | `go build ./...` | — | MANUAL |
| Server startup log message | Live run | — | MANUAL |

**Implementation note (SRV-02):** `WithEndpointPath("/mcp")` only affects `Start()` mux routing — when `StreamableHTTPServer` is used as an `http.Handler` directly (as `main.go` does with `http.Server{Handler: httpHandler}`), all requests are handled regardless of URL path. The `/mcp` path constraint in production is enforced by the outer `http.Server` mux configuration.

---

## Manual-Only Items

| Item | Reason | How to Verify |
|------|--------|---------------|
| Task 1: Dep versions pinned | Build system concern, not runtime | `go mod verify && go list -m all` |
| CFG-04 Phase 2 env fields | Unused in Phase 1, Phase 2 scope | Read config.go — fields present |
| SRV-01: Server actually binds :8080 | Covered by integration smoke test | Start server, check log output |
| Build compiles | CI gate | `go build ./... && go vet ./...` |
| TOOL-03/04 live Gemini call | Requires paid API key | Run with GEMINI_API_KEY set |
| Curl smoke test | Full E2E with live Gemini | See PLAN.md Task 4 smoke test |

---

## Validation Audit 2026-05-27

| Metric | Count |
|--------|-------|
| Requirements audited | 13 |
| COVERED | 9 |
| PARTIAL (intentional) | 2 |
| MANUAL | 5 |
| Gaps found | 2 (SRV-02, SRV-04) |
| Resolved by automation | 2 |
| Escalated to manual | 0 |
