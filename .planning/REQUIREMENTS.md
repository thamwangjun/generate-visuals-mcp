# Requirements: generate-visuals-mcp

**Defined:** 2026-05-27
**Core Value:** Any MCP-compatible client can generate images from a text prompt with a single tool call

## v1 Requirements

### Server

- [ ] **SRV-01**: MCP server starts and listens on a configurable port (default `:8080`)
- [ ] **SRV-02**: Server exposes `/mcp` endpoint using Streamable HTTP transport
- [ ] **SRV-03**: Server serves `/.well-known/oauth-protected-resource` without authentication
- [ ] **SRV-04**: Server name and version are reported correctly in MCP `initialize` response

### Tool

- [ ] **TOOL-01**: `generate_visuals` tool is registered with a clear description following MCP tool best practices
- [ ] **TOOL-02**: Tool accepts a required `image_prompt` string parameter
- [ ] **TOOL-03**: Tool calls `gemini-3.1-flash-image-preview` via `google.golang.org/genai`
- [ ] **TOOL-04**: Tool returns `ImageContent` in the `CallToolResult` (base64-encoded image data)
- [ ] **TOOL-05**: Tool returns a structured error response (not a panic) on Gemini API failure

### Auth

- [ ] **AUTH-01**: All requests to `/mcp` require a valid JWT bearer token
- [ ] **AUTH-02**: JWKS is fetched from Authelia's `jwks_uri` on startup and refreshed on key rotation
- [ ] **AUTH-03**: Invalid or expired tokens receive a `401` response with `WWW-Authenticate: Bearer` header including `resource_metadata`
- [ ] **AUTH-04**: `/.well-known/*` paths bypass authentication (publicly accessible)
- [ ] **AUTH-05**: Issuer (`iss`) and audience (`aud`) claims are validated

### Config

- [ ] **CFG-01**: `GEMINI_API_KEY` is loaded from environment variable
- [ ] **CFG-02**: If `GEMINI_API_KEY` is not in env, it is loaded from `.env` file
- [ ] **CFG-03**: Environment variable takes precedence over `.env` file value
- [ ] **CFG-04**: Authelia base URL, client ID, listen address, and public base URL are configurable via env vars
- [ ] **CFG-05**: Server fails fast with a clear error if required config is missing

## Out of Scope

| Feature | Reason |
|---------|--------|
| Multiple image backends | Single Gemini backend for v1; abstraction premature |
| stdio transport | HTTP/SSE is the target |
| Image storage / CDN | Return raw `ImageContent` directly |
| Multiple tools | One focused tool per design decision |
| Token introspection | JWT validation is stateless and sufficient |
| Dynamic client registration | Authelia doesn't support RFC 7591 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| SRV-01 | Phase 1 | Pending |
| SRV-02 | Phase 1 | Pending |
| SRV-03 | Phase 2 | Pending |
| SRV-04 | Phase 1 | Pending |
| TOOL-01 | Phase 1 | Pending |
| TOOL-02 | Phase 1 | Pending |
| TOOL-03 | Phase 1 | Pending |
| TOOL-04 | Phase 1 | Pending |
| TOOL-05 | Phase 1 | Pending |
| AUTH-01 | Phase 2 | Pending |
| AUTH-02 | Phase 2 | Pending |
| AUTH-03 | Phase 2 | Pending |
| AUTH-04 | Phase 2 | Pending |
| AUTH-05 | Phase 2 | Pending |
| CFG-01 | Phase 1 | Pending |
| CFG-02 | Phase 1 | Pending |
| CFG-03 | Phase 1 | Pending |
| CFG-04 | Phase 1 | Pending |
| CFG-05 | Phase 1 | Pending |

**Coverage:**
- v1 requirements: 19 total
- Mapped to phases: 19
- Unmapped: 0 ✓

---
*Requirements defined: 2026-05-27*
*Last updated: 2026-05-27 after initial definition*
