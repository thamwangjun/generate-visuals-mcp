# Phase 2: Authelia OAuth Protection - Research

**Researched:** 2026-05-28
**Domain:** Go JWT middleware, JWKS async startup, mcp-go PRM API
**Confidence:** HIGH — keyfunc/v3 and mcp-go@v0.54.1 verified from source; golang-jwt/jwt/v5 verified from pkg.go.dev

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01/D-02:** `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, `MCP_PUBLIC_URL` are required — fail-fast in `config.Load()` using the existing `log.Fatal` pattern alongside `GEMINI_API_KEY`.
- **D-03:** On startup JWKS fetch failure: warn and continue — do NOT fatal.
- **D-04:** Retry with exponential backoff (~1s initial, ~30s cap), indefinitely until JWKS loads.
- **D-05:** While JWKS not yet loaded: return **503 Service Unavailable** (not 401) on `/mcp` requests.
- **D-06:** Container environment rationale — server must recover without restart.
- **D-07:** Mux-based routing in `main.go`. `/mcp` and `/mcp/` through auth middleware; `/.well-known/` directly to the HTTP server (no auth). Routing table is the single source of truth.
- **D-08:** No path-based bypass logic inside the middleware.
- **D-09:** Use `server.WithProtectedResourceMetadata(server.ProtectedResourceMetadataConfig{...})` on `NewStreamableHTTPServer`. No hand-rolled PRM handler.

### Claude's Discretion
- Exact exponential backoff parameters (~1s initial, ~30s cap, optional jitter).
- Structured logging format for JWKS retry attempts and validation failures.
- Whether to emit a log line when JWKS first loads successfully.

### Deferred Ideas (OUT OF SCOPE)
- Rate limiting per JWT `sub` claim
- Scope validation in tool handlers (`scp` claim)
- Structured request logging (prompt, latency)
- Docker/deployment config
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AUTH-01 | All requests to `/mcp` require a valid JWT bearer token | Middleware wraps `/mcp` in mux; keyfunc/v3 validates token |
| AUTH-02 | JWKS fetched from Authelia's `jwks_uri` on startup; refreshed on key rotation | `NewDefaultOverrideCtx` with `RefreshInterval` (1h default) + `RefreshUnknownKID` rate limiter |
| AUTH-03 | Invalid/expired tokens return 401 with `WWW-Authenticate: Bearer resource_metadata=...` | Reference §7.1 pattern; validated token path from keyfunc |
| AUTH-04 | `/.well-known/*` bypasses authentication | Mux routing: `/.well-known/` mounted directly to httpServer, no auth wrapper |
| AUTH-05 | `iss` and `aud` claims validated | `jwt.WithIssuer` + `jwt.WithAudience` parser options |
| SRV-03 | Server serves `/.well-known/oauth-protected-resource` without auth | `WithProtectedResourceMetadata` + mux direct mount |
| CFG-05 (auth) | Server fails fast if auth env vars missing | `log.Fatal` in `config.Load()` for three new required vars |
</phase_requirements>

---

## Summary

Phase 2 adds JWT bearer token validation middleware to the MCP server. The stack is: `github.com/MicahParks/keyfunc/v3` (JWKS fetching and refresh) + `github.com/golang-jwt/jwt/v5` (token parsing and claim validation), backed by Authelia as the OAuth authorization server.

The critical design requirement — async startup with 503-while-loading behavior — is achievable using `keyfunc.NewDefaultOverrideCtx`, which suppresses the first-fetch error by default (`noErrorReturnFirstHTTPReq = true` hardcoded inside the function). This means the JWKS storage is returned immediately even if Authelia is unreachable; validation calls return `jwkset.ErrKeyNotFound` until keys load. The implementation must detect this condition and return 503 rather than 401.

The exponential backoff retry is NOT built into keyfunc/v3 — it provides a `RefreshErrorHandlerFunc` callback and a `RefreshInterval`, but the backoff loop must be implemented by the application. A separate goroutine watching a `loaded` atomic boolean provides the cleanest approach without requiring external libraries.

mcp-go v0.54.1's `WithProtectedResourceMetadata` is confirmed from source: it registers an `http.Handler` on the path derived by `ProtectedResourceMetadataPath(cfg.Resource)`. When the `StreamableHTTPServer` is used as an `http.Handler` (the mux use case), it intercepts PRM requests itself in `ServeHTTP` — meaning mounting `httpServer` on `/.well-known/` in the mux is correct and sufficient.

**Primary recommendation:** Use `keyfunc.NewDefaultOverrideCtx` with a custom `RefreshErrorHandlerFunc` to drive a retry goroutine; use an `atomic.Bool` to track JWKS load state; return 503 in middleware when not loaded.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| JWT bearer validation | API / Backend (middleware) | — | Token validation is a server-side concern; never in the browser tier |
| JWKS fetch and refresh | API / Backend (startup) | — | Background goroutine in the server process |
| PRM endpoint (`/.well-known/oauth-protected-resource`) | API / Backend | — | mcp-go's `StreamableHTTPServer.ServeHTTP` handles it when `WithProtectedResourceMetadata` is configured |
| HTTP routing (mux) | API / Backend (`main.go`) | — | `http.ServeMux` wires auth-protected vs public paths |
| Config validation (auth vars) | API / Backend (`internal/config`) | — | Fail-fast at startup before any listener opens |

---

## 1. keyfunc/v3 Async Startup API

### Finding: `NewDefaultOverrideCtx` is the correct constructor

[VERIFIED: pkg.go.dev/github.com/MicahParks/keyfunc/v3] [VERIFIED: github.com/MicahParks/keyfunc/blob/master/keyfunc.go]

**Latest version:** `v3.8.0` (released 2026-02-11). `NoErrorReturnFirstHTTPReq *bool` was added to `Override` in this version.

**Constructor signatures:**

```go
// Synchronous — blocks on first fetch, returns error if fetch fails.
func NewDefault(urls []string) (Keyfunc, error)
func NewDefaultCtx(ctx context.Context, urls []string) (Keyfunc, error)

// Async-friendly — suppresses first-fetch error by default.
func NewDefaultOverrideCtx(ctx context.Context, urls []string, override Override) (Keyfunc, error)
```

**`Override` struct (v3.8.0):**

```go
type Override struct {
    Client                    *http.Client
    HTTPTimeout               time.Duration
    NoErrorReturnFirstHTTPReq *bool          // NEW in v3.8.0 — explicit control
    RateLimitWaitMax          time.Duration
    RefreshErrorHandlerFunc   func(u string) func(ctx context.Context, err error)
    RefreshInterval           time.Duration
    RefreshUnknownKID         *rate.Limiter
    ValidationSkipAll         bool
}
```

**Critical internal default in `NewDefaultOverrideCtx`** [VERIFIED: source]:

```go
noErrorReturnFirstHTTPReq := true  // hardcoded default inside the function
if override.NoErrorReturnFirstHTTPReq != nil {
    noErrorReturnFirstHTTPReq = *override.NoErrorReturnFirstHTTPReq
}
```

This means `NewDefaultOverrideCtx` **already suppresses the first-fetch error** even without setting `NoErrorReturnFirstHTTPReq`. The returned `Keyfunc` is always valid; the JWKS storage starts empty.

**Other defaults in `NewDefaultOverrideCtx`:**
- `refreshInterval = time.Hour` (overridable)
- `refreshUnknownKID = rate.NewLimiter(rate.Every(5*time.Minute), 1)` — triggers a re-fetch when an unknown `kid` is requested; max once per 5 minutes

**What happens when storage is empty:**
[VERIFIED: pkg.go.dev/github.com/MicahParks/jwkset#HTTPClientStorageOptions]

When `NoErrorReturnFirstHTTPReq = true` and the first fetch fails, the JWKS storage is empty. Calling `jwt.Parse(token, kf.Keyfunc, ...)` will fail because no key matching the token's `kid` exists. The error propagates up as a key-not-found condition (not a network error). The middleware must treat this as a 503 state.

### Concrete async startup pattern

```go
// internal/auth/validator.go

package auth

import (
    "context"
    "fmt"
    "log"
    "math"
    "net/http"
    "strings"
    "sync/atomic"
    "time"

    "github.com/MicahParks/keyfunc/v3"
    "github.com/golang-jwt/jwt/v5"
    "golang.org/x/time/rate"
)

type Validator struct {
    kf       keyfunc.Keyfunc
    loaded   atomic.Bool       // true once JWKS has been fetched at least once
    issuer   string
    audience string
}

// NewValidatorAsync starts JWKS loading in the background and returns
// immediately. Requests arriving before JWKS loads receive 503.
func NewValidatorAsync(ctx context.Context, jwksURL, issuer, audience string) (*Validator, error) {
    v := &Validator{issuer: issuer, audience: audience}

    // RefreshErrorHandlerFunc is called by keyfunc when a refresh attempt fails.
    // We use it to drive a separate "has ever loaded" flag via a one-time check
    // in the background goroutine below.
    errHandler := func(u string) func(ctx context.Context, err error) {
        attempt := 0
        return func(ctx context.Context, err error) {
            attempt++
            log.Printf("auth: JWKS refresh failed (attempt %d, url=%s): %v", attempt, u, err)
        }
    }

    override := keyfunc.Override{
        RefreshInterval:         5 * time.Minute,
        RefreshErrorHandlerFunc: errHandler,
        // RefreshUnknownKID uses keyfunc default: rate.NewLimiter(rate.Every(5*time.Minute), 1)
    }

    kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, override)
    if err != nil {
        // This error is from internal wiring (e.g., URL list empty), not from
        // the HTTP fetch. NewDefaultOverrideCtx suppresses first-fetch errors.
        return nil, fmt.Errorf("keyfunc setup failed: %w", err)
    }
    v.kf = kf

    // Background goroutine: poll until we can successfully validate a probe
    // token OR until we confirm the storage has keys. Use exponential backoff.
    go v.waitForLoad(ctx, jwksURL)

    return v, nil
}

// waitForLoad retries until the JWKS storage has at least one key.
// It sets v.loaded atomically on success and logs progress.
func (v *Validator) waitForLoad(ctx context.Context, jwksURL string) {
    const (
        initialDelay = 1 * time.Second
        maxDelay     = 30 * time.Second
    )
    delay := initialDelay
    attempt := 0

    for {
        select {
        case <-ctx.Done():
            return
        case <-time.After(delay):
        }

        attempt++
        // Probe: attempt to read keys via Storage().
        // Storage().KeyRead is not exported directly; instead we check if
        // a dummy Parse returns "key not found" vs succeeds.
        // A simpler probe: fetch jwks_uri directly with net/http.
        resp, err := http.Get(jwksURL) //nolint:noctx — probe, not per-request
        if err == nil && resp.StatusCode == http.StatusOK {
            resp.Body.Close()
            v.loaded.Store(true)
            log.Printf("auth: JWKS loaded successfully (attempt %d)", attempt)
            return
        }
        if resp != nil {
            resp.Body.Close()
        }

        log.Printf("auth: JWKS not yet available (attempt %d), retrying in %s: %v",
            attempt, delay, err)
        delay = minDuration(time.Duration(float64(delay)*math.Phi), maxDelay) // golden-ratio backoff
    }
}
```

**NOTE:** The `waitForLoad` goroutine above is a liveness probe — it sets `v.loaded` once Authelia responds. The actual JWKS key data is managed by keyfunc's own background goroutine. Once `loaded = true`, the keyfunc storage should have keys within milliseconds (keyfunc fetched them independently). The double-check is safe because `jwt.Parse` itself will return an error if keys are still not present.

**Simpler alternative for the loaded flag:** Check `kf.Storage().KeyRead(ctx, kid)` after a successful probe — but this requires knowing a `kid` in advance. The HTTP probe approach is more robust for startup.

---

## 2. mcp-go `WithProtectedResourceMetadata` API

[VERIFIED: /home/thamw/go/pkg/mod/github.com/mark3labs/mcp-go@v0.54.1/server/protected_resource.go]
[VERIFIED: /home/thamw/go/pkg/mod/github.com/mark3labs/mcp-go@v0.54.1/server/streamable_http.go]

### Confirmed function signature

```go
func WithProtectedResourceMetadata(config ProtectedResourceMetadataConfig) StreamableHTTPOption
```

This is a `StreamableHTTPOption` (not a `server.NewMCPServer` option) — it goes on `server.NewStreamableHTTPServer(...)`.

### `ProtectedResourceMetadataConfig` — all fields

```go
type ProtectedResourceMetadataConfig struct {
    Resource                              string   `json:"resource"`                                            // REQUIRED
    AuthorizationServers                  []string `json:"authorization_servers,omitempty"`                     // RECOMMENDED
    ScopesSupported                       []string `json:"scopes_supported,omitempty"`
    BearerMethodsSupported                []string `json:"bearer_methods_supported,omitempty"`
    ResourceName                          string   `json:"resource_name,omitempty"`
    ResourceDocumentation                 string   `json:"resource_documentation,omitempty"`
    ResourcePolicyURI                     string   `json:"resource_policy_uri,omitempty"`
    ResourceTosURI                        string   `json:"resource_tos_uri,omitempty"`
    JWKSURI                               string   `json:"jwks_uri,omitempty"`
    ResourceSigningAlgValuesSupported     []string `json:"resource_signing_alg_values_supported,omitempty"`
    TLSClientCertificateBoundAccessTokens *bool    `json:"tls_client_certificate_bound_access_tokens,omitempty"`
    AuthorizationDetailsTypesSupported    []string `json:"authorization_details_types_supported,omitempty"`
    DPoPSigningAlgValuesSupported         []string `json:"dpop_signing_alg_values_supported,omitempty"`
    DPoPBoundAccessTokensRequired         *bool    `json:"dpop_bound_access_tokens_required,omitempty"`
}
```

### How the PRM path is derived

```go
// ProtectedResourceMetadataPath returns:
// - "/.well-known/oauth-protected-resource"  if Resource has no path component
// - "/.well-known/oauth-protected-resource/mcp"  if Resource = "https://host/mcp"
func ProtectedResourceMetadataPath(resource string) string
```

For `PublicBaseURL = "https://mcp.example.com"` (no path): path = `/.well-known/oauth-protected-resource`.
For `PublicBaseURL = "https://mcp.example.com/mcp"` (with path): path = `/.well-known/oauth-protected-resource/mcp`.

Use a bare host as `PublicBaseURL` to get the standard `/.well-known/oauth-protected-resource` path.

### ServeHTTP intercept behavior

From `streamable_http.go` lines 326-329 [VERIFIED: source]:

```go
if s.protectedResourceMetadataHandler != nil && r.URL.Path == s.protectedResourceMetadataPath {
    s.protectedResourceMetadataHandler.ServeHTTP(w, r)
    return
}
```

When `httpServer` is mounted on `/.well-known/` in the mux, requests to `/.well-known/oauth-protected-resource` are dispatched to `httpServer.ServeHTTP`, which intercepts them internally. **No separate mux entry for the PRM path is needed.**

The handler sets:
- `Content-Type: application/json`
- `Cache-Control: no-store`
- `Access-Control-Allow-Origin: *`
- HTTP 200 with the JSON-encoded config

### Minimal config for this project

```go
server.WithProtectedResourceMetadata(server.ProtectedResourceMetadataConfig{
    Resource:               cfg.PublicBaseURL,
    AuthorizationServers:   []string{cfg.AutheliaBaseURL},
    ScopesSupported:        []string{"openid", "profile"},
    BearerMethodsSupported: []string{"header"},
    ResourceName:           "generate-visuals-mcp",
})
```

---

## 3. Exponential Backoff — Recommended Pattern

[ASSUMED — stdlib approach, no external library needed]

The `waitForLoad` goroutine uses exponential backoff via simple multiplication. No external package is required.

### Recommended parameters (Claude's discretion)

```go
const (
    backoffInitial = 1 * time.Second
    backoffMax     = 30 * time.Second
    backoffFactor  = 2.0  // or math.Phi (1.618) for gentler growth
)

delay := backoffInitial
for {
    // ... try ...
    if success {
        break
    }
    delay = time.Duration(float64(delay) * backoffFactor)
    if delay > backoffMax {
        delay = backoffMax
    }
}
```

With factor 2.0 and 1s initial: 1s → 2s → 4s → 8s → 16s → 30s (cap). Reaches cap after 5 failed attempts (~61s total wait). This is appropriate for a container startup scenario where Authelia may need 10-30s to become ready.

**Optional jitter:** `delay + time.Duration(rand.Int63n(int64(delay/4)))` — prevents thundering herd if multiple instances restart simultaneously. Low priority for single-instance deployments.

**No external library needed.** Do NOT add `github.com/cenkalti/backoff` or similar. The stdlib pattern is 8 lines.

---

## 4. File Change Summary

### Modified files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `log.Fatal` checks for `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, `MCP_PUBLIC_URL` alongside existing `GEMINI_API_KEY` check |
| `main.go` | Replace `httpHandler` + direct `httpSrv.Handler` with `http.ServeMux`. Extract `buildHandler` function or inline. Wire mux: `/mcp` → auth middleware → `httpServer`; `/mcp/` → same; `/.well-known/` → `httpServer` directly |

### New files

| File | Purpose |
|------|---------|
| `internal/auth/middleware.go` | `Validator` struct, `NewValidatorAsync`, `Middleware` func, `waitForLoad` goroutine, `extractBearer`, `unauthorized`, `serviceUnavailable` helpers |
| `internal/auth/middleware_test.go` | Tests for middleware: 503 before load, 401 on missing/bad token, 200 on valid token (using test JWKS), PRM endpoint accessibility |

`internal/auth/doc.go` already exists (placeholder) — no change needed.

---

## 5. Dependency Additions

### Packages to add

```bash
cd /home/thamw/development/remote-dev/generate-visuals-mcp

go get github.com/MicahParks/keyfunc/v3@v3.8.0
go get github.com/golang-jwt/jwt/v5@v5.3.1
```

`golang.org/x/time` (for `rate.Limiter` used internally by keyfunc) will be added transitively by `go get keyfunc/v3`.

### Version notes

| Package | Version | Released | Notes |
|---------|---------|----------|-------|
| `github.com/MicahParks/keyfunc/v3` | v3.8.0 | 2026-02-11 | Adds `NoErrorReturnFirstHTTPReq *bool` to `Override`; minimum required for explicit async control |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | 2026-01-28 | Fixes CVE GO-2025-3553 (excessive memory in header parsing) present in v5.0.0–v5.2.1 |

[VERIFIED: pkg.go.dev/github.com/MicahParks/keyfunc/v3?tab=versions]
[VERIFIED: pkg.go.dev/github.com/golang-jwt/jwt/v5?tab=versions]

### Post-install: go.mod will gain

```
require (
    github.com/MicahParks/keyfunc/v3 v3.8.0
    github.com/golang-jwt/jwt/v5     v5.3.1
)
```

Plus indirect dependencies from keyfunc/v3 (MicahParks/jwkset, golang.org/x/time).

---

## 6. Gotchas

### Gotcha 1: `NewDefault` vs `NewDefaultOverrideCtx` — async behavior

**What:** `NewDefault` and `NewDefaultCtx` call `jwkset.NewDefaultHTTPClientCtx`, which does NOT set `noErrorReturnFirstHTTPReq = true`. These variants block and return an error if the first fetch fails. `NewDefaultOverrideCtx` is the only constructor that suppresses the first-fetch error by default.

**How to avoid:** Always use `keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, override)` for the async startup pattern. Never `keyfunc.NewDefault` or `keyfunc.NewDefaultCtx`.

[VERIFIED: github.com/MicahParks/keyfunc/blob/master/keyfunc.go]

---

### Gotcha 2: PRM path depends on `PublicBaseURL` having no path component

**What:** `ProtectedResourceMetadataPath` appends the resource's URL path to the well-known prefix. If `PublicBaseURL = "https://mcp.example.com/mcp"`, the PRM path becomes `/.well-known/oauth-protected-resource/mcp`, not `/.well-known/oauth-protected-resource`. MCP clients (and Claude Desktop) expect the standard path.

**How to avoid:** Use `PublicBaseURL` as the bare host without a path: `"https://mcp.example.com"`. The `/mcp` endpoint path is separate (configured via `WithEndpointPath`). Both can coexist independently.

[VERIFIED: /home/thamw/go/pkg/mod/github.com/mark3labs/mcp-go@v0.54.1/server/protected_resource.go]

---

### Gotcha 3: `WithProtectedResourceMetadata` is a `StreamableHTTPOption`, not a `server.NewMCPServer` option

**What:** The reference guide §7.3 shows `server.NewStreamableHTTPServer(mcpServer, server.WithProtectedResourceMetadata(...))`. This is correct — but it's easy to accidentally pass it to `server.NewMCPServer` which takes `ServerOption` not `StreamableHTTPOption`. The compiler will catch this, but it's a likely first-attempt mistake.

**How to avoid:** Pass `WithProtectedResourceMetadata` only to `NewStreamableHTTPServer`, not `NewMCPServer`.

---

### Gotcha 4: Mux routing and trailing slash for `/mcp/`

**What:** Go's `http.ServeMux` does NOT automatically route `/mcp/` to a handler registered only for `/mcp`. The reference guide §7.3 correctly registers both:

```go
mux.Handle("/mcp", authMiddleware(httpServer))
mux.Handle("/mcp/", authMiddleware(httpServer))
```

If only `/mcp` is registered, some clients that append trailing slashes will get 404.

**How to avoid:** Register both `/mcp` and `/mcp/` in the mux.

---

### Gotcha 5: 503 during JWKS loading must NOT set `WWW-Authenticate`

**What:** A 503 response means "service temporarily unavailable" — it should NOT include `WWW-Authenticate: Bearer ...`. That header signals "you need a token", which is misleading when the problem is JWKS not loaded. Sending 503 with `WWW-Authenticate` may cause some MCP clients to trigger an OAuth flow unnecessarily.

**How to avoid:** In the `serviceUnavailable` helper, omit `WWW-Authenticate`. Optionally add `Retry-After: 10` (per the CONTEXT.md nice-to-have).

---

### Gotcha 6: Authelia access tokens are opaque by default

**What:** Without `access_token_signed_response_alg: 'RS256'` in the Authelia client config, access tokens are opaque strings (`authelia_at_XXXX`). `jwt.Parse` will fail to decode them (not a valid JWT). The middleware will return 401 for all requests even after JWKS loads.

**How to avoid:** The Authelia client config MUST have `access_token_signed_response_alg: 'RS256'`. This is a pre-deploy Authelia configuration requirement, not a code change. Document it prominently in the phase plan.

[CITED: .planning/references/mcp-go-authelia.md §10]

---

### Gotcha 7: `iss` claim = Authelia base URL, no trailing slash

**What:** Authelia sets `iss` to the exact base URL string (e.g., `"https://authelia.example.com"` — no trailing slash, no path suffix). `jwt.WithIssuer(v.issuer)` performs an exact-string comparison.

**How to avoid:** `cfg.AutheliaBaseURL` must not have a trailing slash. Strip it in config or validate.

[CITED: .planning/references/mcp-go-authelia.md §10]

---

### Gotcha 8: `aud` claim = `client_id` by default in Authelia

**What:** Unless a custom `audience` list is set in the Authelia client YAML, Authelia puts `client_id` as the single `aud` value. Pass `cfg.AutheliaClientID` as the audience to `NewValidatorAsync`.

[CITED: .planning/references/mcp-go-authelia.md §10]

---

### Gotcha 9: `keyfunc.Keyfunc` interface, not a function type

**What:** In keyfunc/v3, `keyfunc.Keyfunc` is an **interface**, not a `jwt.Keyfunc` function directly. To get the `jwt.Keyfunc` function for passing to `jwt.Parse`, call `.Keyfunc` (method on the interface) or `.KeyfuncCtx(ctx)` (context-aware variant):

```go
kf, err := keyfunc.NewDefaultOverrideCtx(ctx, urls, override)
// kf is keyfunc.Keyfunc (interface)

// Pass to jwt.Parse:
token, err := jwt.Parse(tokenStr, kf.Keyfunc, opts...)
//                                ^^^^^^^^^^^
//                                kf.Keyfunc is the jwt.Keyfunc func
```

[VERIFIED: pkg.go.dev/github.com/MicahParks/keyfunc/v3]

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/MicahParks/keyfunc/v3` | v3.8.0 | JWKS fetching, caching, background refresh, `jwt.Keyfunc` adapter | Standard bridge between jwkset and golang-jwt; referenced in mcp-go-authelia.md §5 |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT parsing, claim validation (`iss`, `aud`, `exp`) | The canonical Go JWT library; v5 has CVE fix over v5.0–v5.2.1 |

### Already present (no change)

| Library | Version | Notes |
|---------|---------|-------|
| `github.com/mark3labs/mcp-go` | v0.54.1 | `WithProtectedResourceMetadata` is in this version |

### Supporting (transitive)

| Library | Purpose |
|---------|---------|
| `github.com/MicahParks/jwkset` | Underlying JWKS storage; pulled in by keyfunc/v3 |
| `golang.org/x/time` | `rate.Limiter` for keyfunc's `RefreshUnknownKID`; pulled in transitively |

---

## Package Legitimacy Audit

> slopcheck not available in this environment. Both packages are well-established with years of history; registry verification performed manually.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| `github.com/MicahParks/keyfunc/v3` | pkg.go.dev | ~3 yrs (v3 line) | Listed on pkg.go.dev with 8+ versions | github.com/MicahParks/keyfunc | [ASSUMED] | Approved — widely referenced in Go JWT middleware guides |
| `github.com/golang-jwt/jwt/v5` | pkg.go.dev | ~10 yrs (jwt-go fork) | Standard library in Go ecosystem | github.com/golang-jwt/jwt | [ASSUMED] | Approved — canonical Go JWT library |

*slopcheck was unavailable at research time. Both packages are tagged `[ASSUMED]` and the planner should confirm before executing `go get`.*

---

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── auth/
│   ├── doc.go              # existing placeholder
│   └── middleware.go       # new: Validator, NewValidatorAsync, Middleware, waitForLoad
├── config/
│   ├── config.go           # modified: add fail-fast for auth vars
│   └── config_test.go      # existing
└── tools/
    ├── generate_visuals.go # existing
    └── ...
main.go                     # modified: mux routing, buildHandler pattern
main_test.go                # may need update: existing tests bypass auth
```

### Pattern: Validator with Atomic Load State

```go
// Source: research synthesis from keyfunc/v3 API
type Validator struct {
    kf       keyfunc.Keyfunc
    loaded   atomic.Bool    // set to true by waitForLoad goroutine
    issuer   string
    audience string
}

func (v *Validator) Middleware(prmURL string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !v.loaded.Load() {
                serviceUnavailable(w)
                return
            }
            // ... extract bearer, jwt.Parse, store in context, call next
        })
    }
}
```

### Pattern: Mux Routing (D-07, D-08)

```go
// Source: research synthesis from mcp-go source + CONTEXT.md D-07
mux := http.NewServeMux()
mux.Handle("/mcp", validator.Middleware(prmURL)(httpServer))
mux.Handle("/mcp/", validator.Middleware(prmURL)(httpServer))
mux.Handle("/.well-known/", httpServer)  // httpServer intercepts PRM internally

httpSrv := &http.Server{
    Addr:    cfg.ListenAddr,
    Handler: mux,
    // keep existing timeouts
}
```

### Anti-Patterns to Avoid

- **Path bypass inside middleware (violates D-08):** Do not check `strings.HasPrefix(r.URL.Path, "/.well-known/")` inside the `Middleware` function. The mux routing table is the single source of truth (D-07/D-08).
- **`keyfunc.NewDefault` for async startup:** This blocks and fails if Authelia is unreachable. Use `NewDefaultOverrideCtx` only.
- **Returning 401 when JWKS not loaded:** A missing key is not an auth failure — it means the validator is not ready. Return 503.
- **Registering `WithProtectedResourceMetadata` on `NewMCPServer`:** Wrong type — it belongs on `NewStreamableHTTPServer`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JWKS fetch, cache, refresh | Custom HTTP polling loop | `keyfunc/v3` + `jwkset` | Key rotation, `kid` lookup, concurrent safe storage — all handled |
| JWT parsing and claim validation | Custom base64/JSON decode | `golang-jwt/jwt/v5` | Expiry, leeway, issuer/audience validation edge cases |
| PRM endpoint handler | Custom JSON marshal + `http.Handler` | `server.WithProtectedResourceMetadata` | CORS headers, HEAD support, `Cache-Control: no-store` — all included |

---

## Code Examples

### Complete middleware skeleton (from research)

```go
// Source: keyfunc/v3 API + golang-jwt/jwt/v5 API + reference §7.1

func (v *Validator) Middleware(prmURL string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !v.loaded.Load() {
                w.Header().Set("Content-Type", "application/json")
                w.Header().Set("Retry-After", "10")
                w.WriteHeader(http.StatusServiceUnavailable)
                _, _ = w.Write([]byte(`{"error":"auth_not_ready","error_description":"JWKS not yet loaded"}`))
                return
            }

            tokenString, err := extractBearer(r)
            if err != nil {
                unauthorized(w, prmURL, err.Error())
                return
            }

            opts := []jwt.ParserOption{
                jwt.WithIssuer(v.issuer),
                jwt.WithAudience(v.audience),
                jwt.WithExpirationRequired(),
                jwt.WithLeeway(10 * time.Second),
            }
            token, err := jwt.Parse(tokenString, v.kf.Keyfunc, opts...)
            if err != nil || !token.Valid {
                unauthorized(w, prmURL, "invalid or expired token")
                return
            }

            ctx := context.WithValue(r.Context(), claimsKey, token)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### `NewValidatorAsync` with exponential backoff goroutine

```go
// Source: keyfunc/v3 NewDefaultOverrideCtx API

func NewValidatorAsync(ctx context.Context, jwksURL, issuer, audience string) (*Validator, error) {
    v := &Validator{issuer: issuer, audience: audience}

    override := keyfunc.Override{
        RefreshInterval: 5 * time.Minute,
        RefreshErrorHandlerFunc: func(u string) func(context.Context, error) {
            return func(_ context.Context, err error) {
                log.Printf("auth: JWKS refresh error (url=%s): %v", u, err)
            }
        },
    }

    kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, override)
    if err != nil {
        return nil, fmt.Errorf("keyfunc init: %w", err)
    }
    v.kf = kf

    go v.waitForLoad(ctx, jwksURL)
    return v, nil
}

func (v *Validator) waitForLoad(ctx context.Context, jwksURL string) {
    delay := time.Second
    for attempt := 1; ; attempt++ {
        select {
        case <-ctx.Done():
            return
        case <-time.After(delay):
        }
        resp, err := http.Get(jwksURL)
        if err == nil && resp.StatusCode == http.StatusOK {
            resp.Body.Close()
            v.loaded.Store(true)
            log.Printf("auth: JWKS ready (attempt %d)", attempt)
            return
        }
        if resp != nil {
            resp.Body.Close()
        }
        log.Printf("auth: JWKS not ready (attempt %d, next retry in %s): %v", attempt, delay, err)
        delay = min(delay*2, 30*time.Second)
    }
}
```

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build | ✓ | go1.26.3 | — |
| Authelia instance | JWKS validation (runtime) | Not checked | — | Server starts without it; 503 until available |
| `keyfunc/v3` module | Auth middleware | Not in cache | — | `go get` required |
| `golang-jwt/jwt/v5` module | Auth middleware | Not in cache | — | `go get` required |

**Missing dependencies with no fallback:** none — the server can start without Authelia reachable (503 behavior).

**Modules to fetch:**
```bash
go get github.com/MicahParks/keyfunc/v3@v3.8.0
go get github.com/golang-jwt/jwt/v5@v5.3.1
```

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | none — `go test ./...` |
| Quick run command | `go test ./internal/auth/... -v -run TestMiddleware` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AUTH-01 | `/mcp` returns 401 with missing token | unit | `go test ./internal/auth/... -run TestMiddleware_NoToken` | ❌ Wave 0 |
| AUTH-01 | `/mcp` returns 401 with invalid token | unit | `go test ./internal/auth/... -run TestMiddleware_InvalidToken` | ❌ Wave 0 |
| AUTH-01 | `/mcp` returns 503 before JWKS loaded | unit | `go test ./internal/auth/... -run TestMiddleware_NotLoaded` | ❌ Wave 0 |
| AUTH-02 | JWKS re-fetched on unknown kid | manual | Authelia key rotation test | — |
| AUTH-03 | 401 includes `WWW-Authenticate: Bearer resource_metadata=...` | unit | `go test ./internal/auth/... -run TestMiddleware_401Header` | ❌ Wave 0 |
| AUTH-04 | `/.well-known/oauth-protected-resource` returns 200 without auth | integration | `go test ./... -run TestPRMEndpoint_NoAuth` | ❌ Wave 0 |
| AUTH-05 | Token with wrong `iss` rejected | unit | `go test ./internal/auth/... -run TestMiddleware_WrongIssuer` | ❌ Wave 0 |
| AUTH-05 | Token with wrong `aud` rejected | unit | `go test ./internal/auth/... -run TestMiddleware_WrongAudience` | ❌ Wave 0 |
| SRV-03 | PRM JSON contains `authorization_servers` | integration | `go test ./... -run TestPRMEndpoint_Content` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/auth/... -run TestMiddleware`
- **Per wave merge:** `go test ./...`
- **Phase gate:** `go test ./...` green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/auth/middleware_test.go` — all auth unit tests (listed above)
- [ ] Integration test helper: test JWKS server (in-process `httptest.NewServer`) serving a test RSA public key
- [ ] Framework already present: Go stdlib testing — no install needed

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | JWT bearer validation via keyfunc/v3 + golang-jwt/jwt/v5 |
| V3 Session Management | no | Stateless JWT; no server-side session |
| V4 Access Control | yes | Mux routing enforces `/mcp` requires auth; `/.well-known/` is public |
| V5 Input Validation | yes | `jwt.Parse` validates structure; `WithExpirationRequired` + `WithIssuer` + `WithAudience` |
| V6 Cryptography | yes | RS256 signature verification via JWKS — never hand-rolled |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Bearer token replay (within TTL) | Spoofing | Accept as architectural tradeoff; Authelia 1h token lifetime is standard |
| Expired token accepted | Spoofing | `jwt.WithExpirationRequired()` + `jwt.WithLeeway(10s)` |
| Wrong issuer token accepted | Spoofing | `jwt.WithIssuer(cfg.AutheliaBaseURL)` exact-string match |
| Token for different resource | Spoofing | `jwt.WithAudience(cfg.AutheliaClientID)` |
| JWKS cache poisoning | Tampering | keyfunc fetches only from `cfg.AutheliaBaseURL + "/jwks.json"` — no user-controllable input |
| 401→OAuth flow on 503 | Confusion | 503 omits `WWW-Authenticate` header (Gotcha 5) |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `keyfunc.NewDefaultOverrideCtx` has `noErrorReturnFirstHTTPReq = true` as hardcoded default (not just configurable) | §1, Code Examples | If wrong, startup still blocks on first fetch; would need explicit `NoErrorReturnFirstHTTPReq: boolPtr(true)` in Override |
| A2 | `waitForLoad` HTTP probe correctly tracks keyfunc's internal load state | §1, Code Examples | If keyfunc fetches slower than probe succeeds, first few requests may still fail with key-not-found; add small sleep or verify via Storage API instead |
| A3 | `golang.org/x/time` will be pulled as a transitive dep of keyfunc/v3 | §5 | If not, add explicit `go get golang.org/x/time` |
| A4 | `min()` builtin is available in Go 1.26 | Code Examples | Go 1.21+ added `min`/`max` builtins; Go 1.26 has them. No risk. |

**A1 is the highest-risk assumption.** The source reading was from a WebFetch of GitHub (not a local file), so it is VERIFIED at MEDIUM confidence. The planner should add a code-compilation step that confirms the behavior.

---

## Open Questions

1. **`waitForLoad` probe vs keyfunc load timing**
   - What we know: The HTTP probe to `/jwks.json` confirms Authelia is reachable, but keyfunc's internal goroutine fetches independently. There is a small race between "probe succeeded" and "keyfunc has parsed and stored keys."
   - What's unclear: How long after a successful HTTP probe does keyfunc's storage have valid keys?
   - Recommendation: After setting `v.loaded.Store(true)`, the first few requests may still get key-not-found errors if keyfunc's goroutine hasn't run yet. To be safe, add a 500ms sleep after setting `loaded`, or use `kf.Storage().KeyRead` to probe instead of raw HTTP. This is a detail for the planner to decide in the implementation task.

2. **Trailing-slash mux behavior with `http.ServeMux` in Go 1.22+**
   - What we know: Go 1.22 enhanced `http.ServeMux` with method-based and path-parameter routing.
   - What's unclear: Does the new pattern-based mux handle `/mcp` and `/mcp/` differently than before?
   - Recommendation: Register both `/mcp` and `/mcp/` explicitly to be safe, regardless of Go version behavior.

---

## Sources

### Primary (HIGH confidence)
- `/home/thamw/go/pkg/mod/github.com/mark3labs/mcp-go@v0.54.1/server/protected_resource.go` — ProtectedResourceMetadataConfig fields, ProtectedResourceMetadataPath logic, NewProtectedResourceMetadataHandler behavior
- `/home/thamw/go/pkg/mod/github.com/mark3labs/mcp-go@v0.54.1/server/streamable_http.go` — WithProtectedResourceMetadata option, ServeHTTP PRM intercept at line 326-329
- `https://pkg.go.dev/github.com/MicahParks/keyfunc/v3` — Exported API: constructors, Override struct, Keyfunc interface
- `https://pkg.go.dev/github.com/MicahParks/jwkset#HTTPClientStorageOptions` — NoErrorReturnFirstHTTPReq semantics

### Secondary (MEDIUM confidence)
- `https://github.com/MicahParks/keyfunc/blob/master/keyfunc.go` — Source of `NewDefaultOverrideCtx` showing `noErrorReturnFirstHTTPReq := true` default (WebFetch of GitHub rendered HTML)
- `https://pkg.go.dev/github.com/MicahParks/keyfunc/v3?tab=versions` — v3.8.0 latest version confirmed
- `https://pkg.go.dev/github.com/golang-jwt/jwt/v5?tab=versions` — v5.3.1 latest, CVE note for v5.0–v5.2.1

### Tertiary (LOW confidence — marked [ASSUMED])
- Exponential backoff parameters (1s initial, 30s cap, factor 2.0) — standard Go community pattern, not from authoritative source
- `waitForLoad` goroutine timing relative to keyfunc internal fetch — inferred from API behavior, not benchmarked

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — keyfunc/v3 and mcp-go APIs verified from installed source and pkg.go.dev
- Architecture: HIGH — mux routing pattern derived directly from mcp-go@v0.54.1 source
- Pitfalls: HIGH — most from source code inspection; §Gotcha 6 and 7 from reference doc (MEDIUM)
- Backoff implementation: LOW (ASSUMED) — stdlib pattern, no authoritative benchmark

**Research date:** 2026-05-28
**Valid until:** 2026-06-28 (keyfunc/v3 is active, check for v3.9+ if planning beyond this date)
