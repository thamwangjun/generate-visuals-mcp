# Phase 1: Core Server + Image Generation Tool — Research

**Researched:** 2026-05-27
**Domain:** Go MCP server (`mark3labs/mcp-go`), Google GenAI SDK (`google.golang.org/genai`), image generation
**Confidence:** HIGH (all three critical packages verified against Go module proxy and official docs)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Use `internal/` packages — `internal/tools/`, `internal/config/`, `internal/auth/`. Auth package scaffolded but empty in Phase 1 to avoid refactoring in Phase 2.
- **D-02:** Entry point in `main.go` at project root. HTTP wiring lives in `main.go`; tool definitions in `internal/tools/`; config loading in `internal/config/`.
- **D-03:** The `generate_visuals` tool description is stored as a named `const generateVisualsDescription` in `internal/tools/` with a `// TODO: write final description` placeholder.
- **D-04:** Use `github.com/joho/godotenv` to load the `.env` file before reading env vars. Silently skip if `.env` is absent (expected in container/CI environments). Fail fast only when required env vars are missing after all sources are checked.
- **D-05:** Env var takes precedence over `.env` file value — `godotenv.Load()` called before reading, so an already-set env var wins.
- **D-06:** Gemini API errors follow the 3-part template: what failed, likely cause (quota, content policy, network), and a recovery suggestion ("retry once; if persistent, simplify the prompt"). Return via `mcp.NewToolResultError()` with `isError: true`.
- **D-07:** If Gemini returns success status but no image bytes, treat as an error: `"Gemini returned a response but no image data was generated. This may be due to content policy restrictions. Try rephrasing the prompt."` Return via `mcp.NewToolResultError()`.
- **D-08:** Every tool handler wraps its body in a `defer/recover` to catch panics and return a structured error instead of crashing the handler.
- **D-09:** `image_prompt` parameter: `Required()`, no `maxLength` constraint (Gemini handles prompt length server-side). Parameter name is `image_prompt` per REQUIREMENTS.md TOOL-02.
- **D-10:** Tool annotations: `openWorldHint: true`, `readOnlyHint: false`, `destructiveHint: false`, `idempotentHint: false`.
- **D-11:** Server name: `"generate-visuals-mcp"`, version: `"1.0.0"` in the MCP `initialize` response.

### Claude's Discretion

None defined.

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SRV-01 | MCP server starts and listens on a configurable port (default `:8080`) | `server.NewStreamableHTTPServer` + `Start(addr)` pattern |
| SRV-02 | Server exposes `/mcp` endpoint using Streamable HTTP transport | `WithEndpointPath("/mcp")` option |
| SRV-04 | Server name and version reported correctly in MCP `initialize` response | `server.NewMCPServer("generate-visuals-mcp", "1.0.0", ...)` |
| TOOL-01 | `generate_visuals` tool registered with clear description | `mcp.NewTool(...)` + tool annotations |
| TOOL-02 | Tool accepts required `image_prompt` string parameter | `mcp.WithString("image_prompt", mcp.Required(), ...)` |
| TOOL-03 | Tool calls `gemini-3.1-flash-image-preview` via `google.golang.org/genai` | `client.Models.GenerateContent(ctx, "gemini-3.1-flash-image-preview", ...)` |
| TOOL-04 | Tool returns `ImageContent` in `CallToolResult` (base64-encoded) | `mcp.NewToolResultImage(...)` or manual `mcp.ImageContent{}` struct |
| TOOL-05 | Tool returns structured error on Gemini API failure | `mcp.NewToolResultError(...)` with `isError: true` |
| CFG-01 | `GEMINI_API_KEY` loaded from environment variable | `os.Getenv("GEMINI_API_KEY")` |
| CFG-02 | If not in env, loaded from `.env` file | `_ = godotenv.Load()` before reading env vars |
| CFG-03 | Environment variable takes precedence over `.env` file value | `godotenv.Load()` does not override already-set env vars |
| CFG-04 | Authelia base URL, client ID, listen address, public base URL configurable via env vars | Env var reading pattern in `internal/config/` |
| CFG-05 | Server fails fast with clear error if required config is missing | Explicit check after loading, `log.Fatalf(...)` |
</phase_requirements>

---

## Summary

This phase implements a Go MCP server using `mark3labs/mcp-go` v0.54.1 with the Streamable HTTP transport. The `generate_visuals` tool calls `gemini-3.1-flash-image-preview` via `google.golang.org/genai` v1.58.0 and returns base64-encoded image bytes as `mcp.ImageContent`.

