# Phase 1 Plan: Core Server + Image Generation Tool

**Requirements:** SRV-01, SRV-02, SRV-04, TOOL-01, TOOL-02, TOOL-03, TOOL-04, TOOL-05, CFG-01, CFG-02, CFG-03, CFG-04, CFG-05

**Done when:** `curl` to `/mcp` with a valid MCP initialize + tools/call request returns a base64 image in `ImageContent`.

---

## Overview

Four sequential tasks build the server bottom-up, matching the import graph: dependencies first, then consumers.

1. **Add Go module dependencies** — `go get` the three packages; nothing else compiles without them.
2. **Config package** — `internal/config/config.go` + tests. Tools and main.go both import config; it must exist first.
3. **Tool package** — `internal/tools/generate_visuals.go` + `internal/auth/` scaffold + tests. Imports config; must exist before main.go.
4. **main.go + integration smoke test** — wire MCP server, Streamable HTTP transport, and `net/http.Server`; verify with `go build ./...` and a live curl call.

Each task is independently verifiable before the next begins.

---

## Threat Model

**Trust boundary:** The `/mcp` HTTP endpoint is unauthenticated in Phase 1 (localhost-only by design; Phase 2 adds Authelia JWT). The only external trust boundary is outbound: the tool calls the Gemini API with a secret key.

| Threat | STRIDE Category | Disposition | Mitigation |
|--------|----------------|-------------|------------|
| `GEMINI_API_KEY` logged in error output | Information Disclosure | Mitigate | Never pass `cfg.GeminiAPIKey` to any `log.*` call; log only its presence/absence (e.g., `"GEMINI_API_KEY set: true"`) |
| Prompt injection via `image_prompt` (malicious input triggers unintended Gemini behaviour) | Tampering | Accept | Gemini applies its own content policy server-side; no additional server-side sanitization required for Phase 1 |
| Panic-induced server crash / DoS | Denial of Service | Mitigate | `server.WithRecovery()` at transport level (D-08) + `defer/recover` inside tool handler |
| Unbounded request body / slow-body attack | Denial of Service | Accept | `ReadTimeout: 30s` on `http.Server` limits slow-body attacks; `MaxBytesReader` deferred to Phase 2 if needed |
| Package supply-chain tampering | Tampering | Mitigate | All three packages verified against `proxy.golang.org` (see RESEARCH.md Package Legitimacy Audit — all Approved) |

---

## Tasks

---

### Task 1: Add Go module dependencies

**File(s):**
- `go.mod` (updated by `go get`)
- `go.sum` (created by `go get`)

**What:**

Run the following three commands from the repo root. These are the exact versions verified in RESEARCH.md against `proxy.golang.org`. Do not use `@latest` — pin the versions exactly.

```
go get github.com/mark3labs/mcp-go@v0.54.1
go get google.golang.org/genai@v1.58.0
go get github.com/joho/godotenv@v1.5.1
```

After all three `go get` calls complete, run `go mod tidy` to remove any transitive noise and ensure `go.sum` is clean.

Do not modify `go.mod` by hand. Do not add any other packages. Do not use the deprecated `github.com/google/generative-ai-go` package — it reached EOL August 2025 and uses a different type-assertion API incompatible with this plan.

**Acceptance:**

- `go.mod` contains `require` entries for all three packages at the exact specified versions.
- `go.sum` exists and is non-empty.
- `go build ./...` passes (will only build the empty module root at this point — no source files yet).

---

### Task 2: Config package (`internal/config/`)

**File(s):**
- `internal/config/config.go`
- `internal/config/config_test.go`

**What:**

Create `internal/config/config.go` with package `config`. Implement a `Config` struct and a `Load()` function that applies decisions D-04 and D-05:

**`Config` struct fields:**

```
GeminiAPIKey    string   // required — CFG-01, CFG-05
ListenAddr      string   // optional, default ":8080" — SRV-01
PublicBaseURL   string   // optional — Phase 2: MCP_PUBLIC_URL env var
AutheliaBaseURL string   // optional — Phase 2: AUTHELIA_URL env var
AutheliaClientID string  // optional — Phase 2: AUTHELIA_CLIENT_ID env var
```

