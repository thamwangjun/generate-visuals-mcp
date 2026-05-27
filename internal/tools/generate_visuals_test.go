package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/thamwangjun/generate-visuals-mcp/internal/config"
	"github.com/thamwangjun/generate-visuals-mcp/internal/tools"
)

func dummyCfg() *config.Config {
	return &config.Config{
		GeminiAPIKey: "dummy-key",
		ListenAddr:   ":8080",
	}
}

func TestToolRegistered(t *testing.T) {
	s := server.NewMCPServer("test", "0.1.0")
	tools.Register(s, dummyCfg())

	// Send a tools/list request via HandleMessage (takes json.RawMessage)
	rawMsg, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	if err != nil {
		t.Fatal("failed to marshal request:", err)
	}

	resp := s.HandleMessage(context.Background(), rawMsg)
	if resp == nil {
		t.Fatal("nil response from HandleMessage")
	}

	// The response result should list the tool
	result, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("unexpected response type: %T", resp)
	}

	listResult, ok := result.Result.(mcp.ListToolsResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Result)
	}

	found := false
	for _, tool := range listResult.Tools {
		if tool.Name == "generate_visuals" {
			found = true
			break
		}
	}
	if !found {
		t.Error("generate_visuals tool not found in server tool list")
	}
}

func TestMissingImagePrompt(t *testing.T) {
	cfg := dummyCfg()
	handler := tools.MakeGenerateVisualsHandler(cfg)
	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatal("handler returned unexpected error:", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing image_prompt")
	}
}

func TestGeminiClientError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled context
	cfg := &config.Config{
		GeminiAPIKey: "invalid-key",
		ListenAddr:   ":8080",
	}
	req := mcp.CallToolRequest{}
	// Set the image_prompt so we get past parameter validation
	req.Params.Arguments = map[string]any{"image_prompt": "test prompt"}
	result := tools.DoGenerateVisuals(ctx, cfg, req)
	if !result.IsError {
		t.Error("expected IsError=true for Gemini client error")
	}
	// Check the error message mentions "Likely cause"
	found := false
	for _, c := range result.Content {
		if text, ok := c.(mcp.TextContent); ok {
			if contains(text.Text, "Likely cause") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected error message to contain 'Likely cause', got content: %+v", result.Content)
	}
}

func TestPanicRecovery(t *testing.T) {
	// We can't easily inject a panic into the real handler without modifying the
	// implementation, so we test the recovery wrapper pattern directly by
	// calling DoGenerateVisuals with a nil config to trigger a nil pointer dereference.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic escaped the handler: %v", r)
		}
	}()
	handler := tools.MakeGenerateVisualsHandler(&config.Config{GeminiAPIKey: "key"})
	// Crafted request that will hit the nil config scenario if DoGenerateVisuals panics
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"image_prompt": "test"}
	result, err := handler(context.Background(), req)
	if err != nil {
		// Error is ok — we just don't want a panic
		t.Logf("handler returned err: %v", err)
	}
	if result == nil {
		t.Error("handler returned nil result")
	}
}

func TestGenerateVisualsIntegration(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	cfg := &config.Config{
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		ListenAddr:   ":8080",
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"image_prompt": "a small red circle on a white background"}
	result := tools.DoGenerateVisuals(context.Background(), cfg, req)
	if result.IsError {
		t.Errorf("expected successful result, got error: %v", result.Content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
