---
phase: 02-authelia-oauth-protection
reviewed: 2026-05-29T00:00:00Z
depth: standard
files_reviewed: 7
files_reviewed_list:
  - go.mod
  - internal/auth/middleware.go
  - internal/auth/middleware_test.go
  - internal/config/config.go
  - internal/config/config_test.go
  - main.go
  - main_test.go
findings:
  critical: 3
  warning: 3
  info: 2
  total: 8
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-05-29T00:00:00Z
**Depth:** standard
**Files Reviewed:** 7
**Status:** issues_found

## Summary

Phase 2 wires Authelia OAuth protection into the MCP server via JWT bearer-token validation backed by a JWKS endpoint. The overall architecture is sound: async JWKS loading with 503-until-ready, claim validation (issuer, audience, expiry with leeway), and correct WWW-Authenticate header omission on 503. Three blockers were found: a TOCTOU race between the probe goroutine and keyfunc's internal key storage (valid tokens silently get 401 during the startup window), a missing algorithm whitelist that leaves an algorithm-substitution gap when the JWKS omits per-key `alg` fields, and four config fail-fast tests that pass for the wrong reason due to a broken subprocess working directory (the subprocess exits non-zero because there are no Go files in the target directory, not because `log.Fatal` fires). Three warnings cover a missing HTTP timeout on the probe, internal error detail leakage into JSON responses, and no graceful shutdown wiring.

---

## Critical Issues

### CR-01: TOCTOU race — `loaded=true` set before keyfunc has populated its key storage

**File:** `internal/auth/middleware.go:77-83`

**Issue:** `waitForLoad` performs its own independent `http.Get(jwksURL)` probe. When that probe returns HTTP 200, it sleeps 200 ms unconditionally and then calls `v.loaded.Store(true)`. The 200 ms sleep is not a synchronization primitive — it is a guess. The keyfunc library maintains its own background goroutine that fetches and parses the JWKS independently. There is no happens-before relationship between keyfunc's internal parse-and-store completing and `loaded` flipping to `true`. Under CPU pressure, slow JWKS parsing, or cold startup, valid bearer tokens are presented during this window and rejected with `401 invalid or expired token` (keyfunc returns "key not found"), when the semantically correct response is `503` (not yet ready). The caller sees a hard authentication failure with no indication that the service is still initializing.

The probe and keyfunc's fetch are also entirely separate HTTP connections. The JWKS server can return 200 to the probe while keyfunc's own in-flight request is still pending, making the race window arbitrarily wide under load.

**Fix:** Eliminate the independent probe and instead poll keyfunc's own storage for key presence. The `keyfunc.Keyfunc` value returned by `NewDefaultOverrideCtx` implements `jwkset.Storage` access. Use `kf.Storage().KeyReadAll(ctx)` and only set `loaded=true` once at least one key is present:

```go
func (v *Validator) waitForLoad(ctx context.Context) {
    delay := time.Second
    const maxDelay = 30 * time.Second
    attempt := 0
    for {
        select {
        case <-ctx.Done():
            return
        case <-time.After(delay):
        }
        attempt++
        keys, err := v.kf.Storage().KeyReadAll(ctx)
        if err == nil && len(keys) > 0 {
            v.loaded.Store(true)
            log.Printf("auth: JWKS ready (attempt %d)", attempt)
            return
        }
        log.Printf("auth: JWKS not ready (attempt %d, next retry in %s): %v", attempt, delay, err)
        delay = min(delay*2, maxDelay)
    }
}
```

This eliminates the independent HTTP probe, the 200 ms magic sleep, and the race. If `keyfunc/v3` does not directly expose `Storage()` on the `Keyfunc` interface, store the underlying `jwkset.Storage` separately during `NewValidatorAsync`.

---

### CR-02: Missing algorithm whitelist — algorithm-substitution attack if JWKS omits per-key `alg` field

**File:** `internal/auth/middleware.go:112-123`