**`Load()` function behaviour:**

1. Call `_ = godotenv.Load()` first — per D-04/D-05: this loads `.env` without overriding already-set env vars, and silently skips if `.env` is absent (correct for containers/CI). Do NOT use `godotenv.Overload()` — that would violate D-05 by overwriting live env vars.
2. Read all fields via `os.Getenv(...)`.
3. For `ListenAddr`: use a local `getenv(key, fallback string) string` helper that returns the fallback when the env var is unset or empty. Env var name: `LISTEN_ADDR`. Default: `":8080"`.
4. For `MCP_PUBLIC_URL`, `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`: read via `os.Getenv`, leave empty string if unset (unused in Phase 1 but scaffolded per CFG-04 and Phase 2 integration).
5. After reading all env vars, check `cfg.GeminiAPIKey == ""`. If true: `log.Fatal("GEMINI_API_KEY is required but not set. Set it in environment or .env file.")` — per CFG-05. Do not proceed with an empty key.
6. Return `*Config`.

**Security constraint:** Do NOT log the value of `GeminiAPIKey` anywhere in this file or any file. If you need to log key presence for debugging, log `"GEMINI_API_KEY: set"` or `"GEMINI_API_KEY: missing"` only.

**`internal/config/config_test.go`** — cover these cases using Go stdlib `testing` (no external test framework):

- `TestConfigLoad_FromEnv`: Set `GEMINI_API_KEY=test-key` and `LISTEN_ADDR=:9090` via `t.Setenv`; call `Load()`; assert `cfg.GeminiAPIKey == "test-key"` and `cfg.ListenAddr == ":9090"`.
- `TestConfigLoad_DefaultListenAddr`: Set only `GEMINI_API_KEY=test-key`; call `Load()`; assert `cfg.ListenAddr == ":8080"`.
- `TestConfigLoad_MissingKey`: Do NOT set `GEMINI_API_KEY`; assert that `Load()` calls `log.Fatal`. Capture this with a subprocess test: use `os/exec` to run `go test -run TestConfigLoad_MissingKey_Fatal` in a subprocess and assert the exit code is non-zero. Alternatively, refactor `Load()` to accept a `fatalFn func(string)` for testability — your discretion on the pattern, but the fatal-on-missing behaviour must be tested.
- `TestConfigLoad_EnvWinsOverDotenv`: Demonstrates D-05: set `GEMINI_API_KEY=from-env` in the process env, create a temporary `.env` file with `GEMINI_API_KEY=from-dotenv`, point the test to load it (may need to `chdir` or pass the path), and assert `cfg.GeminiAPIKey == "from-env"`.

**Acceptance:**

- `go test ./internal/config/... -v` passes with all four test cases green.
- `go vet ./internal/config/...` emits no warnings.

---

### Task 3: Tool package + auth scaffold (`internal/tools/` + `internal/auth/`)

**File(s):**
- `internal/tools/generate_visuals.go`
- `internal/tools/generate_visuals_test.go`
- `internal/auth/doc.go`

**What:**

**`internal/auth/doc.go`** — create this file with package `auth` and a single doc comment:

```go
// Package auth provides JWT Bearer token validation middleware for the MCP server.
// Phase 1: empty placeholder. Phase 2 implements Authelia OAuth validation here.
package auth
```

This scaffolds the package so Phase 2 can add `middleware.go` without creating the package from scratch, matching the layout in `mcp-go-authelia.md §4`.

---

**`internal/tools/generate_visuals.go`** — package `tools`. Implement the tool definition and handler.

**`generateVisualsDescription` const (D-03):**

```go
// TODO: write final description
const generateVisualsDescription = `Generates an image from a text prompt using the Gemini image model.
Returns a base64-encoded image in ImageContent format. Use for creating illustrations,
diagrams, and visual assets from natural language descriptions. Do not use for editing
existing images — this tool creates new images from scratch.`
```

The `// TODO:` comment precedes the const declaration as a reminder that the description text is a placeholder for prompt engineering. This matches D-03 exactly.

