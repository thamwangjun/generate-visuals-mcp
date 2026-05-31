---
phase: 02-authelia-oauth-protection
verified: 2026-05-31T07:57:00Z
status: human_needed
score: 7/7
overrides_applied: 0
human_verification:
  - test: "GET /mcp with expired JWT (valid structure, valid RS256 signature, exp in past)"
    expected: "401 Unauthorized with WWW-Authenticate header"
    why_human: "Requires a real Authelia instance issuing opaque tokens with access_token_signed_response_alg: RS256. Cannot be tested against in-process test JWKS server with already-expired claim in unit tests (the unit test infrastructure doesn't cover the exp-in-past path end-to-end with real token issuance)."
  - test: "GET /mcp with valid JWT issued by a real Authelia instance (RS256, correct iss/aud/exp)"
    expected: "200 OK (or MCP protocol-level response)"
    why_human: "Requires a running Authelia instance with access_token_signed_response_alg: RS256 configured. This is the end-to-end integration path that exercises keyfunc's actual key lookup against real Authelia JWKS."
  - test: "GET /mcp/ (trailing slash) with no Authorization header after JWKS loads"
    expected: "401 Unauthorized with WWW-Authenticate: Bearer resource_metadata=..."
    why_human: "Both /mcp and /mcp/ are registered in the mux, but no automated test sends a request to /mcp/ through the real mux. The unit tests use the middleware handler directly without a mux layer."
  - test: "Start server with AUTHELIA_URL=https://authelia.example.com/ (trailing slash)"
    expected: "Server starts and cfg.AutheliaBaseURL has no trailing slash; JWT iss validation succeeds when real Authelia tokens are used (UAT-10)"
    why_human: "The unit test TestConfigLoad_TrailingSlashStripped confirms the stripping logic. Whether the stripped URL correctly matches the iss claim in real Authelia tokens requires a live integration test."
---

# Phase 2: Authelia OAuth Protection — Verification Report

