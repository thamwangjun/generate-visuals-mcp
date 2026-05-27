# generate-visuals-mcp

## What This Is

A Go MCP server that exposes a single `generate_visuals` tool for AI image generation. It uses the `google.golang.org/genai` SDK to call `gemini-3.1-flash-image-preview` and returns the result as `ImageContent` in the MCP `CallToolResult`. The server runs over HTTP/SSE using `mark3labs/mcp-go` and is protected by Authelia OAuth JWT bearer token validation.

## Core Value

Any MCP-compatible client can generate images from a text prompt with a single tool call, with no additional infrastructure beyond a Gemini API key and an Authelia instance.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Single `generate_visuals` tool: accepts `image_prompt` string, returns `ImageContent`
- [ ] Gemini integration via `google.golang.org/genai`, model `gemini-3.1-flash-image-preview`
- [ ] HTTP/SSE transport (Streamable HTTP) via `mark3labs/mcp-go`
- [ ] Authelia OAuth protection: JWT bearer token validation with JWKS caching
- [ ] Config via env var (`GEMINI_API_KEY`) or `.env` file; env var takes precedence

### Out of Scope

- Multiple image generation backends — single Gemini backend only; abstraction layer adds complexity without value for v1
- stdio transport — HTTP/SSE is the target transport
- Image storage/CDN — raw `ImageContent` returned directly to the caller
- Multiple tools — one focused tool is the design

## Context

- Uses `mark3labs/mcp-go` for MCP protocol handling and HTTP transport
- Authelia is the authorization server; the MCP server is the resource server (JWT validation via JWKS, no per-request introspection round-trips)
- Reference docs in `.planning/references/` cover the Authelia OAuth wiring and MCP tool definition best practices
- Model codename "Nano Banana 2" = `gemini-3.1-flash-image-preview`

## Constraints

- **Language**: Go — established choice, not revisitable
- **MCP library**: `mark3labs/mcp-go` — matches the reference implementation in `.planning/references/`
- **Auth**: Authelia OAuth with JWT (RS256) — must set `access_token_signed_response_alg: RS256` in Authelia client config; opaque tokens will not work
- **Model**: `gemini-3.1-flash-image-preview` — fixed, no provider abstraction

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| JWT validation over token introspection | Stateless, no per-request round-trip to Authelia | — Pending |
| Single `generate_visuals` tool | Focused scope; one tool with clear purpose beats multiple overlapping tools | — Pending |
| env var takes precedence over `.env` | Standard 12-factor convention; env var allows override without changing files | — Pending |

---
*Last updated: 2026-05-27 after initial project setup*
