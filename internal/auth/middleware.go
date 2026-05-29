package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	keyfunc "github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/thamwangjun/generate-visuals-mcp/internal/config"
)

// contextKey is an unexported type for request context keys in this package.
type contextKey struct{}

// claimsKey is the context key used to store the validated JWT token.
var claimsKey = contextKey{}

// Validator holds the JWKS keyfunc and JWT validation parameters.
type Validator struct {
	kf       keyfunc.Keyfunc
	loaded   atomic.Bool
	issuer   string
	audience string
}

// NewValidatorAsync initialises a Validator that starts fetching JWKS in the background.
// It returns immediately; use Middleware which will return 503 until JWKS is available.
// Returns an error only if the keyfunc wiring itself fails (not if the JWKS URL is unreachable).
func NewValidatorAsync(ctx context.Context, cfg *config.Config) (*Validator, error) {
	jwksURL := cfg.AutheliaBaseURL + "/jwks.json"

	v := &Validator{
		issuer:   cfg.AutheliaBaseURL,
		audience: cfg.AutheliaClientID,
	}

	override := keyfunc.Override{
		RefreshInterval: 5 * time.Minute,
		RefreshErrorHandlerFunc: func(u string) func(ctx context.Context, err error) {
			return func(_ context.Context, err error) {
				log.Printf("auth: JWKS refresh error (url=%s): %v", u, err)
			}
		},
	}

	kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, override)
	if err != nil {
		return nil, fmt.Errorf("auth: keyfunc wiring error: %w", err)
	}
	v.kf = kf

	go v.waitForLoad(ctx)

	return v, nil
}

// waitForLoad polls the keyfunc's key storage with exponential backoff until at least
// one key is present, then sets loaded=true so Middleware stops returning 503.
// This avoids the TOCTOU race of probing the URL independently from keyfunc's own fetch.
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

// Middleware returns an HTTP middleware that validates JWT Bearer tokens.
// prmURL is the Protected Resource Metadata URL included in WWW-Authenticate headers on 401.
// Returns 503 (no WWW-Authenticate) until JWKS is loaded; 401 for missing/invalid tokens.
func (v *Validator) Middleware(prmURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !v.loaded.Load() {
				serviceUnavailable(w)
				return
			}

			tokenString, err := extractBearer(r)
			if err != nil {
				unauthorized(w, prmURL, err.Error())
				return
			}

			token, err := jwt.Parse(
				tokenString,
				v.kf.Keyfunc,
				jwt.WithIssuer(v.issuer),
				jwt.WithAudience(v.audience),
				jwt.WithExpirationRequired(),
				jwt.WithLeeway(10*time.Second),
				jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
			)
			if err != nil || !token.Valid {
				unauthorized(w, prmURL, "invalid or expired token")
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, token)))
		})
	}
}

// extractBearer extracts the Bearer token from the Authorization header.
func extractBearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing Authorization header")
	}
	if len(h) < 8 || h[:7] != "Bearer " {
		return "", fmt.Errorf("Authorization header is not a Bearer token")
	}
	return h[7:], nil
}

// unauthorized writes a 401 response with WWW-Authenticate and a JSON error body.
func unauthorized(w http.ResponseWriter, prmURL, detail string) {
	detailJSON, _ := json.Marshal(detail)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+prmURL+`"`)
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"error":"invalid_token","error_description":%s}`, detailJSON)
}

// serviceUnavailable writes a 503 response. It deliberately omits WWW-Authenticate
// to avoid triggering a spurious OAuth flow in the client (D-05 / Gotcha 5).
func serviceUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "10")
	w.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprint(w, `{"error":"auth_not_ready","error_description":"JWKS not yet loaded"}`)
}
