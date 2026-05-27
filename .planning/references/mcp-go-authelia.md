# MCP Server with mark3labs/mcp-go and Authelia OAuth

A complete guide to building a protected remote MCP server in Go, using
`mark3labs/mcp-go` for the MCP layer and an existing Authelia instance as the
OAuth 2.1 authorization server.

---

## Table of Contents

1. [How the pieces fit together](#1-how-the-pieces-fit-together)
2. [Prerequisites](#2-prerequisites)
3. [Authelia configuration](#3-authelia-configuration)
4. [Project layout](#4-project-layout)
5. [Go module setup](#5-go-module-setup)
6. [Token validation strategy](#6-token-validation-strategy)
7. [Building the server](#7-building-the-server)
   - 7.1 [JWT validator middleware](#71-jwt-validator-middleware)
   - 7.2 [The MCP server and tools](#72-the-mcp-server-and-tools)
   - 7.3 [Wiring the HTTP stack](#73-wiring-the-http-stack)
   - 7.4 [main.go](#74-maingo)
8. [Running and verifying](#8-running-and-verifying)
9. [Connecting a client](#9-connecting-a-client)
10. [Important Authelia-specific gotchas](#10-important-authelia-specific-gotchas)
11. [Production hardening checklist](#11-production-hardening-checklist)

---

## 1. How the pieces fit together

The MCP authorization spec (2025-11-25) follows the standard OAuth 2.1
resource-server pattern. Your Go binary is the **resource server**; Authelia is
the **authorization server**. They are two separate processes — Authelia issues
tokens, your server validates them.

```
MCP Client (Claude Desktop / mcp-go client)
    │
    │  (1) GET /mcp  →  401 + WWW-Authenticate: Bearer
    │                         resource_metadata=<PRM URL>
    ▼
MCP Server (your Go binary)
    │
    │  (2) GET /.well-known/oauth-protected-resource
    │       ← { authorization_servers: ["https://authelia.example.com"] }
    ▼
MCP Client discovers Authelia
    │
    │  (3) GET https://authelia.example.com/.well-known/oauth-authorization-server
    │       ← { authorization_endpoint, token_endpoint, jwks_uri, … }
    ▼
MCP Client performs Authorization Code + PKCE flow against Authelia
    │
    │  (4) POST /api/oidc/token  →  { access_token, refresh_token }
    ▼
MCP Client sends request with Bearer token
    │
    │  (5) POST /mcp  + Authorization: Bearer <token>
    ▼
MCP Server validates JWT (or introspects opaque token) → calls tool handler
```

Key facts to keep in mind:

- `mcp-go` provides `WithProtectedResourceMetadata()` to auto-serve step (2).
  It does **not** validate tokens — that is your middleware.
- Authelia's access tokens are **opaque by default** (format `authelia_at_…`).
  For stateless JWT validation you must set `access_token_signed_response_alg:
  'RS256'` on the client. If you skip that, you must use Authelia's token
  introspection endpoint instead.
- Authelia does **not** support Dynamic Client Registration (RFC 7591) yet, so
  you pre-register the client in YAML.

---

## 2. Prerequisites

| Requirement | Notes |
|---|---|
| Go ≥ 1.22 | mcp-go targets supported Go releases |
| Authelia ≥ 4.38 | Needed for `access_token_signed_response_alg` and RFC 8414 metadata |
| HTTPS on both services | MCP spec mandates HTTPS for remote servers |
| Authelia reachable by clients | The MCP client must be able to hit Authelia's endpoints directly |

---

## 3. Authelia configuration

### 3.1 Generate a hashed client secret

Authelia stores client secrets as PBKDF2 hashes. Generate one with the
Authelia CLI:

```bash
authelia crypto hash generate pbkdf2 --variant sha512 --password 'your-client-secret'
```

Copy the output (starts with `$pbkdf2-sha512$…`). Store the **plaintext** secret
somewhere safe — you will need it in the MCP client config later.

### 3.2 Generate a signing key (if not already done)

Authelia's OIDC provider needs at least one RSA key for signing tokens:

```bash
authelia crypto certificate rsa generate --bits 4096 --directory /config/secrets/oidc/
```

Or use an existing key. Point Authelia at it in `configuration.yml`.

### 3.3 Add the OIDC provider and client

In your Authelia `configuration.yml`:

```yaml
identity_providers:
  oidc:
    hmac_secret: 'your-hmac-secret-here'   # or use a file secret

    jwks:
      - key_id: 'main'
        algorithm: 'RS256'
        use: 'sig'
        key: |
          -----BEGIN RSA PRIVATE KEY-----
          … your 4096-bit RSA private key …
          -----END RSA PRIVATE KEY-----

    # Enforce PKCE for public clients (the default).
    # Set to 'always' if you want to require it for confidential clients too.
    enforce_pkce: 'public_clients_only'

    lifespans:
      access_token: '1h'
      authorize_code: '1m'
      id_token: '1h'
      refresh_token: '90m'

    clients:
      - client_id: 'my-mcp-server'
        client_name: 'My MCP Server'
        client_secret: '$pbkdf2-sha512$310000$…'   # paste hashed secret here

        public: false

        # Required: emit access tokens as RS256 JWTs so the MCP server can
        # validate them statically without calling the introspection endpoint.
        access_token_signed_response_alg: 'RS256'

        # Authorization policy — adjust to your security requirements.
        authorization_policy: 'one_factor'

        # PKCE required for this client (good practice even for confidential clients).
        require_pkce: true
        pkce_challenge_method: 'S256'

        redirect_uris:
          # Replace with wherever your MCP client callback actually runs.
          - 'http://localhost:8085/oauth/callback'

        scopes:
          - 'openid'
          - 'profile'
          - 'offline_access'   # enables refresh tokens

        grant_types:
          - 'authorization_code'
          - 'refresh_token'

        response_types:
          - 'code'

        response_modes:
          - 'query'

        # Explicit consent is cleaner than pre-configured for MCP use cases.
        consent_mode: 'explicit'

        token_endpoint_auth_method: 'client_secret_post'
```

Restart Authelia after editing. Verify the discovery endpoint responds:

```bash
curl -s https://authelia.example.com/.well-known/oauth-authorization-server | jq .
```

You should see `authorization_endpoint`, `token_endpoint`, `jwks_uri`, and
`code_challenge_methods_supported` containing `S256`.

The JWKS endpoint Authelia exposes is at:

```
https://authelia.example.com/jwks.json
```

You can confirm the `jwks_uri` value from the discovery document above.

---

## 4. Project layout

```
my-mcp-server/
├── main.go
├── auth/
│   └── middleware.go    # JWT Bearer token validation
├── tools/
│   └── tools.go         # your MCP tool definitions
└── go.mod
```

---

## 5. Go module setup

```bash
mkdir my-mcp-server && cd my-mcp-server
go mod init github.com/yourorg/my-mcp-server

go get github.com/mark3labs/mcp-go@latest
go get github.com/MicahParks/keyfunc/v3@latest
go get github.com/golang-jwt/jwt/v5@latest
```

The three dependencies are:

| Package | Purpose |
|---|---|
| `mark3labs/mcp-go` | MCP server, transport, PRM metadata handler |
| `MicahParks/keyfunc/v3` | Fetches and caches Authelia's JWKS; produces a `jwt.Keyfunc` |
| `golang-jwt/jwt/v5` | Parses and validates the JWT access token |

---

## 6. Token validation strategy

### Option A — JWT validation (recommended)

Requires `access_token_signed_response_alg: 'RS256'` in the Authelia client
config. The MCP server fetches Authelia's public keys from `jwks_uri` once on
startup (and refreshes them on key rotation), then validates every incoming
Bearer token locally with no round-trip to Authelia.

**Pros:** fast, no dependency on Authelia per-request.  
**Cons:** revoked tokens remain valid until they expire. For 1-hour access
tokens this is usually acceptable.

### Option B — Token introspection

Use Authelia's introspection endpoint (`/api/oidc/introspection`) on every
request. Works with opaque tokens (no `access_token_signed_response_alg`
needed). Slower (one HTTP call per MCP request) but revocation is immediate.

The example in this guide uses **Option A**. The section
[Important Authelia-specific gotchas](#10-important-authelia-specific-gotchas)
covers how to switch to introspection.

---

## 7. Building the server

### 7.1 JWT validator middleware

```go
// auth/middleware.go
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const ClaimsKey contextKey = "jwt_claims"

// Validator holds a cached JWKS keyfunc and validation parameters.
type Validator struct {
	keyfunc  jwt.Keyfunc
	issuer   string
	audience string
}

// NewValidator creates a Validator that fetches JWKS from jwksURL and
// validates tokens against the given issuer and audience.
//
// jwksURL  — e.g. "https://authelia.example.com/jwks.json"
// issuer   — must match the `iss` claim; usually your Authelia base URL
// audience — the `aud` claim your Authelia client config sets (client_id or
//            a custom audience list); leave empty to skip audience validation
func NewValidator(ctx context.Context, jwksURL, issuer, audience string) (*Validator, error) {
	// keyfunc.NewDefault fetches the JWKS and refreshes it in the background.
	// It handles key rotation transparently.
	kf, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("failed to initialise JWKS keyfunc: %w", err)
	}

	return &Validator{
		keyfunc:  kf.Keyfunc,
		issuer:   issuer,
		audience: audience,
	}, nil
}

// Middleware returns an http.Handler that enforces Bearer token authentication.
// On success it stores the parsed *jwt.Token in the request context under
// ClaimsKey. On failure it responds with 401 and a WWW-Authenticate header.
func (v *Validator) Middleware(protectedResourceMetadataURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip the well-known PRM endpoint — it must be publicly accessible
			// so MCP clients can discover the authorization server.
			if strings.HasPrefix(r.URL.Path, "/.well-known/") {
				next.ServeHTTP(w, r)
				return
			}

			tokenString, err := extractBearer(r)
			if err != nil {
				unauthorized(w, protectedResourceMetadataURL, err.Error())
				return
			}

			opts := []jwt.ParserOption{
				jwt.WithIssuer(v.issuer),
				jwt.WithExpirationRequired(),
				jwt.WithLeeway(10 * time.Second),
			}
			if v.audience != "" {
				opts = append(opts, jwt.WithAudience(v.audience))
			}

			token, err := jwt.Parse(tokenString, v.keyfunc, opts...)
			if err != nil || !token.Valid {
				unauthorized(w, protectedResourceMetadataURL, "invalid or expired token")
				return
			}

			// Attach claims to context for use in tool handlers.
			ctx := context.WithValue(r.Context(), ClaimsKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearer pulls the token string from "Authorization: Bearer <token>".
func extractBearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing Authorization header")
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", fmt.Errorf("malformed Authorization header")
	}
	return parts[1], nil
}

// unauthorized writes a 401 response with a WWW-Authenticate challenge.
// The resource_metadata parameter tells the MCP client where to find the PRM
// document, which in turn points at Authelia.
func unauthorized(w http.ResponseWriter, resourceMetadataURL, detail string) {
	challenge := fmt.Sprintf(
		`Bearer realm="mcp", resource_metadata="%s", error="invalid_token", error_description="%s"`,
		resourceMetadataURL,
		detail,
	)
	w.Header().Set("WWW-Authenticate", challenge)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

// ClaimsFromContext retrieves the validated JWT token from a request context.
// Returns nil if no token is stored (should not happen after middleware runs).
func ClaimsFromContext(ctx context.Context) *jwt.Token {
	t, _ := ctx.Value(ClaimsKey).(*jwt.Token)
	return t
}
```

### 7.2 The MCP server and tools

```go
// tools/tools.go
package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Register adds all tools to the given MCPServer.
func Register(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool(
			"echo",
			mcp.WithDescription("Echoes the input back to the caller"),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("The message to echo"),
			),
		),
		echoHandler,
	)

	s.AddTool(
		mcp.NewTool(
			"hello",
			mcp.WithDescription("Returns a personalised greeting"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name to greet"),
			),
		),
		helloHandler,
	)
}

func echoHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message, err := req.RequireString("message")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(message), nil
}

func helloHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}
```

### 7.3 Wiring the HTTP stack

```go
// server_setup.go  (part of package main, shown separately for clarity)
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/yourorg/my-mcp-server/auth"
	"github.com/yourorg/my-mcp-server/tools"
)

type Config struct {
	// PublicBaseURL is the externally reachable URL of this MCP server,
	// e.g. "https://mcp.example.com". Used as the OAuth resource identifier.
	PublicBaseURL string

	// AutheliaBaseURL is your Authelia instance root, e.g. "https://authelia.example.com".
	AutheliaBaseURL string

	// ClientID is the client_id registered in Authelia. Authelia uses this
	// as the 'aud' claim in JWT access tokens by default.
	ClientID string

	// ListenAddr is the address to listen on, e.g. ":8080".
	ListenAddr string
}

func buildHandler(ctx context.Context, cfg Config) (http.Handler, error) {
	// ── 1. Build JWT validator ─────────────────────────────────────────────
	// Authelia's JWKS endpoint is always at <base>/jwks.json.
	jwksURL := cfg.AutheliaBaseURL + "/jwks.json"

	// The issuer in Authelia JWT claims equals the Authelia base URL.
	// The audience defaults to the client_id unless you set a custom
	// 'audience' list in the Authelia client config.
	validator, err := auth.NewValidator(ctx, jwksURL, cfg.AutheliaBaseURL, cfg.ClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to build JWT validator: %w", err)
	}

	// ── 2. Create the MCP server ───────────────────────────────────────────
	mcpServer := server.NewMCPServer(
		"My MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)
	tools.Register(mcpServer)

	// ── 3. Create the Streamable HTTP transport with PRM metadata ──────────
	// WithProtectedResourceMetadata auto-mounts a handler at the RFC 9728
	// well-known path so MCP clients can discover Authelia automatically.
	prmURL := cfg.PublicBaseURL + "/.well-known/oauth-protected-resource"

	httpServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithProtectedResourceMetadata(server.ProtectedResourceMetadataConfig{
			Resource: cfg.PublicBaseURL,
			AuthorizationServers: []string{
				cfg.AutheliaBaseURL, // points clients at your Authelia instance
			},
			ScopesSupported:        []string{"openid", "profile"},
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "My MCP Server",
		}),
	)

	// ── 4. Wrap with auth middleware ───────────────────────────────────────
	// The middleware skips /.well-known/* paths so PRM discovery stays public.
	authMiddleware := validator.Middleware(prmURL)

	mux := http.NewServeMux()
	mux.Handle("/mcp", authMiddleware(httpServer))
	// Also handle /mcp/ for clients that append a trailing slash.
	mux.Handle("/mcp/", authMiddleware(httpServer))
	// The well-known path is served by httpServer itself (no auth needed).
	mux.Handle("/.well-known/", httpServer)

	return mux, nil
}
```

### 7.4 main.go

```go
// main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	cfg := Config{
		PublicBaseURL:   getenv("MCP_PUBLIC_URL", "https://mcp.example.com"),
		AutheliaBaseURL: getenv("AUTHELIA_URL", "https://authelia.example.com"),
		ClientID:        getenv("AUTHELIA_CLIENT_ID", "my-mcp-server"),
		ListenAddr:      getenv("LISTEN_ADDR", ":8080"),
	}

	ctx := context.Background()

	handler, err := buildHandler(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to build handler: %v", err)
	}

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("MCP server listening on %s", cfg.ListenAddr)
	log.Printf("Public URL:   %s", cfg.PublicBaseURL)
	log.Printf("Authelia URL: %s", cfg.AutheliaBaseURL)

	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

---

## 8. Running and verifying

### Start the server

```bash
export MCP_PUBLIC_URL="https://mcp.example.com"
export AUTHELIA_URL="https://authelia.example.com"
export AUTHELIA_CLIENT_ID="my-mcp-server"
export LISTEN_ADDR=":8080"

go run .
```

### Verify the PRM endpoint (no auth required)

```bash
curl -s https://mcp.example.com/.well-known/oauth-protected-resource | jq .
```

Expected response:

```json
{
  "resource": "https://mcp.example.com",
  "authorization_servers": ["https://authelia.example.com"],
  "scopes_supported": ["openid", "profile"],
  "bearer_methods_supported": ["header"],
  "resource_name": "My MCP Server"
}
```

### Verify unauthenticated requests are rejected

```bash
curl -sv https://mcp.example.com/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

Expected: `HTTP 401` with a `WWW-Authenticate` header containing
`resource_metadata=`.

### Obtain a token from Authelia and test

```bash
# Replace with a real token obtained via the authorization code flow.
TOKEN="eyJhbGciOiJSUzI1NiIs..."

curl -s https://mcp.example.com/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}'
```

A successful response includes `result.serverInfo`.

---

## 9. Connecting a client

### Using mcp-go as an OAuth client

If you are writing a Go client with `mcp-go`, pre-fill the credentials you
registered in Authelia:

```go
package main

import (
	"context"
	"log"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	tokenStore := client.NewMemoryTokenStore()

	oauthCfg := client.OAuthConfig{
		ClientID:     "my-mcp-server",       // matches Authelia client_id
		ClientSecret: "your-client-secret",  // plaintext — NOT the hashed value
		RedirectURI:  "http://localhost:8085/oauth/callback",
		Scopes:       []string{"openid", "profile", "offline_access"},
		TokenStore:   tokenStore,
		PKCEEnabled:  true, // required; matches Authelia require_pkce: true
	}

	c, err := client.NewOAuthStreamableHttpClient(
		"https://mcp.example.com/mcp",
		oauthCfg,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	if err := c.Start(ctx); err != nil {
		// Check if we need to go through the authorization flow.
		if client.IsOAuthAuthorizationRequiredError(err) {
			runAuthFlow(ctx, err, c)
		} else {
			log.Fatal(err)
		}
	}
	defer c.Close()

	result, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "my-client", Version: "1.0.0"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Connected to: %s %s", result.ServerInfo.Name, result.ServerInfo.Version)
}
```

`runAuthFlow` follows the pattern in `examples/oauth_client/main.go` in the
mcp-go repository: open a browser to the authorization URL, listen on
localhost for the callback, exchange the code, then retry.

### Claude Desktop / Claude Code

Add the server to your MCP config. Claude's built-in OAuth support handles the
full flow automatically when it receives a 401 with the `resource_metadata`
header:

```json
{
  "mcpServers": {
    "my-mcp-server": {
      "type": "remote",
      "url": "https://mcp.example.com/mcp"
    }
  }
}
```

Claude will redirect you to Authelia for login on first use.

---

## 10. Important Authelia-specific gotchas

### Access tokens are opaque by default

Without `access_token_signed_response_alg: 'RS256'` in the Authelia client
config, access tokens look like `authelia_at_XXXX` and cannot be validated by
JWKS. If you see validation errors or your middleware rejecting all tokens, check
that this setting is present and restart Authelia.

### The `iss` claim equals the Authelia base URL exactly

Authelia sets `iss` to its configured `issuer` value, which equals the base URL
you access Authelia at. If you use `https://authelia.example.com`, the `iss`
claim is `https://authelia.example.com` — no trailing slash, no path segment.
Pass this exact string to `auth.NewValidator`.

### The `aud` claim is the `client_id` by default

Unless you set a custom `audience` list in the Authelia client YAML, Authelia
puts the `client_id` as the single `aud` value in JWT access tokens. Pass the
`client_id` as the `audience` parameter to `NewValidator` accordingly.

### Dynamic Client Registration is not supported

Authelia does not implement RFC 7591 yet. Do not leave `ClientID` empty in
`client.OAuthConfig` expecting auto-registration — it will fail. Always
pre-register in Authelia's YAML.

### Discovery chain: AS metadata vs PRM

`mcp-go`'s client discovery probes your MCP server's
`/.well-known/oauth-protected-resource` first, reads `authorization_servers`,
then fetches `/.well-known/oauth-authorization-server` from Authelia. This
chain works correctly with Authelia ≥ 4.38 which exposes both endpoints.
Make sure your PRM `authorization_servers` array contains your Authelia base URL
with no trailing slash.

### Switching to token introspection (Option B)

If you need revocation to take effect immediately, replace the JWT middleware
with an introspection call. Authelia's introspection endpoint is at
`/api/oidc/introspection` and accepts HTTP Basic auth (client_id + client_secret):

```go
// auth/introspect.go  — simplified example
func introspectToken(ctx context.Context, token, autheliaBase, clientID, clientSecret string) (bool, error) {
	endpoint := autheliaBase + "/api/oidc/introspection"

	body := url.Values{}
	body.Set("token", token)
	body.Set("token_type_hint", "access_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(body.Encode()))
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Active, nil
}
```

Note: for introspection to work with `token_endpoint_auth_method:
'client_secret_post'`, change the call above to post credentials in the body
rather than using `SetBasicAuth`.

---

## 11. Production hardening checklist

- [ ] **HTTPS on the MCP server.** The spec mandates it. Use a reverse proxy
  (Caddy, nginx, Traefik) or configure `tls.Config` in the `http.Server`.
- [ ] **HTTPS on Authelia.** Already required for Authelia's cookie security;
  also required by RFC 8414 for the AS metadata endpoint.
- [ ] **Short access token lifetime.** The default 1 hour is reasonable. Pair
  with refresh tokens (`offline_access` scope) for long-running clients.
- [ ] **Scope validation in tool handlers.** Pull the claims from context and
  check that the `scp` or `scope` claim covers what the tool requires.
- [ ] **Audience check.** `NewValidator` accepts an `audience` argument — always
  set it. An empty audience skips the check and accepts tokens intended for
  other resources.
- [ ] **JWKS refresh on validation failure.** `keyfunc.NewDefault` handles
  background refresh, but also refetches on unknown `kid`. This is the correct
  behaviour for key rotation.
- [ ] **Log validation failures.** Emit structured logs with the error reason
  (without echoing the token) so you can detect credential stuffing.
- [ ] **Rate-limit the MCP endpoint.** Tools can be expensive. Add a middleware
  that rate-limits by subject claim (`sub`).
- [ ] **Do not forward the Bearer token to downstream services.** Validate it
  for access to the MCP server, then use the server's own credentials for any
  internal API calls.