**Issue:** `jwt.Parse` is called without `jwt.WithValidMethods(...)`. The keyfunc library checks the JWK's `alg` field against the token's `alg` header (keyfunc.go line 241), but only when the JWK explicitly carries an `alg` parameter: `if a := jwk.Marshal().ALG.String(); a != "" && a != alg`. The `alg` parameter is optional in RFC 7517 §4.4. If the live Authelia JWKS omits `alg` from a key entry, the check is skipped entirely and keyfunc returns the raw RSA public key for any algorithm the token claims. An attacker who presents a token with `"alg": "HS256"` signed using the RSA public key bytes as an HMAC secret could potentially pass signature verification on certain key sizes and jwt library versions. RFC 8725 §3.1 mandates that validators explicitly restrict the accepted algorithm set.

**Fix:** Add `jwt.WithValidMethods` to the parse call:

```go
token, err := jwt.Parse(
    tokenString,
    v.kf.Keyfunc,
    jwt.WithIssuer(v.issuer),
    jwt.WithAudience(v.audience),
    jwt.WithExpirationRequired(),
    jwt.WithLeeway(10*time.Second),
    jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
)
```

Restrict to the specific algorithm(s) Authelia is configured to use (e.g., `"RS256"` only) if known. This defense layer is independent of JWKS key content.

---

### CR-03: Config fail-fast subprocess tests pass for the wrong reason — zero coverage of actual fatal behavior

**File:** `internal/config/config_test.go:39` (and lines 67, 96, 125)

**Issue:** All four subprocess-test parent functions set `cmd.Dir = filepath.Join("..")`. When `go test` runs the `config` package, the working directory is `internal/config/`. `filepath.Join("..")` with one argument returns the string `".."`, which the OS resolves relative to the test binary's working directory — resolving to `internal/`, a directory that contains no Go source files. The subprocess command is `go test -run TestConfigLoad_MissingKey_Fatal -v .`, which fails with:

```
no Go files in .../generate-visuals-mcp/internal
FAIL  . [setup failed]
```

The parent test checks only that `cmd.ProcessState.ExitCode() != 0` — which is true, but the exit is a build failure, not a `log.Fatal` triggered by missing config. The actual fail-fast paths in `config.Load()` are never executed by any of these four tests. If all four `log.Fatal` calls were deleted from `config.go`, all four tests would still pass.

This was confirmed by running the subprocess command manually in `internal/`:
```
$ cd internal && go test -run TestConfigLoad_MissingKey_Fatal -v .
# .
no Go files in .../generate-visuals-mcp/internal
FAIL  . [setup failed]
```

**Fix:** Use the full module import path and remove `cmd.Dir`:

```go
func TestConfigLoad_MissingKey(t *testing.T) {
    cmd := exec.Command("go", "test",
        "-run", "TestConfigLoad_MissingKey_Fatal",
        "-v",
        "github.com/thamwangjun/generate-visuals-mcp/internal/config",
    )
    cmd.Env = append(os.Environ(), "GEMINI_API_KEY=", "CONFIG_TEST_SUBPROCESS=1")
    out, _ := cmd.CombinedOutput()
    if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() == 0 {
        t.Logf("output: %s", out)
        t.Error("expected non-zero exit code when GEMINI_API_KEY is missing")
    }
}
```

Apply the same fix to all four parent test functions (`TestConfigLoad_MissingKey`, `TestConfigLoad_MissingAutheliaURL`, `TestConfigLoad_MissingAutheliaClientID`, `TestConfigLoad_MissingPublicURL`).

---

## Warnings

### WR-01: `waitForLoad` probe uses `http.Get` with no timeout — goroutine can block indefinitely

**File:** `internal/auth/middleware.go:77`

**Issue:** `http.Get(jwksURL)` uses `http.DefaultClient`, which has no request timeout. If the JWKS server is reachable at the TCP layer but slow to respond (e.g., firewall accepts connections and drops packets), this call blocks indefinitely. The `waitForLoad` goroutine never returns, `loaded` is never set to `true`, and every subsequent request returns 503 forever with no log output to indicate the hang. This issue is also present in the CR-01 fix recommendation above, but exists independently as a robustness gap even if the probe is retained.

**Fix:** Use a dedicated client with an explicit timeout:

```go
probeClient := &http.Client{Timeout: 10 * time.Second}
resp, err := probeClient.Get(jwksURL)
```

---

### WR-02: `unauthorized` embeds internal error strings using `%q` — not guaranteed valid JSON

**File:** `internal/auth/middleware.go:147`

**Issue:** The response body is assembled with:
```go
fmt.Fprintf(w, `{"error":"invalid_token","error_description":%q}`, detail)
```

