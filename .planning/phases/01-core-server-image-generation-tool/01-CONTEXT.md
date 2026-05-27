# Phase 1: Core Server + Image Generation Tool - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning

<domain>
## Phase Boundary

A working MCP server with the `generate_visuals` tool that calls `gemini-3.1-flash-image-preview` and returns `ImageContent`. No auth — callable by any client on localhost. Phase 2 adds Authelia OAuth protection.

</domain>

<decisions>
## Implementation Decisions

### Project Layout
- **D-01:** Use `internal/` packages — `internal/tools/`, `internal/config/`, `internal/auth/`. Auth package scaffolded but empty in Phase 1 to avoid refactoring in Phase 2.
- **D-02:** Entry point in `main.go` at project root. HTTP wiring lives in `main.go`; tool definitions in `internal/tools/`; config loading in `internal/config/`.

### Tool Description
- **D-03:** The `generate_visuals` tool description is stored as a named `const generateVisualsDescription` in `internal/tools/` with a `// TODO: write final description` placeholder. The const is referenced in the tool definition — description text can be updated without touching handler logic.

### .env File Loading
- **D-04:** Use `github.com/joho/godotenv` to load the `.env` file before reading env vars. Silently skip if `.env` is absent (expected in container/CI environments). Fail fast only when required env vars are missing after all sources are checked.
- **D-05:** Env var takes precedence over `.env` file value — `godotenv.Load()` called before reading, so an already-set env var wins.

### Error Response Content
- **D-06:** Gemini API errors follow the 3-part template: what failed, likely cause (quota, content policy, network), and a recovery suggestion ("retry once; if persistent, simplify the prompt"). Return via `mcp.NewToolResultError()` with `isError: true`.
- **D-07:** If Gemini returns success status but no image bytes, treat as an error: `"Gemini returned a response but no image data was generated. This may be due to content policy restrictions. Try rephrasing the prompt."` Return via `mcp.NewToolResultError()`.
- **D-08:** Every tool handler wraps its body in a `defer/recover` to catch panics and return a structured error instead of crashing the handler.

### Tool Schema & Annotations
- **D-09:** `image_prompt` parameter: `Required()`, no `maxLength` constraint (Gemini handles prompt length server-side). Parameter name is `image_prompt` per REQUIREMENTS.md TOOL-02.
- **D-10:** Tool annotations: `openWorldHint: true` (calls Gemini externally), `readOnlyHint: false`, `destructiveHint: false`, `idempotentHint: false` (calling twice = two API calls).

### Server Identity
- **D-11:** Server name: `"generate-visuals-mcp"`, version: `"1.0.0"` in the MCP `initialize` response (SRV-04).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### MCP Protocol & Tool Design
- `.planning/references/mcp-tool-best-practices.md` — Tool description quality checklist, schema constraints, annotations decision table, error response 3-part template, panic handling pattern. Read before writing the tool definition.

### Authentication Wiring (Phase 2 reference — scaffolding context)
- `.planning/references/mcp-go-authelia.md` — Full `mark3labs/mcp-go` + Authelia OAuth wiring. Defines the package layout (`internal/tools/`, `internal/config/`, `internal/auth/`) and middleware pattern. Read to understand why Phase 1 scaffolds `internal/auth/` as an empty package.

### Requirements
- `.planning/REQUIREMENTS.md` — 19 requirements across Server, Tool, Auth, Config. Phase 1 covers SRV-01, SRV-02, SRV-04, TOOL-01–05, CFG-01–05.
- `.planning/PROJECT.md` — Fixed constraints: Go, `mark3labs/mcp-go`, `gemini-3.1-flash-image-preview`, no abstraction layer.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None yet — codebase is empty (`go.mod` only with module `github.com/thamwangjun/generate-visuals-mcp`, Go 1.26).

### Established Patterns
- None yet — this is the first phase. Patterns established here become the baseline.

### Integration Points
- Phase 2 (`internal/auth/`) wraps the HTTP handler returned by `server.NewStreamableHTTPServer()`. Build Phase 1's `main.go` so the handler is a named variable that Phase 2's middleware can wrap without touching tool code.

</code_context>

<specifics>
## Specific Ideas

- Tool description as a named `const` with a `// TODO` placeholder — makes iterating on prompt engineering easy without touching handler logic.
- Project layout mirrors the reference doc (`mcp-go-authelia.md` §4) exactly so Phase 2 is a straight diff.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 1-Core Server + Image Generation Tool*
*Context gathered: 2026-05-27*
