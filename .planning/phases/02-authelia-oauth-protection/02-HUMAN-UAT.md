---
status: partial
phase: 02-authelia-oauth-protection
source: [02-VERIFICATION.md]
started: 2026-05-31T00:00:00Z
updated: 2026-05-31T00:00:00Z
---

## Current Test

[awaiting human testing — requires live Authelia instance]

## Tests

### 1. Expired JWT returns 401

expected: `GET /mcp` with a real Authelia-issued JWT where `exp` is in the past returns `401`
result: [pending]

### 2. Valid JWT from real Authelia returns 200

expected: `GET /mcp` with a valid Authelia JWT (RS256, correct iss/aud) returns 200 and proxies to MCP handler
result: [pending]

Pre-requisite: Authelia client config must include `access_token_signed_response_alg: RS256` (documented in PLAN.md pre-flight section)

### 3. Trailing-slash route `/mcp/` with no token returns 401

expected: `GET /mcp/` (trailing slash) without a token returns `401 WWW-Authenticate: Bearer resource_metadata=...`
result: [pending]

### 4. Trailing slash on AUTHELIA_URL stripped end-to-end

expected: If `AUTHELIA_URL` is set with a trailing slash (e.g. `https://auth.example.com/`), the server correctly strips it and `iss` claim matching succeeds against the live Authelia token
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