The `%q` verb produces Go-syntax double-quoted strings, not JSON strings. These overlap for ASCII-safe input but diverge for inputs containing non-ASCII characters, null bytes, or certain control characters — `%q` uses Go escape sequences (`\a`, `\b`, `\x..`) that are not valid JSON escape sequences. If `detail` ever originates from a library error message containing such bytes, the response body is invalid JSON. Additionally, `err.Error()` from `extractBearer` is currently a benign constant, but the call site pattern (`unauthorized(w, prmURL, err.Error())`) makes it easy for a future caller to accidentally pass an internal error string containing sensitive details.

**Fix:** Use `json.Marshal` for correct JSON string encoding and restrict caller-visible detail to fixed constants:

```go
func unauthorized(w http.ResponseWriter, prmURL, detail string) {
    detailJSON, _ := json.Marshal(detail) // always produces a valid JSON string
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+prmURL+`"`)
    w.WriteHeader(http.StatusUnauthorized)
    fmt.Fprintf(w, `{"error":"invalid_token","error_description":%s}`, detailJSON)
}
```

---

### WR-03: `main.go` passes `context.Background()` — background goroutines are never cancelled on shutdown

**File:** `main.go:30-34`

**Issue:** `context.Background()` is passed to `auth.NewValidatorAsync`, which forwards it to `keyfunc.NewDefaultOverrideCtx` (the keyfunc refresh goroutine) and to `waitForLoad`. This context is never cancelled. When the process receives `SIGTERM` or `SIGINT`, `httpSrv.ListenAndServe` returns with an error and `log.Fatalf` terminates the process — but the keyfunc refresh goroutine and `waitForLoad` (if still running during a transient JWKS outage) are abandoned without cleanup. There is also no graceful HTTP shutdown, so in-flight MCP requests are abruptly terminated during rolling restarts in a containerized environment.

**Fix:** Wire signal-aware context cancellation and graceful shutdown:

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
defer stop()

validator, err := auth.NewValidatorAsync(ctx, cfg)
if err != nil {
    log.Fatalf("auth: failed to initialize validator: %v", err)
}
// ... build mux ...
go func() {
    <-ctx.Done()
    shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _ = httpSrv.Shutdown(shutCtx)
}()

if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
    log.Fatalf("server error: %v", err)
}
```

---

## Info

### IN-01: Duplicate `/mcp` and `/mcp/` middleware instantiation

**File:** `main.go:47-48`

**Issue:** Two separate calls to `validator.Middleware(prmURL)(httpServer)` create two independent `http.Handler` closures over the same underlying handler. This works correctly but is redundant. Any future per-route middleware state (e.g., metrics counters, rate limiters) would need to be added to both independently, and a reader might assume the two registrations share a wrapper instance when they do not.

**Fix:** Create one wrapped handler and register it on both paths:

```go
protected := validator.Middleware(prmURL)(httpServer)
mux.Handle("/mcp", protected)
mux.Handle("/mcp/", protected)
```

---

### IN-02: Flaky timing assumption in 503 tests — `time.Sleep(50ms)` premise is incorrect

**File:** `internal/auth/middleware_test.go:163, 198`

**Issue:** `TestMiddleware_NotLoaded` and `TestMiddleware_503NoWWWAuthenticate` call `time.Sleep(50 * time.Millisecond)` to "let waitForLoad attempt and fail." The comment implies the sleep ensures the goroutine has fired. However, `waitForLoad` starts with `case <-time.After(delay)` where `delay = time.Second` — the goroutine does not make its first probe attempt for one full second. The 50 ms sleep does not cause the goroutine to do anything. The tests pass because `loaded` starts as `false` (the zero value of `atomic.Bool`) and simply has not been set yet — which is guaranteed without any sleep.

While the tests happen to be correct, the misleading comment and unnecessary sleep introduce maintenance confusion: if someone later removes the initial 1-second delay from `waitForLoad`, they might trust the 50 ms sleep as sufficient synchronization when it is not.

**Fix:** Remove the sleep and replace the comment with an accurate explanation:

```go
// loaded starts false (zero value of atomic.Bool).
// waitForLoad cannot set it to true within its initial 1-second back-off delay,
// so no synchronization is needed here.
```

---

_Reviewed: 2026-05-29T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
