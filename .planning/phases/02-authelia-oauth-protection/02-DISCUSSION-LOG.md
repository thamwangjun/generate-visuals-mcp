# Phase 2: Authelia OAuth Protection - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-28
**Phase:** 2-Authelia OAuth Protection
**Areas discussed:** Config fail-fast, JWKS startup behavior, HTTP routing structure

---

## Config Fail-fast

| Option | Description | Selected |
|--------|-------------|----------|
| Fail fast on both | Extend log.Fatal pattern to AUTHELIA_URL, AUTHELIA_CLIENT_ID, MCP_PUBLIC_URL | ✓ |
| Fail fast on AUTHELIA_URL only | CLIENT_ID optional — audience validation skipped if empty | |
| No fail-fast | Let runtime fail when keyfunc.NewDefault is called | |

**User's choice:** Fail fast on all three auth vars.

### AUTHELIA_URL required?

| Option | Description | Selected |
|--------|-------------|----------|
| Required — fail fast | Embedded in WWW-Authenticate and PRM; wrong value misleads clients | ✓ |
| Default to localhost | Allow dev setups without the var | |

**User's choice:** Required — fail fast.

---

## JWKS Startup Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Fatal — refuse to start | keyfunc.NewDefault failure → log.Fatalf; consistent with fail-fast pattern | |
| Warn and continue | Log warning, start listening, all /mcp requests get 401 until JWKS loads | |
| Warn + exponential backoff (Other) | Warn, start, retry with exponential backoff indefinitely | ✓ |

**User's choice:** Warn and continue with exponential backoff retry.

### While JWKS is loading:

| Option | Description | Selected |
|--------|-------------|----------|
| Return 503 Service Unavailable | Distinct from 401; clients that retry on 503 recover automatically | ✓ |
| Return 401 Unauthorized | Simpler; same path as invalid tokens | |
| Queue requests | Hold and flush once JWKS loads | |

**User's choice:** 503 Service Unavailable.

### Backoff ceiling:

| Option | Description | Selected |
|--------|-------------|----------|
| Retry indefinitely | Cap at ~30s between attempts; recovers whenever Authelia comes back | ✓ |
| Give up after N attempts | Fatal after ~30s total | |

**User's choice:** Retry indefinitely.

**Notes:** Rationale is container environments where Authelia may start after the MCP server.

---

## HTTP Routing Structure

User asked for explanation of mux vs wrap tradeoffs before selecting.

**Mux advantages noted:** explicit at a glance, no hidden bypasses inside middleware, easier to extend with future public endpoints, standard Go pattern.

**Mux disadvantage noted:** `httpServer` mounted three times in the mux (but functionally correct).

| Option | Description | Selected |
|--------|-------------|----------|
| Mux-based routing | ServeMux routes /mcp through auth, /.well-known/ directly to httpServer | ✓ |
| Wrap approach | Wrap entire handler; bypass logic inside middleware | |

**User's choice:** Mux-based routing.

### PRM endpoint:

| Option | Description | Selected |
|--------|-------------|----------|
| Built-in WithProtectedResourceMetadata() | mcp-go auto-serves /.well-known/oauth-protected-resource | ✓ |
| Hand-rolled handler | Custom http.HandlerFunc for PRM JSON | |

**User's choice:** Built-in WithProtectedResourceMetadata().

---

## Claude's Discretion

- Exact exponential backoff parameters (initial delay, jitter, cap)
- Structured logging format for retry attempts and validation failures
- Whether to emit a log line when JWKS first loads successfully (readiness signal)

## Deferred Ideas

- Rate limiting per JWT `sub` claim — v2+ backlog
- Scope validation in tool handlers — v2+ production hardening
- Structured request logging (prompt, latency) — v2+ backlog
- Docker/deployment config — v2+ backlog