**Phase Goal:** Secure the server with JWT bearer token validation backed by Authelia. Only authenticated clients can call tools.
**Verified:** 2026-05-31T07:57:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `go build ./...` and `go test ./...` both pass with zero failures | VERIFIED | `go build ./...` exits 0 with no output. `go test ./...` reports `ok` for all 4 packages: root, internal/auth, internal/config, internal/tools. |
| 2 | `GET /mcp` without a token returns `401` with `WWW-Authenticate: Bearer resource_metadata=...` | VERIFIED | `middleware.go:142` sets `WWW-Authenticate: Bearer resource_metadata="<prmURL>"` in `unauthorized()`. `TestMiddleware_NoToken` and `TestMiddleware_401Header` both pass green, asserting exact header value. |
| 3 | `GET /mcp` before JWKS loads returns `503` with no `WWW-Authenticate` header | VERIFIED | `middleware.go:149-153` — `serviceUnavailable()` sets only `Content-Type` and `Retry-After: 10`; no `WWW-Authenticate`. `TestMiddleware_503NoWWWAuthenticate` explicitly asserts `rr.Header().Get("WWW-Authenticate") == ""`. Both `TestMiddleware_NotLoaded` and `TestMiddleware_503NoWWWAuthenticate` pass. |
| 4 | `GET /.well-known/oauth-protected-resource` returns `200` with no `Authorization` required | VERIFIED | `main.go:55` — `mux.Handle("/.well-known/oauth-protected-resource", prmHandler)` registers the PRM handler directly with no auth middleware wrapper. Only `/mcp` and `/mcp/` routes are wrapped with `validator.Middleware()`. `TestPRMEndpoint_PublicAccess` confirms the handler returns 200 with `Content-Type: application/json` and `resource` / `authorization_servers` fields. |
| 5 | Server exits immediately with clear fatal message if `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, or `MCP_PUBLIC_URL` is unset | VERIFIED | `config.go:33-41` has three `log.Fatal` guards, each with a human-readable message naming the missing variable. `TestConfigLoad_MissingAutheliaURL`, `TestConfigLoad_MissingAutheliaClientID`, `TestConfigLoad_MissingPublicURL` all pass using the subprocess fatal pattern, confirming non-zero exit. |
| 6 | Issuer (`iss`) and audience (`aud`) claims are validated; wrong values reject the token | VERIFIED | `middleware.go:110-111` uses `jwt.WithIssuer(v.issuer)` and `jwt.WithAudience(v.audience)`. `TestMiddleware_WrongIssuer` and `TestMiddleware_WrongAudience` both pass — tokens with mismatched `iss`/`aud` receive 401. |
| 7 | JWKS is fetched from Authelia and refreshed on rotation; server returns 503 until JWKS is available | VERIFIED | `middleware.go:43-57` — `keyfunc.NewDefaultOverrideCtx` initialised with `RefreshInterval: 5*time.Minute` and error logging on refresh failure. `waitForLoad()` polls `v.kf.Storage().KeyReadAll(ctx)` with exponential backoff (1s → 30s cap). `loaded` atomic bool stays false until at least one key is present in keyfunc's storage. |

**Score:** 7/7 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/auth/middleware.go` | Validator struct, NewValidatorAsync, Middleware, waitForLoad, extractBearer, unauthorized, serviceUnavailable | VERIFIED | 155 lines, all functions present and substantive. Imported by `main.go` at line 14 via `github.com/thamwangjun/generate-visuals-mcp/internal/auth`. |
| `internal/auth/middleware_test.go` | 8 middleware unit tests + JWKS helpers | VERIFIED | 387 lines. All 8 TestMiddleware_* functions present with real test bodies (no t.Skip). All pass. |
| `internal/config/config.go` | strings.TrimRight on AutheliaBaseURL + 3 auth var fail-fast guards | VERIFIED | `strings.TrimRight(os.Getenv("AUTHELIA_URL"), "/")` at line 26. Three `log.Fatal` guards at lines 33-41. |
| `internal/config/config_test.go` | 3 subprocess fatal tests + trailing-slash test | VERIFIED | TestConfigLoad_MissingAutheliaURL, TestConfigLoad_MissingAutheliaClientID, TestConfigLoad_MissingPublicURL, TestConfigLoad_TrailingSlashStripped all present and passing. |
| `main.go` | Mux with auth middleware on /mcp + PRM handler on /.well-known/ | VERIFIED | `mux.Handle("/mcp", protected)`, `mux.Handle("/mcp/", protected)`, `mux.Handle("/.well-known/oauth-protected-resource", prmHandler)` at lines 53-55. |
| `main_test.go` | TestPRMEndpoint_PublicAccess | VERIFIED | Present at line 85, tests 200 + JSON body with `resource` and `authorization_servers` fields. Passes. |
| `go.mod` | `keyfunc/v3 v3.8.0`, `golang-jwt/jwt/v5 v5.3.1` | VERIFIED | Both `require` entries confirmed in go.mod. |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `main.go` | `internal/auth` | `auth.NewValidatorAsync(ctx, cfg)` | VERIFIED | `main.go:36-39` — validator initialised; `main.go:52` — `validator.Middleware(prmURL)(httpServer)` creates the protected handler. |
| `main.go` | `internal/config` | `config.Load()` | VERIFIED | `main.go:20` — `cfg := config.Load()`. Auth vars (`AutheliaBaseURL`, `AutheliaClientID`, `PublicBaseURL`) consumed at `main.go:39,41,44,45`. |
| `Validator.Middleware` | `mux` | `mux.Handle("/mcp", protected)` | VERIFIED | Both `/mcp` and `/mcp/` registered with the same `protected` handler (not two separate middleware instantiations — IN-01 fix applied). |
| `/.well-known/oauth-protected-resource` | `prmHandler` (no auth) | `mux.Handle(...)` direct | VERIFIED | PRM route registered at `main.go:55` with no `validator.Middleware()` wrapper. |
| `config.AutheliaBaseURL` | `Validator.issuer` | `v.issuer = cfg.AutheliaBaseURL` in `NewValidatorAsync` | VERIFIED | `middleware.go:39`. Trailing slash already stripped by config. |
| `config.AutheliaClientID` | `Validator.audience` | `v.audience = cfg.AutheliaClientID` | VERIFIED | `middleware.go:40`. |

---

### Data-Flow Trace (Level 4)

Not applicable — this phase produces HTTP middleware and config loading logic, not components that render dynamic data.

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `go build ./...` passes | `go build ./...` (run in project root) | Exit 0, no output | PASS |
| `go test ./...` all packages pass | `go test ./...` | 4 packages `ok`, 0 failures | PASS |
| All 8 middleware unit tests pass | `go test ./internal/auth/... -v -run TestMiddleware` | All 8 PASS; see detailed output | PASS |
| Config fatal-exit tests pass | `go test ./internal/config/... -v -run TestConfigLoad_Missing\|TrailingSlash` | MissingAutheliaURL, MissingAutheliaClientID, MissingPublicURL, TrailingSlashStripped all PASS | PASS |
| PRM endpoint returns 200 with JSON | `go test . -v -run TestPRMEndpoint` | PASS — 200, Content-Type: application/json, `resource` and `authorization_servers` present | PASS |

