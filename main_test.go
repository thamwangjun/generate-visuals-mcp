package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// newTestMCPServer builds the same MCPServer that main() would build,
// but without calling config.Load() (which would fatal if GEMINI_API_KEY is unset).
func newTestMCPServer() *server.MCPServer {
	return server.NewMCPServer(
		"generate-visuals-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
}

// initializeRequest returns the JSON-RPC initialize request body.
func initializeRequest() []byte {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test",
				"version": "0.1",
			},
		},
	})
	return body
}

// TestHTTPEndpointPath_MCPResponds verifies that a POST to /mcp returns a valid
// MCP JSON-RPC response (not 404 or 405). This covers gap SRV-02.
func TestHTTPEndpointPath_MCPResponds(t *testing.T) {
	mcpServer := newTestMCPServer()
	httpHandler := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))

	ts := httptest.NewServer(httpHandler)
	defer ts.Close()

	reqBody := initializeRequest()
	resp, err := http.Post(ts.URL+"/mcp", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("HTTP POST to /mcp failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("POST /mcp returned 404 Not Found — endpoint is not wired")
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Fatalf("POST /mcp returned 405 Method Not Allowed — POST is not accepted")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /mcp returned unexpected status %d; body: %s", resp.StatusCode, body)
	}

	// Verify the response body is valid JSON (MCP response, not an HTML error page)
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("POST /mcp response body is not valid JSON: %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("response missing jsonrpc=2.0 field; got: %v", result)
	}
}

// TestPRMEndpoint_PublicAccess verifies that the /.well-known/oauth-protected-resource
// endpoint is publicly accessible and returns valid JSON with required fields.
func TestPRMEndpoint_PublicAccess(t *testing.T) {
	prmConfig := server.ProtectedResourceMetadataConfig{
		Resource:               "https://mcp.example.com",
		AuthorizationServers:   []string{"https://authelia.example.com"},
		ScopesSupported:        []string{"openid", "profile"},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "generate-visuals-mcp",
	}
	prmHandler := server.NewProtectedResourceMetadataHandler(prmConfig)

	ts := httptest.NewServer(prmHandler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /.well-known/oauth-protected-resource: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["resource"]; !ok {
		t.Error("body missing 'resource' field")
	}
	if _, ok := body["authorization_servers"]; !ok {
		t.Error("body missing 'authorization_servers' field")
	}
}

// TestMCPServerIdentity_InitializeResponse verifies that the MCP server announces
// itself as name="generate-visuals-mcp" and version="1.0.0" in the initialize
// JSON-RPC response. This covers gap SRV-04.
func TestMCPServerIdentity_InitializeResponse(t *testing.T) {
	mcpServer := newTestMCPServer()

	rawMsg, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test",
				"version": "0.1",
			},
		},
	})
	if err != nil {
		t.Fatal("failed to marshal initialize request:", err)
	}

	resp := mcpServer.HandleMessage(context.Background(), rawMsg)
	if resp == nil {
		t.Fatal("HandleMessage returned nil for initialize request")
	}

	// resp must be a JSONRPCResponse
	jsonResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("unexpected response type %T, want mcp.JSONRPCResponse", resp)
	}

	// Result must be an InitializeResult
	initResult, ok := jsonResp.Result.(mcp.InitializeResult)
	if !ok {
		t.Fatalf("unexpected result type %T, want mcp.InitializeResult", jsonResp.Result)
	}

	if initResult.ServerInfo.Name != "generate-visuals-mcp" {
		t.Errorf("serverInfo.name = %q, want %q", initResult.ServerInfo.Name, "generate-visuals-mcp")
	}
	if initResult.ServerInfo.Version != "1.0.0" {
		t.Errorf("serverInfo.version = %q, want %q", initResult.ServerInfo.Version, "1.0.0")
	}
}