**`Register(s *server.MCPServer, cfg *config.Config)` function:**

Call `s.AddTool(...)` with:

- Tool name: `"generate_visuals"` (TOOL-01)
- `mcp.WithDescription(generateVisualsDescription)`
- `mcp.WithString("image_prompt", mcp.Required(), mcp.Description("Text description of the image to generate."))` — per D-09 (Required, no maxLength, parameter name `image_prompt`)
- Annotations per D-10:
  - `mcp.WithOpenWorldHint(true)` — calls Gemini externally
  - `mcp.WithReadOnlyHint(false)`
  - `mcp.WithDestructiveHint(false)`
  - `mcp.WithIdempotentHint(false)` — two calls = two API charges
  - Attempt `mcp.WithTitleAnnotation("Generate Visual")` — if this function does not exist in mcp-go v0.54.1, fall back to setting the title via `mcp.WithAnnotations(mcp.ToolAnnotations{Title: "Generate Visual"})`. Check the package at implementation time.
- Handler: `makeGenerateVisualsHandler(cfg)`

**`makeGenerateVisualsHandler(cfg *config.Config) server.ToolHandlerFunc`:**

Returns a closure. Inside the closure body, implement D-08's panic recovery pattern: wrap the core logic in an inner anonymous function with `defer/recover`. On panic: log the panic value with `log.Printf("generate_visuals panic: %v", r)` and return `mcp.NewToolResultError(...)` with the three-part error message (what failed, likely cause, recovery suggestion). Call the inner function as `doGenerateVisuals(ctx, cfg, req)`.

**`doGenerateVisuals(ctx context.Context, cfg *config.Config, req mcp.CallToolRequest) *mcp.CallToolResult`:**

Steps, each with its specific error handling:

1. Extract `image_prompt` via `req.RequireString("image_prompt")`. On error: return `mcp.NewToolResultError("image_prompt parameter is required but was not provided. Provide a text description of the image to generate.")` — no Go error return, per anti-pattern note in RESEARCH.md §Pitfall 5.

2. Create Gemini client per-request (per RESEARCH.md open question resolution: per-request for Phase 1):
   ```go
   client, err := genai.NewClient(ctx, &genai.ClientConfig{
       APIKey:  cfg.GeminiAPIKey,
       Backend: genai.BackendGeminiAPI,
   })
   ```
   On error: return `mcp.NewToolResultError(fmt.Sprintf("Failed to initialize Gemini client. Likely cause: invalid or missing API key. Recovery: verify GEMINI_API_KEY is correct and has image generation permissions. Error: %v", err))` — per D-06 three-part template.
   On success: `defer client.Close()`.

3. Call `client.Models.GenerateContent(ctx, "gemini-3.1-flash-image-preview", genai.Text(prompt), &genai.GenerateContentConfig{ResponseModalities: []string{string(genai.ModalityText), string(genai.ModalityImage)}})`.
   - If `genai.ModalityText` / `genai.ModalityImage` constants are not exported in v1.58.0, use raw strings `"TEXT"` and `"IMAGE"` instead (assumption A1 in RESEARCH.md).
   - On error: return `mcp.NewToolResultError(fmt.Sprintf("Gemini image generation failed. Likely cause: quota exceeded, content policy violation, or network error. Retry once; if persistent, simplify the prompt. Error: %v", err))` — per D-06.

4. Check `len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil`. If true: return D-07 error — `mcp.NewToolResultError("Gemini returned a response but no image data was generated. This may be due to content policy restrictions. Try rephrasing the prompt.")`.

5. Iterate `resp.Candidates[0].Content.Parts`. For each part: check `part.InlineData != nil && len(part.InlineData.Data) > 0`. When found: base64-encode with `base64.StdEncoding.EncodeToString(part.InlineData.Data)`, use `part.InlineData.MIMEType` as mime type (fallback to `"image/png"` if empty), return `mcp.NewToolResultImage("", imgBase64, mimeType)`. Continue iterating past text parts without returning early — Gemini often emits a text caption alongside the image (RESEARCH.md §Pitfall 2).

6. If the loop completes without finding any `InlineData`: return the D-07 error message again.

