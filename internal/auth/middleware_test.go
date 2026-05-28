package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/thamwangjun/generate-visuals-mcp/internal/config"
)

// makeTestJWKS generates a JWKS JSON document from an RSA public key.
// Uses a hardcoded kid = "test-key-1".
func makeTestJWKS(t *testing.T, privKey *rsa.PrivateKey) []byte {
	t.Helper()
	pubKey := &privKey.PublicKey

	// Encode n and e as base64url (no padding)
	nBytes := pubKey.N.Bytes()
	eBytes := big.NewInt(int64(pubKey.E)).Bytes()

	nEncoded := base64URLEncode(nBytes)
	eEncoded := base64URLEncode(eBytes)

	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": "test-key-1",
				"n":   nEncoded,
				"e":   eEncoded,
			},
		},
	}
	b, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("makeTestJWKS: json.Marshal: %v", err)
	}
	return b
}

// base64URLEncode encodes bytes as base64url without padding.
func base64URLEncode(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	result := make([]byte, 0, ((len(b)+2)/3)*4)
	for i := 0; i < len(b); i += 3 {
		var block [3]byte
		n := copy(block[:], b[i:])
		result = append(result, alphabet[block[0]>>2])
		result = append(result, alphabet[(block[0]&0x3)<<4|block[1]>>4])
		if n > 1 {
			result = append(result, alphabet[(block[1]&0xf)<<2|block[2]>>6])
		}
		if n > 2 {
			result = append(result, alphabet[block[2]&0x3f])
		}
	}
	return string(result)
}

// makeTestJWKSServer spins up an httptest.Server serving the JWKS JSON at /jwks.json.
// Returns the server URL and the private key (for signing test tokens).
// Calls t.Cleanup(ts.Close).
func makeTestJWKSServer(t *testing.T) (serverURL string, privKey *rsa.PrivateKey) {
	t.Helper()
	var err error
	privKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("makeTestJWKSServer: rsa.GenerateKey: %v", err)
	}

	jwksBytes := makeTestJWKS(t, privKey)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jwks.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(jwksBytes) //nolint:errcheck
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)

	return ts.URL, privKey
}

// signTestToken creates a signed RS256 JWT with the given claims and kid = "test-key-1".
func signTestToken(t *testing.T, privKey *rsa.PrivateKey, issuer, audience string, expiry time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer,
		"aud": []string{audience},
		"sub": "test-user",
		"exp": expiry.Unix(),
		"iat": time.Now().Unix(),
	})
	token.Header["kid"] = "test-key-1"

	signed, err := token.SignedString(privKey)
	if err != nil {
		t.Fatalf("signTestToken: SignedString: %v", err)
	}
	return signed
}

// makeTestConfig returns a minimal config for tests.
func makeTestConfig(autheliaURL, clientID, publicURL string) *config.Config {
	return &config.Config{
		AutheliaBaseURL:  autheliaURL,
		AutheliaClientID: clientID,
		PublicBaseURL:    publicURL,
		GeminiAPIKey:     "test-gemini-key",
		ListenAddr:       ":0",
	}
}

// TestMiddleware_NotLoaded tests that a 503 is returned when JWKS is not yet loaded.
func TestMiddleware_NotLoaded(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_NoToken tests that a 401 is returned when no Authorization header is present.
func TestMiddleware_NoToken(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_InvalidToken tests that a 401 is returned for a malformed JWT.
func TestMiddleware_InvalidToken(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_ValidToken tests that a 200 is returned for a valid JWT.
func TestMiddleware_ValidToken(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_WrongIssuer tests that a 401 is returned for a token with wrong iss.
func TestMiddleware_WrongIssuer(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_WrongAudience tests that a 401 is returned for a token with wrong aud.
func TestMiddleware_WrongAudience(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_401Header tests that the 401 response contains the correct WWW-Authenticate header.
func TestMiddleware_401Header(t *testing.T) {
	t.Skip("not implemented")
}

// TestMiddleware_503NoWWWAuthenticate tests that the 503 response does NOT contain WWW-Authenticate.
func TestMiddleware_503NoWWWAuthenticate(t *testing.T) {
	t.Skip("not implemented")
}

// Ensure config import is used.
var _ = config.Config{}