All three critical packages are verified against the Go module proxy (authoritative source) and official documentation. The `google.golang.org/genai` SDK is the current unified Go SDK — the old `github.com/google/generative-ai-go` reached end-of-life August 31, 2025. The `mcp-go` package provides built-in `WithRecovery()` for panic handling at the server level, and the CONTEXT.md decision D-08 adds a belt-and-suspenders `defer/recover` inside the tool handler itself.

**Primary recommendation:** Use `server.NewMCPServer` + `server.NewStreamableHTTPServer` with `WithEndpointPath("/mcp")` and `WithRecovery()`. Call `client.Models.GenerateContent` with `ResponseModalities: []string{"TEXT", "IMAGE"}`. Extract image bytes from `part.InlineData.Data` and return via `mcp.NewToolResultImage("", base64str, mimeType)`.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| HTTP request routing | main.go (net/http) | mcp-go StreamableHTTPServer | `main.go` wires the port; `mcp-go` owns the `/mcp` path handler |
| MCP protocol (initialize, tools/list, tools/call) | mcp-go library | — | Library fully implements MCP 2025-11-25 spec |
| Tool registration | `internal/tools/` | mcp-go AddTool | Tool definition + handler live in `internal/tools/generate_visuals.go` |
| Config loading | `internal/config/` | main.go | Config struct loaded once in `internal/config/`, consumed by `main.go` and tool handler |
| Image generation | Gemini API (external) | `internal/tools/` | Tool handler constructs and fires the API call; response bytes returned |
| Auth scaffolding | `internal/auth/` | — | Empty package in Phase 1; Phase 2 wraps the HTTP handler |
| Panic recovery | mcp-go `WithRecovery()` | tool handler `defer/recover` | Two-layer: server-level catches any panics; handler-level returns structured MCP error |

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/mark3labs/mcp-go` | v0.54.1 | MCP server, Streamable HTTP transport, tool registration | The dominant Go MCP implementation; referenced in official MCP tooling docs |
| `google.golang.org/genai` | v1.58.0 | Gemini API client — GenerateContent, image output | Official Google Gen AI Go SDK; successor to EOL `generative-ai-go` |
| `github.com/joho/godotenv` | v1.5.1 | Load `.env` file for local development | Standard Go dotenv library; idiomatic approach in the ecosystem |

### Supporting (pulled in transitively)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket support (mcp-go SSE) | Pulled by mcp-go; no direct usage |
| `golang.org/x/crypto` | v0.36.0 | Crypto primitives | Transitive; no direct usage in Phase 1 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `google.golang.org/genai` | `github.com/google/generative-ai-go` | Old SDK reached EOL August 2025; do not use |
| `godotenv.Load()` ignore error | `godotenv.Overload()` | `Overload` overwrites existing env vars, violating D-05 |

**Installation:**
```bash
go get github.com/mark3labs/mcp-go@v0.54.1
go get google.golang.org/genai@v1.58.0
go get github.com/joho/godotenv@v1.5.1
```

---

## Package Legitimacy Audit

> slopcheck does not support Go modules (npm/PyPI focused). All packages verified against the Go module proxy (`proxy.golang.org`) — the authoritative registry for Go modules.

| Package | Registry | Age | Source Repo | proxy.golang.org | Disposition |
|---------|----------|-----|-------------|------------------|-------------|
| `github.com/mark3labs/mcp-go` | Go proxy | Published 2026-05-22 (v0.54.1) | github.com/mark3labs/mcp-go | Confirmed | Approved |
| `google.golang.org/genai` | Go proxy | Published 2026-05-22 (v1.58.0) | github.com/googleapis/go-genai | Confirmed | Approved |
| `github.com/joho/godotenv` | Go proxy | Published 2023-02-05 (v1.5.1) | github.com/joho/godotenv | Confirmed | Approved |

**Packages removed due to slopcheck verdict:** none  
**Packages flagged as suspicious:** none  

*slopcheck was unavailable for Go modules — all packages verified via `proxy.golang.org` (authoritative Go registry) instead. Source repos confirmed as canonical GitHub organizations.*

---

## Architecture Patterns

### System Architecture Diagram

```
Client (Claude Desktop / any MCP client)
    │
    │  POST /mcp  (JSON-RPC: tools/call)
    ▼
main.go — net/http.Server (port from LISTEN_ADDR env, default :8080)
    │
    │  http.Handler
    ▼
server.StreamableHTTPServer  (mcp-go)
    │  path: /mcp (WithEndpointPath)
    │  panic recovery: WithRecovery()
    ▼
server.MCPServer  (mcp-go — protocol layer)
    │  tools/call dispatch
    ▼