**Imports required:** `context`, `encoding/base64`, `fmt`, `log`, `github.com/mark3labs/mcp-go/mcp`, `github.com/mark3labs/mcp-go/server`, `github.com/thamwangjun/generate-visuals-mcp/internal/config`, `google.golang.org/genai`.

---

**`internal/tools/generate_visuals_test.go`** — package `tools_test` (external test package). Cover these cases:

- `TestToolRegistered`: Create a `server.NewMCPServer(...)`, call `Register(s, cfg)` with a dummy config, then call `s.ListTools(ctx, ...)` or inspect the server's tool list and assert `generate_visuals` is present. If `ListTools` is not a direct method, use `s.HandleMessage` with a `tools/list` JSON-RPC message and unmarshal the response.
- `TestMissingImagePrompt`: Construct a `mcp.CallToolRequest` with no `image_prompt` argument. Call the handler directly (via `makeGenerateVisualsHandler` — export it as `MakeGenerateVisualsHandler` for test access, or test through the server). Assert the result has `IsError: true` and the text mentions `image_prompt`.
- `TestGeminiClientError`: Provide a config with `GeminiAPIKey: "invalid-key-that-will-not-connect"` and call `doGenerateVisuals` with a cancelled context (so the Gemini client creation or call fails immediately). Assert `IsError: true` and the message contains "Likely cause".
- `TestPanicRecovery`: Trigger a panic inside a handler by temporarily replacing `doGenerateVisuals` with a function that panics, or by calling the handler with a crafted input. Assert the panic is caught and the result has `IsError: true`.

Note: TOOL-03 and TOOL-04 (live Gemini API call returning real image bytes) require `GEMINI_API_KEY` set in the environment. Gate these with `if os.Getenv("GEMINI_API_KEY") == "" { t.Skip("GEMINI_API_KEY not set") }`. Name these `TestGenerateVisualsIntegration`.

**Acceptance:**

- `go test ./internal/tools/... -v -run 'TestToolRegistered|TestMissingImagePrompt|TestGeminiClientError|TestPanicRecovery'` passes (no API key required).
- `go test ./internal/auth/... -v` passes (package compiles, no test files needed yet — just confirms the package builds).
- `go vet ./internal/...` emits no warnings.

---

### Task 4: main.go + build verification + integration smoke test

**File(s):**
- `main.go`

**What:**

Create `main.go` at the repo root (package `main`). Wire the MCP server, Streamable HTTP transport, and `net/http.Server` following the architecture pattern in RESEARCH.md §Pattern 1.

**Exact structure:**

1. Import `internal/config` and `internal/tools`.
2. Call `cfg := config.Load()` — fails fast per CFG-05 if `GEMINI_API_KEY` is missing.
3. Create MCP server per D-11: `server.NewMCPServer("generate-visuals-mcp", "1.0.0", server.WithToolCapabilities(true), server.WithRecovery())`.
4. Call `tools.Register(mcpServer, cfg)`.
5. Create the HTTP handler as a **named variable** `httpHandler` — this is the Phase 2 integration point (per CONTEXT.md §Integration Points and D-02): `httpHandler := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))`.
6. Add a comment immediately after line 5: `// Phase 2: wrap httpHandler with auth middleware here — see internal/auth/`.
7. Construct `net/http.Server` with:
   - `Addr: cfg.ListenAddr`
   - `Handler: httpHandler`
   - `ReadTimeout: 30 * time.Second`
   - `WriteTimeout: 120 * time.Second` — required for Gemini image generation (10–60s); using mcp-go's `Start()` helper would impose its own defaults, so construct `http.Server` manually (RESEARCH.md §Pitfall 3)
   - `IdleTimeout: 120 * time.Second`
8. Log: `log.Printf("generate-visuals-mcp listening on %s", cfg.ListenAddr)`.
9. Call `httpSrv.ListenAndServe()`. On non-nil error: `log.Fatalf("server error: %v", err)`.

Do NOT use `server.NewStreamableHTTPServer(...).Start(addr)` — that pattern does not allow custom `WriteTimeout`.

