# Roadmap: generate-visuals-mcp

**Created:** 2026-05-27
**Milestone:** v1.0 — Working MCP image generation server

---

## Phase 1: Core Server + Image Generation Tool

**Goal:** A working MCP server with the `generate_visuals` tool that calls Gemini and returns `ImageContent`. No auth — callable by any client on localhost.

**Delivers:**
- Go module with `mark3labs/mcp-go` and `google.golang.org/genai`
- `generate_visuals` tool: prompt in, `ImageContent` out
- HTTP/SSE Streamable HTTP transport on configurable port
- Config loading: env var + `.env` file, env var takes precedence
- Structured error responses following MCP tool best practices

**Requirements covered:** SRV-01, SRV-02, SRV-04, TOOL-01–05, CFG-01–05

**Done when:** `curl` to `/mcp` with a valid initialize + tool call returns a base64 image in `ImageContent`

---

## Phase 2: Authelia OAuth Protection

**Goal:** Secure the server with JWT bearer token validation backed by Authelia. Only authenticated clients can call tools.

**Delivers:**
- JWT validator middleware using `MicahParks/keyfunc/v3` and `golang-jwt/jwt/v5`
- JWKS fetched from Authelia on startup, refreshed on key rotation
- `/.well-known/oauth-protected-resource` endpoint (public, no auth)
- `401 + WWW-Authenticate` response for missing/invalid tokens
- Issuer and audience claim validation
- Config: `AUTHELIA_URL`, `AUTHELIA_CLIENT_ID`, `MCP_PUBLIC_URL` env vars

**Requirements covered:** SRV-03, AUTH-01–05

**Done when:** unauthenticated requests to `/mcp` return `401` with correct `WWW-Authenticate` header; requests with a valid Authelia JWT succeed

---

## Backlog (v2+)

- Rate limiting per JWT `sub` claim
- Structured request logging (prompt, latency, model response metadata)
- Docker image / deployment config
- Multiple image sizes or style parameters on the tool

---
*Last updated: 2026-05-27 after initial roadmap creation*