internal/tools/generate_visuals.go — generateVisualsHandler
    │  defer/recover (D-08)
    │  extract image_prompt param
    │  read GEMINI_API_KEY from internal/config/
    ▼
google.golang.org/genai  (Gemini API client)
    │  Models.GenerateContent(ctx, "gemini-3.1-flash-image-preview", ...)
    │  ResponseModalities: ["TEXT", "IMAGE"]
    ▼
Gemini API  (external — api.google.com)
    │
    │  response: Candidates[0].Content.Parts
    │  part.InlineData.Data ([]byte)  part.InlineData.MIMEType (string)
    ▼
mcp.NewToolResultImage("", base64str, mimeType)
    │  → *mcp.CallToolResult{Content: [ImageContent{...}]}
    ▼
Client receives ImageContent
```

### Recommended Project Structure

```
generate-visuals-mcp/
├── main.go                     # HTTP wiring, server startup, config loading
├── internal/
│   ├── config/
│   │   └── config.go           # Config struct, Load() func, env var reading
│   ├── tools/
│   │   └── generate_visuals.go # Tool def, handler, description const
│   └── auth/
│       └── auth.go             # Empty package — Phase 2 placeholder
├── go.mod
├── go.sum
└── .env                        # Local dev only — not committed
```

### Pattern 1: MCP Server Setup (main.go)

```go
// Source: mark3labs/mcp-go official docs (mcp-go.dev/transports/http/) + mcp-go-authelia.md reference
package main

import (
    "log"
    "net/http"
    "time"

    "github.com/mark3labs/mcp-go/server"
    "github.com/thamwangjun/generate-visuals-mcp/internal/config"
    "github.com/thamwangjun/generate-visuals-mcp/internal/tools"
)

func main() {
    cfg := config.Load() // fails fast if GEMINI_API_KEY missing

    // Create MCP server
    mcpServer := server.NewMCPServer(
        "generate-visuals-mcp",
        "1.0.0",
        server.WithToolCapabilities(true),
        server.WithRecovery(), // server-level panic recovery
    )

    // Register tools
    tools.Register(mcpServer, cfg)

    // Create Streamable HTTP transport — handler is a named var for Phase 2 middleware
    httpHandler := server.NewStreamableHTTPServer(
        mcpServer,
        server.WithEndpointPath("/mcp"),
    )

    // Phase 2: wrap httpHandler with auth middleware here, without touching tool code

    httpSrv := &http.Server{
        Addr:         cfg.ListenAddr,
        Handler:      httpHandler,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 120 * time.Second, // longer for image generation
        IdleTimeout:  120 * time.Second,
    }

    log.Printf("generate-visuals-mcp listening on %s", cfg.ListenAddr)
    if err := httpSrv.ListenAndServe(); err != nil {
        log.Fatalf("server error: %v", err)
    }
}
```

**Key design choice:** `httpHandler` is a named variable so Phase 2 can wrap it with `authMiddleware(httpHandler)` without changing any tool code (per D-02 integration point in CONTEXT.md).

### Pattern 2: Config Loading (internal/config/config.go)

```go
// Source: godotenv official docs (pkg.go.dev/github.com/joho/godotenv)
// Decision D-04, D-05: Load .env silently, env var takes precedence
package config

import (
    "fmt"
    "log"
    "os"

    "github.com/joho/godotenv"
)

type Config struct {
    GeminiAPIKey  string
    ListenAddr    string
    // Phase 2 fields (scaffolded, unused in Phase 1):
    PublicBaseURL   string
    AutheliaBaseURL string
    AutheliaClientID string
}

func Load() *Config {
    // D-04/D-05: Load .env BEFORE reading os.Getenv, but do NOT override already-set env vars.
    // godotenv.Load() silently skips if .env is absent — correct for container/CI.
    _ = godotenv.Load()

    cfg := &Config{
        GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
        ListenAddr:       getenv("LISTEN_ADDR", ":8080"),
        PublicBaseURL:    os.Getenv("MCP_PUBLIC_URL"),
        AutheliaBaseURL:  os.Getenv("AUTHELIA_URL"),
        AutheliaClientID: os.Getenv("AUTHELIA_CLIENT_ID"),
    }

    // CFG-05: Fail fast if required config missing
    if cfg.GeminiAPIKey == "" {
        log.Fatal("GEMINI_API_KEY is required but not set. Set it in environment or .env file.")
    }

    return cfg
}

func getenv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

**Why `_ = godotenv.Load()`:** `godotenv.Load()` returns an error when `.env` is absent. Discarding it silently is the standard pattern for environments where `.env` is intentionally absent (containers, CI). [VERIFIED: pkg.go.dev/github.com/joho/godotenv]