**Required imports:** `log`, `net/http`, `time`, `github.com/mark3labs/mcp-go/server`, `github.com/thamwangjun/generate-visuals-mcp/internal/config`, `github.com/thamwangjun/generate-visuals-mcp/internal/tools`.

---

**Build verification (run after writing main.go):**

```
go build ./...
go vet ./...
```

Both must exit 0. Fix any compilation or vet errors before proceeding to the smoke test.

---

**Integration smoke test (requires `GEMINI_API_KEY` to be set):**

Start the server in one terminal:
```
GEMINI_API_KEY=<your-key> go run .
```

In a second terminal, send an MCP `initialize` followed by a `tools/call`:

```bash
# Step 1: initialize (establishes the MCP session)
curl -s -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-11-25",
      "capabilities": {},
      "clientInfo": {"name": "smoke-test", "version": "0.1"}
    }
  }'
```

Expected: JSON response with `result.serverInfo.name == "generate-visuals-mcp"` and `result.serverInfo.version == "1.0.0"` (SRV-04).

```bash
# Step 2: call generate_visuals
curl -s -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "generate_visuals",
      "arguments": {"image_prompt": "a small red circle on a white background"}
    }
  }'
```

Expected: JSON response with `result.content[0].type == "image"` and `result.content[0].data` being a non-empty base64 string. The `isError` field must be absent or `false`.

**Acceptance:**

- `go build ./...` exits 0.
- `go vet ./...` exits 0.
- `go test ./...` exits 0 (unit tests; integration tests skip if `GEMINI_API_KEY` is not set in the test environment).
- Live `curl` smoke test returns `result.content[0].type == "image"` with non-empty base64 data in `result.content[0].data` (TOOL-04).
- Server startup log shows `generate-visuals-mcp listening on :8080` (SRV-01).
- `initialize` response has `serverInfo.name == "generate-visuals-mcp"` and `serverInfo.version == "1.0.0"` (SRV-04).

---

## Requirements Coverage

| Requirement | Covered By |
|-------------|-----------|
| SRV-01 | Task 4 — `ListenAddr` from config, `http.Server` with default `:8080` |
| SRV-02 | Task 4 — `server.NewStreamableHTTPServer` with `WithEndpointPath("/mcp")` |
| SRV-04 | Task 4 — `server.NewMCPServer("generate-visuals-mcp", "1.0.0", ...)` |
| TOOL-01 | Task 3 — `generate_visuals` registered with description + annotations |
| TOOL-02 | Task 3 — `mcp.WithString("image_prompt", mcp.Required(), ...)` |
| TOOL-03 | Task 3 — `client.Models.GenerateContent(..., "gemini-3.1-flash-image-preview", ...)` |
| TOOL-04 | Task 3 — `mcp.NewToolResultImage("", imgBase64, mimeType)` |
| TOOL-05 | Task 3 — `mcp.NewToolResultError(...)` on all Gemini error paths (D-06, D-07) |
| CFG-01 | Task 2 — `os.Getenv("GEMINI_API_KEY")` |
| CFG-02 | Task 2 — `_ = godotenv.Load()` before reading env vars (D-04) |
| CFG-03 | Task 2 — `godotenv.Load()` does not override already-set env vars (D-05) |
| CFG-04 | Task 2 — `LISTEN_ADDR`, `MCP_PUBLIC_URL`, `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID` fields in Config |
| CFG-05 | Task 2 — `log.Fatal(...)` if `GeminiAPIKey == ""` after loading |

**Out of scope for Phase 1 (Phase 2):** SRV-03, AUTH-01–AUTH-05.

---

## File Map

```
generate-visuals-mcp/
├── main.go                               # Task 4
├── internal/
│   ├── config/
│   │   ├── config.go                     # Task 2
│   │   └── config_test.go                # Task 2
│   ├── tools/
│   │   ├── generate_visuals.go           # Task 3
│   │   └── generate_visuals_test.go      # Task 3
│   └── auth/
│       └── doc.go                        # Task 3 (Phase 2 scaffold)
├── go.mod                                # Task 1 (updated)
└── go.sum                                # Task 1 (created)
```