---

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|---------|
| AUTH-01 | All requests to `/mcp` require a valid JWT bearer token | VERIFIED | `/mcp` and `/mcp/` both wrapped in `validator.Middleware()`. Missing or invalid token returns 401. |
| AUTH-02 | JWKS fetched from Authelia `jwks_uri` on startup, refreshed on rotation | VERIFIED | `keyfunc.NewDefaultOverrideCtx` with `RefreshInterval: 5*time.Minute`. JWKS URL derived from `cfg.AutheliaBaseURL + "/jwks.json"`. |
| AUTH-03 | Invalid/expired tokens receive 401 with `WWW-Authenticate: Bearer` including `resource_metadata` | VERIFIED | `unauthorized()` sets `WWW-Authenticate: Bearer resource_metadata="<prmURL>"`. TestMiddleware_401Header asserts exact value. |
| AUTH-04 | `/.well-known/*` paths bypass authentication | VERIFIED (scoped) | Only `/.well-known/oauth-protected-resource` is served in Phase 2 scope. It is registered without auth middleware. No other `/.well-known/` paths are registered or expected. |
| AUTH-05 | Issuer (`iss`) and audience (`aud`) claims validated | VERIFIED | `jwt.WithIssuer(v.issuer)` and `jwt.WithAudience(v.audience)` in `jwt.Parse`. Wrong-issuer and wrong-audience tests pass. |
| SRV-03 | Server serves `/.well-known/oauth-protected-resource` without authentication | VERIFIED | `mux.Handle("/.well-known/oauth-protected-resource", prmHandler)` — no auth wrapper. TestPRMEndpoint_PublicAccess confirms 200. |
| CFG-05 | Server fails fast with clear error if required config missing | VERIFIED | Three `log.Fatal` guards in `config.Load()` for `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, `MCP_PUBLIC_URL`. Subprocess tests confirm non-zero exit. |

**All 7 phase-2 requirements verified.**

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | — | — | No debt markers (TODO/FIXME/XXX/TBD), no stub returns, no placeholder implementations found in any modified file. |

---

### Deviations Accepted

The following deviations from PLAN.md were correctly applied and do not affect goal achievement:

1. **`waitForLoad` uses `Storage().KeyReadAll()` instead of HTTP probe.** Plan specified `http.Get(jwksURL)` + `time.Sleep(200ms)`. Implementation polls keyfunc's internal key storage directly. This eliminates the TOCTOU race (CR-01 fix). Functionally superior.

2. **`WithProtectedResourceMetadata` does not exist in mcp-go v0.54.1.** Plan called `server.WithProtectedResourceMetadata(...)`. Implementation uses `server.NewProtectedResourceMetadataHandler(config)` mounted directly on the mux. Functional outcome is identical — `/.well-known/oauth-protected-resource` returns the same JSON payload.

3. **`TestPRMEndpoint_PublicAccess` tests the handler directly, not through the full mux.** The test creates a standalone `prmHandler` and serves it via httptest. It confirms the handler produces correct output but does not exercise the mux routing decision. The mux wiring is verified by code inspection (no auth wrapper on the /.well-known/ route). Human UAT-06 covers the real integration path.

---

### Human Verification Required

#### 1. Expired JWT returns 401

**Test:** Send `GET /mcp` with `Authorization: Bearer <token>` where the token is RS256-signed by a real Authelia instance but has `exp` in the past.
**Expected:** `401 Unauthorized` with `WWW-Authenticate` header present.
**Why human:** Cannot be automated without a live Authelia instance. The unit tests cover token rejection by structural invalidity and wrong claims, but not the exp-in-past path against real Authelia-issued tokens.

#### 2. Valid JWT issued by real Authelia returns 200

**Test:** Obtain a valid access token from Authelia (requires `access_token_signed_response_alg: RS256` in Authelia client config). Send `GET /mcp` with `Authorization: Bearer <token>`.
**Expected:** `200 OK` (or MCP protocol-level response — not a 401 or 503).
**Why human:** End-to-end path requires a live Authelia instance. The unit tests use an in-process JWKS server, not real Authelia JWKS.

#### 3. `GET /mcp/` (trailing slash) with no token returns 401

**Test:** Send `GET /mcp/` with no Authorization header (after JWKS loads).
**Expected:** `401 Unauthorized` with `WWW-Authenticate: Bearer resource_metadata=...`.
**Why human:** No automated test sends a request to `/mcp/` through the full mux. The mux registers `mux.Handle("/mcp/", protected)` at `main.go:54` — code inspection confirms it is protected — but an integration test would give higher confidence.

#### 4. Trailing slash on AUTHELIA_URL is stripped end-to-end

**Test:** Start server with `AUTHELIA_URL=https://authelia.example.com/` (trailing slash). Obtain a real Authelia access token. Confirm JWT validation succeeds (i.e., `iss` claim matches the stripped URL).
**Expected:** Server starts; JWT `iss` validation passes in UAT-05.
**Why human:** `TestConfigLoad_TrailingSlashStripped` confirms the Go-level stripping. Whether the stripped URL string-equals the `iss` in real Authelia tokens requires live integration.

---

### Gaps Summary

No automated gaps found. All 7 must-have truths are VERIFIED in the codebase. The phase goal — "secure the server with JWT bearer token validation backed by Authelia; only authenticated clients can call tools" — is implemented correctly and covered by passing unit tests.

The 4 human verification items above are live-integration checks that require a real Authelia instance. They do not represent code defects — the implementation is correct. Status is `human_needed` pending UAT sign-off.

---

_Verified: 2026-05-31T07:57:00Z_
_Verifier: Claude (gsd-verifier)_