**Why env var wins over `.env`:** `godotenv.Load()` does NOT override env vars already set in the process environment. So calling it before `os.Getenv()` means the env var (set by the shell or container) always wins. This matches D-05. [VERIFIED: pkg.go.dev/github.com/joho/godotenv]

### Pattern 3: Tool Definition + Handler (internal/tools/generate_visuals.go)

```go
// Source: mark3labs/mcp-go examples/everything/main.go (VERIFIED), mcp-tool-best-practices.md
package tools

import (
    "context"
    "encoding/base64"
    "fmt"
    "log"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    "github.com/thamwangjun/generate-visuals-mcp/internal/config"
    "google.golang.org/genai"
)

// generateVisualsDescription is a named const so description text can be updated
// without touching handler logic (D-03).
const generateVisualsDescription = `// TODO: write final description
Generates an image from a text prompt using the Gemini image model.
Returns a base64-encoded image. Use for creating illustrations, diagrams,
and visual assets from natural language descriptions. Do not use for
editing existing images — this tool creates new images from scratch.`

func Register(s *server.MCPServer, cfg *config.Config) {
    s.AddTool(
        mcp.NewTool(
            "generate_visuals",
            mcp.WithDescription(generateVisualsDescription),
            mcp.WithString("image_prompt",
                mcp.Required(),
                mcp.Description("Text description of the image to generate."),
            ),
            // D-10: Tool annotations
            mcp.WithTitleAnnotation("Generate Visual"),
            mcp.WithOpenWorldHint(true),       // calls Gemini externally
            mcp.WithReadOnlyHint(false),
            mcp.WithDestructiveHint(false),
            mcp.WithIdempotentHint(false),     // two calls = two API charges
        ),
        makeGenerateVisualsHandler(cfg),
    )
}

func makeGenerateVisualsHandler(cfg *config.Config) server.ToolHandlerFunc {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // D-08: Catch all panics; return structured MCP error (not a crash)
        var result *mcp.CallToolResult
        func() {
            defer func() {
                if r := recover(); r != nil {
                    log.Printf("generate_visuals panic: %v", r)
                    result = mcp.NewToolResultError(
                        fmt.Sprintf("generate_visuals encountered an unexpected error: %v. "+
                            "This may be a transient issue. Retry once; if persistent, simplify the prompt.", r),
                    )
                }
            }()
            result = doGenerateVisuals(ctx, cfg, req)
        }()
        return result, nil
    }
}

func doGenerateVisuals(ctx context.Context, cfg *config.Config, req mcp.CallToolRequest) *mcp.CallToolResult {
    prompt, err := req.RequireString("image_prompt")
    if err != nil {
        return mcp.NewToolResultError(
            "image_prompt parameter is required but was not provided. " +
                "Provide a text description of the image to generate.",
        )
    }

    client, err := genai.NewClient(ctx, &genai.ClientConfig{
        APIKey:  cfg.GeminiAPIKey,
        Backend: genai.BackendGeminiAPI,
    })
    if err != nil {
        // D-06: 3-part error template — what failed, likely cause, recovery suggestion
        return mcp.NewToolResultError(fmt.Sprintf(
            "Failed to initialize Gemini client. "+
                "Likely cause: invalid or missing API key. "+
                "Recovery: verify GEMINI_API_KEY is correct and has image generation permissions. Error: %v", err,
        ))
    }
    defer client.Close()

    resp, err := client.Models.GenerateContent(
        ctx,
        "gemini-3.1-flash-image-preview",
        genai.Text(prompt),
        &genai.GenerateContentConfig{
            ResponseModalities: []string{
                string(genai.ModalityText),
                string(genai.ModalityImage),
            },
        },
    )
    if err != nil {
        // D-06: 3-part error template
        return mcp.NewToolResultError(fmt.Sprintf(
            "Gemini image generation failed. "+
                "Likely cause: quota exceeded, content policy violation, or network error. "+
                "Retry once; if persistent, simplify the prompt. Error: %v", err,
        ))
    }

    // Extract image bytes from response parts
    if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
        // D-07: no image bytes returned
        return mcp.NewToolResultError(
            "Gemini returned a response but no image data was generated. " +
                "This may be due to content policy restrictions. Try rephrasing the prompt.",
        )
    }

    for _, part := range resp.Candidates[0].Content.Parts {
        if part.InlineData != nil && len(part.InlineData.Data) > 0 {
            imgBase64 := base64.StdEncoding.EncodeToString(part.InlineData.Data)
            mimeType := part.InlineData.MIMEType
            if mimeType == "" {
                mimeType = "image/png" // Gemini default output format
            }
            // TOOL-04: Return ImageContent — helper creates CallToolResult with ImageContent
            return mcp.NewToolResultImage("", imgBase64, mimeType)
        }
    }

    // D-07: No InlineData found in any part
    return mcp.NewToolResultError(
        "Gemini returned a response but no image data was generated. " +
            "This may be due to content policy restrictions. Try rephrasing the prompt.",
    )
}
```

### Pattern 4: Returning ImageContent — Two Valid Approaches

```go
// Source: mark3labs/mcp-go examples/everything/main.go (VERIFIED via GitHub)

// Approach A — Use the helper (recommended, matches NewToolResultText pattern)
imgBase64 := base64.StdEncoding.EncodeToString(imageBytes)
return mcp.NewToolResultImage("", imgBase64, "image/png"), nil
// Signature: NewToolResultImage(text, imageData, mimeType string) *CallToolResult
// text="" means no accompanying caption text

// Approach B — Manual struct construction (same result, more explicit)
return &mcp.CallToolResult{
    Content: []mcp.Content{
        mcp.ImageContent{
            Type:     "image",
            Data:     imgBase64,
            MIMEType: "image/png",
        },
    },
}, nil
```

The `examples/everything/main.go` in the mcp-go repo uses Approach B (manual struct) for image content. `NewToolResultImage` is a helper that produces the same result. [VERIFIED: github.com/mark3labs/mcp-go/blob/main/examples/everything/main.go]

### Anti-Patterns to Avoid

- **Do not use `return nil, err` from a tool handler for Gemini errors.** Returning a Go error causes a JSON-RPC protocol-level error that the agent cannot read or recover from. Use `mcp.NewToolResultError(...)` + `return result, nil`. [CITED: mcp-tool-best-practices.md §7]
- **Do not call `genai.NewClient` once at startup and share it.** The client holds context-bound connections; create per-request or store carefully. In this phase, creating per-request is safe given the simple architecture.
- **Do not use `github.com/google/generative-ai-go/genai` (the old SDK).** It reached EOL August 31, 2025. The import path is different — `google.golang.org/genai` is the correct path. [CITED: pkg.go.dev migration notice]
- **Do not use `ResponseMIMEType` for image generation.** `ResponseMIMEType` controls the text output format (e.g., `application/json`). For images, use `ResponseModalities` set to include `"IMAGE"`. [ASSUMED based on SDK docs]
- **Do not use `genai.Text(prompt)` for `Contents []*genai.Content` when the function accepts `any`.** `genai.Text(...)` is a convenience shorthand that the SDK accepts in place of a full `[]*genai.Content` slice for the single-message text case. [CITED: pkg.go.dev/google.golang.org/genai]

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| MCP protocol (JSON-RPC, initialize, tool dispatch) | Custom JSON-RPC handler | `mcp-go` `server.MCPServer` | Protocol has edge cases: protocol versions, capability negotiation, streaming |
| Base64 encoding | Manual encoding | `encoding/base64` (stdlib) + `mcp.NewToolResultImage` | Already handles encoding contract for MCP `ImageContent.data` field |
| `.env` file parsing | Manual line reader | `godotenv.Load()` | Handles quoting, comments, multi-line values, shell escaping |
| Panic recovery at transport level | Custom HTTP middleware | `server.WithRecovery()` | Library middleware runs before handler and is spec-aware |
| Streamable HTTP session management | Custom session tracking | `mcp-go` built-in | Session IDs, SSE keepalives, stateful vs. stateless modes are non-trivial |

---

## Common Pitfalls

### Pitfall 1: Old SDK Import Path

**What goes wrong:** Code compiles but uses deprecated `github.com/google/generative-ai-go/genai` instead of `google.golang.org/genai`.  
**Why it happens:** Training data and pre-2025 blog posts reference the old package name. Go tools happily resolve both.  
**How to avoid:** `go.mod` should show `google.golang.org/genai`. Any import of `github.com/google/generative-ai-go` is a red flag.  
**Warning signs:** Old SDK uses type-assertion pattern (`part.(genai.Text)`, `part.(genai.ImageData)`) instead of field access (`part.Text`, `part.InlineData`).

### Pitfall 2: Gemini Returns Text Parts Alongside Image Parts

**What goes wrong:** Loop finds a text part first, returns early, never returns the image.  
**Why it happens:** Gemini models often emit a brief caption alongside the image.  
**How to avoid:** Always iterate ALL parts; only return when `part.InlineData != nil && len(part.InlineData.Data) > 0`. The code pattern above handles this correctly.  
**Warning signs:** Tests always return text errors even when the API call succeeds.

### Pitfall 3: WriteTimeout Too Short for Image Generation

**What goes wrong:** HTTP server closes the connection mid-response during image generation; client gets an incomplete or empty response.  
**Why it happens:** Default `http.Server.WriteTimeout` of 10–30s may be shorter than Gemini image generation time (can be 10–60s+ depending on model load).  
**How to avoid:** Set `WriteTimeout: 120 * time.Second` in the `http.Server`. The `mcp-go` `Start()` method uses its own defaults — prefer constructing `http.Server` manually.  
**Warning signs:** Intermittent timeouts under load, especially for longer prompts.

### Pitfall 4: godotenv.Load() Panic on Relative Path in Tests

**What goes wrong:** `godotenv.Load()` looks for `.env` relative to the working directory at test time (the package directory), not the repo root.  
**Why it happens:** Go test runs from the package directory; `.env` lives at repo root.  
**How to avoid:** `_ = godotenv.Load()` discards the "file not found" error — tests work fine without `.env` since the env var can be set in the test environment.  
**Warning signs:** `godotenv.Load()` error printed to stderr during tests (harmless but noisy if not discarded).

### Pitfall 5: `mcp.NewToolResultError` vs. Returning a Go Error

**What goes wrong:** Returning `nil, err` from a tool handler creates a JSON-RPC protocol error that the MCP client sees as a transport failure, not a tool failure. The agent cannot see the error text or recover.  
**Why it happens:** Go error convention is `return nil, err`. Tool handlers look like normal Go functions.  
**How to avoid:** Tool errors ALWAYS use `mcp.NewToolResultError(msg)` + `return result, nil`. The Go error return is reserved for protocol-level failures only.  
**Warning signs:** Agent receives opaque errors; cannot retry or rephrase prompt.

### Pitfall 6: `WithRecovery()` Does Not Cover All Panics

**What goes wrong:** A panic in a goroutine spawned inside the handler is NOT caught by `WithRecovery()` or the handler-level `defer/recover`.  
**Why it happens:** `defer/recover` only catches panics in the current goroutine.  
**How to avoid:** Phase 1 has no goroutines inside handlers. If goroutines are added later, each must have its own `defer/recover`.  
**Warning signs:** Server crash with `panic` in stderr from an unexpected goroutine.

---

## Code Examples

All patterns are compiled into the Architecture Patterns section above. Key verified patterns summary:

### Gemini Client Creation
```go
// Source: pkg.go.dev/google.golang.org/genai (VERIFIED: proxy.golang.org v1.58.0)
client, err := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey:  apiKey,
    Backend: genai.BackendGeminiAPI,
})
defer client.Close()
```

### GenerateContent for Image
```go
// Source: ai.google.dev/gemini-api/docs/image-generation (CITED)
// genai.Text(prompt) is a convenience for single-turn text content
resp, err := client.Models.GenerateContent(
    ctx,
    "gemini-3.1-flash-image-preview",
    genai.Text(prompt),
    &genai.GenerateContentConfig{
        ResponseModalities: []string{
            string(genai.ModalityText),
            string(genai.ModalityImage),
        },
    },
)
```

### Extract Image Bytes
```go
// Source: google cloud docs image generation example (CITED), verified field names from pkg.go.dev
for _, part := range resp.Candidates[0].Content.Parts {
    if part.InlineData != nil && len(part.InlineData.Data) > 0 {
        // part.InlineData is *genai.Blob{Data []byte, MIMEType string}
        imageBytes := part.InlineData.Data    // raw bytes — NOT base64
        mimeType   := part.InlineData.MIMEType // e.g. "image/png"
    }
}
```

### Return ImageContent from Tool Handler
```go
// Source: github.com/mark3labs/mcp-go/blob/main/examples/everything/main.go (VERIFIED)
imgBase64 := base64.StdEncoding.EncodeToString(imageBytes)
return mcp.NewToolResultImage("", imgBase64, mimeType), nil
// OR manually:
return &mcp.CallToolResult{
    Content: []mcp.Content{
        mcp.ImageContent{Type: "image", Data: imgBase64, MIMEType: mimeType},
    },
}, nil
```

### Tool Annotation Pattern
```go
// Source: mcp-tool-best-practices.md + pkg.go.dev/github.com/mark3labs/mcp-go/mcp (CITED)
mcp.NewTool("generate_visuals",
    mcp.WithDescription(generateVisualsDescription),
    mcp.WithString("image_prompt", mcp.Required(), mcp.Description("...")),
    mcp.WithOpenWorldHint(true),
    mcp.WithReadOnlyHint(false),
    mcp.WithDestructiveHint(false),
    mcp.WithIdempotentHint(false),
)
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `github.com/google/generative-ai-go` (type assertions) | `google.golang.org/genai` (field access) | EOL August 2025 | New SDK: `part.InlineData.Data` not `part.(genai.ImageData).Data` |
| SSE transport in mcp-go | Streamable HTTP transport | MCP spec 2025-03-26 | HTTP POST-based; no long-lived SSE connection required for basic calls |
| Manual JSON-RPC wiring | `mcp-go` library | — | Library handles all spec compliance |

**Deprecated/outdated:**
- `github.com/google/generative-ai-go/genai`: EOL August 31, 2025. Do not use.
- SSE-only transport: Still supported in mcp-go but Streamable HTTP is the current standard for remote MCP servers.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `genai.ModalityText` and `genai.ModalityImage` are exported constants of type `Modality` (string-based) | Code Examples | If they are not exported or have different types, use raw strings `"TEXT"` and `"IMAGE"` instead — both forms appear in docs |
| A2 | `genai.NewClient` created per-request (not singleton) is safe for concurrent tool calls | Architecture Patterns | If client is not goroutine-safe or has per-client rate limiting, a singleton with sync.Mutex may be needed |
| A3 | `ResponseMIMEType` is not needed for image generation (only `ResponseModalities`) | Anti-Patterns | If omitting `ResponseMIMEType` causes text-only responses, set `ResponseMIMEType: ""` explicitly |
| A4 | `mcp.WithOpenWorldHint`, `mcp.WithReadOnlyHint`, etc. are the correct mcp-go annotation function names | Code Examples | If the annotation API changed, use the `mcp.ToolAnnotations` struct directly |
| A5 | `mcp.NewToolResultImage(text, data, mime)` with `text=""` produces valid output (no required text field) | Pattern 4 | If text cannot be empty, pass a placeholder like `"Generated image"` |

**Risk mitigation:** All A1–A5 assumptions are low-risk: the alternative approaches (raw strings, struct literals) are always available and equally valid.

---

## Open Questions

1. **`genai.NewClient` per-request vs. singleton**
   - What we know: Client creation involves HTTP connection setup; per-request creation adds overhead
   - What's unclear: Whether `genai.Client` is goroutine-safe for concurrent `GenerateContent` calls
   - Recommendation: Start with per-request (simpler, Phase 1 low traffic); refactor to singleton in Phase 2 if needed

2. **Gemini image MIME type output**
   - What we know: Docs show PNG examples; `part.InlineData.MIMEType` is set by the model
   - What's unclear: Whether `gemini-3.1-flash-image-preview` always returns PNG, or sometimes JPEG/WebP
   - Recommendation: Always use `part.InlineData.MIMEType` as the source of truth; fall back to `"image/png"` only if empty

3. **`mcp.WithTitleAnnotation` availability**
   - What we know: The `title` field is defined in the MCP spec annotations as a human-readable display name
   - What's unclear: Whether `mcp-go v0.54.1` exports `WithTitleAnnotation` or uses a different name
   - Recommendation: Check `pkg.go.dev/github.com/mark3labs/mcp-go/mcp` at implementation time; fall back to `mcp.WithAnnotations(mcp.ToolAnnotations{Title: "..."})` if needed

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go runtime | Building and running the server | Yes | go1.26.3 | — |
| Internet access (Gemini API) | TOOL-03: image generation | Assumed (network-dependent) | — | None — Phase 1 requires live Gemini API |
| `GEMINI_API_KEY` | CFG-01/CFG-05 | Not pre-set (must be provided) | — | Load from `.env` file |

**Missing dependencies with no fallback:**
- `GEMINI_API_KEY` must be set by the developer before running; server will fail fast per CFG-05

**Missing dependencies with fallback:**
- None

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` package (no external test framework needed for Phase 1) |
| Config file | None — Go tests use `go test ./...` |
| Quick run command | `go test ./internal/...` |
| Full suite command | `go test ./... -v` |

### Phase Requirements — Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SRV-01 | Server starts and listens on `:8080` | smoke | Manual: `curl http://localhost:8080/mcp` | No — Wave 0 |
| SRV-02 | `/mcp` endpoint responds to MCP initialize | integration | `go test ./... -run TestMCPInitialize` | No — Wave 0 |
| SRV-04 | `initialize` response has correct name/version | unit | `go test ./... -run TestServerIdentity` | No — Wave 0 |
| TOOL-01 | `generate_visuals` tool is registered | unit | `go test ./internal/tools/... -run TestToolRegistered` | No — Wave 0 |
| TOOL-02 | `image_prompt` param required, returns error if missing | unit | `go test ./internal/tools/... -run TestMissingPrompt` | No — Wave 0 |
| TOOL-04 | Tool returns `ImageContent` with base64 data | integration | `go test ./internal/tools/... -run TestImageContent` (requires API key) | No — Wave 0 |
| TOOL-05 | Structured error on Gemini failure | unit | `go test ./internal/tools/... -run TestGeminiError` (mock client) | No — Wave 0 |
| CFG-01–03 | Config loads GEMINI_API_KEY from env, .env, env wins | unit | `go test ./internal/config/... -run TestConfigLoad` | No — Wave 0 |
| CFG-05 | Fail fast if GEMINI_API_KEY missing | unit | `go test ./internal/config/... -run TestConfigMissingKey` | No — Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/... -timeout 30s` (unit tests only; skip integration tests requiring API key)
- **Per wave merge:** `go test ./... -timeout 60s`
- **Phase gate:** Full suite green (with `GEMINI_API_KEY` set) before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/config/config_test.go` — covers CFG-01, CFG-02, CFG-03, CFG-05
- [ ] `internal/tools/generate_visuals_test.go` — covers TOOL-01, TOOL-02, TOOL-05 (unit); TOOL-04 (integration with real API)
- [ ] No test framework setup needed beyond Go stdlib

*(No external test framework to install — Go stdlib `testing` is sufficient for Phase 1.)*

---

## Security Domain

> `security_enforcement` key absent from config.json — treating as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No — Phase 1 is unauthenticated by design (localhost only) | Phase 2 adds Authelia JWT |
| V3 Session Management | Partial — mcp-go manages MCP session IDs | mcp-go built-in session management |
| V4 Access Control | No — Phase 1 has no access control | Phase 2 |
| V5 Input Validation | Yes — `image_prompt` from untrusted input | `req.RequireString()` validates type; prompt passed to external API |
| V6 Cryptography | No — no crypto in Phase 1 | — |

### Known Threat Patterns for This Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| API key exposure in logs | Information Disclosure | Never log `cfg.GeminiAPIKey`; log only key presence/absence |
| Prompt injection via `image_prompt` | Tampering | Gemini API applies its own content policy; no additional server-side sanitization needed for Phase 1 |
| Panic-induced DoS | Denial of Service | `server.WithRecovery()` + handler `defer/recover` (D-08) |
| Unlimited request size | DoS | `http.Server.ReadTimeout` limits slow-body attacks; `MaxBytesReader` can be added if needed |

---

## Sources

### Primary (HIGH confidence)

- `proxy.golang.org` — verified versions of all three packages via direct API calls
- `pkg.go.dev/github.com/mark3labs/mcp-go/mcp` — `NewImageContent`, `NewToolResultImage`, `ImageContent` struct, `NewToolResultError` signatures
- `pkg.go.dev/github.com/mark3labs/mcp-go/server` — `NewMCPServer`, `NewStreamableHTTPServer`, `WithRecovery()`, all server options
- `pkg.go.dev/google.golang.org/genai` — `Part`, `Blob`, `GenerateContentConfig`, `NewClient` signatures
- `pkg.go.dev/github.com/joho/godotenv` — `Load()` signature and behavior
- `github.com/mark3labs/mcp-go/blob/main/examples/everything/main.go` — `ImageContent` struct usage pattern, `handleGetTinyImageTool`

### Secondary (MEDIUM confidence)

- `mcp-go.dev/transports/http/` — `NewStreamableHTTPServer` options, `WithEndpointPath`, `Start` pattern
- `ai.google.dev/gemini-api/docs/image-generation` — model name `gemini-3.1-flash-image-preview`, `ResponseModalities`, `InlineData` extraction
- `mcp-go.dev/servers/middleware/` — `WithRecovery()` behavior description

### Tertiary (LOW confidence)

- WebSearch results on `godotenv` silent skip pattern — corroborates `_ = godotenv.Load()` but is community-sourced

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all three packages verified on Go module proxy with source repo confirmed
- Architecture: HIGH — patterns derived from official mcp-go examples and reference doc (mcp-go-authelia.md)
- Pitfalls: HIGH (items 1–5) / MEDIUM (item 6) — items 1–5 from official docs; item 6 is general Go goroutine knowledge

**Research date:** 2026-05-27  
**Valid until:** 2026-06-27 (mcp-go releases frequently; re-check before Phase 2)
