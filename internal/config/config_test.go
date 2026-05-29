package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestConfigLoad_FromEnv(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("AUTHELIA_URL", "https://authelia.example.com")
	t.Setenv("AUTHELIA_CLIENT_ID", "test-client")
	t.Setenv("MCP_PUBLIC_URL", "https://mcp.example.com")
	cfg := Load()
	if cfg.GeminiAPIKey != "test-key" {
		t.Errorf("GeminiAPIKey = %q, want %q", cfg.GeminiAPIKey, "test-key")
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
}

func TestConfigLoad_DefaultListenAddr(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("AUTHELIA_URL", "https://authelia.example.com")
	t.Setenv("AUTHELIA_CLIENT_ID", "test-client")
	t.Setenv("MCP_PUBLIC_URL", "https://mcp.example.com")
	cfg := Load()
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
}

func TestConfigLoad_MissingKey(t *testing.T) {
	cmd := exec.Command("go", "test", "-run", "TestConfigLoad_MissingKey_Fatal", "-v", "github.com/thamwangjun/generate-visuals-mcp/internal/config")
	cmd.Env = append(os.Environ(), "GEMINI_API_KEY=", "CONFIG_TEST_SUBPROCESS=1")
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() == 0 {
		t.Logf("output: %s", out)
		t.Error("expected non-zero exit code when GEMINI_API_KEY is missing")
	}
}

// TestConfigLoad_MissingKey_Fatal is the subprocess target for TestConfigLoad_MissingKey.
// It only runs when CONFIG_TEST_SUBPROCESS=1 is set.
func TestConfigLoad_MissingKey_Fatal(t *testing.T) {
	if os.Getenv("CONFIG_TEST_SUBPROCESS") != "1" {
		t.Skip("subprocess-only test")
	}
	t.Setenv("GEMINI_API_KEY", "")
	Load() // should call log.Fatal and exit
}

func TestConfigLoad_MissingAutheliaURL(t *testing.T) {
	env := append(os.Environ(),
		"GEMINI_API_KEY=test-key",
		"AUTHELIA_URL=",
		"AUTHELIA_CLIENT_ID=test-client",
		"MCP_PUBLIC_URL=https://mcp.example.com",
		"CONFIG_TEST_SUBPROCESS=1",
	)
	cmd := exec.Command("go", "test", "-run", "TestConfigLoad_MissingAutheliaURL_Fatal", "-v", "github.com/thamwangjun/generate-visuals-mcp/internal/config")
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() == 0 {
		t.Logf("output: %s", out)
		t.Error("expected non-zero exit code when AUTHELIA_URL is missing")
	}
}

func TestConfigLoad_MissingAutheliaURL_Fatal(t *testing.T) {
	if os.Getenv("CONFIG_TEST_SUBPROCESS") != "1" {
		t.Skip("subprocess-only test")
	}
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("AUTHELIA_URL", "")
	t.Setenv("AUTHELIA_CLIENT_ID", "test-client")
	t.Setenv("MCP_PUBLIC_URL", "https://mcp.example.com")
	Load()
}

func TestConfigLoad_MissingAutheliaClientID(t *testing.T) {
	env := append(os.Environ(),
		"GEMINI_API_KEY=test-key",
		"AUTHELIA_URL=https://authelia.example.com",
		"AUTHELIA_CLIENT_ID=",
		"MCP_PUBLIC_URL=https://mcp.example.com",
		"CONFIG_TEST_SUBPROCESS=1",
	)
	cmd := exec.Command("go", "test", "-run", "TestConfigLoad_MissingAutheliaClientID_Fatal", "-v", "github.com/thamwangjun/generate-visuals-mcp/internal/config")
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() == 0 {
		t.Logf("output: %s", out)
		t.Error("expected non-zero exit code when AUTHELIA_CLIENT_ID is missing")
	}
}

func TestConfigLoad_MissingAutheliaClientID_Fatal(t *testing.T) {
	if os.Getenv("CONFIG_TEST_SUBPROCESS") != "1" {
		t.Skip("subprocess-only test")
	}
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("AUTHELIA_URL", "https://authelia.example.com")
	t.Setenv("AUTHELIA_CLIENT_ID", "")
	t.Setenv("MCP_PUBLIC_URL", "https://mcp.example.com")
	Load()
}

func TestConfigLoad_MissingPublicURL(t *testing.T) {
	env := append(os.Environ(),
		"GEMINI_API_KEY=test-key",
		"AUTHELIA_URL=https://authelia.example.com",
		"AUTHELIA_CLIENT_ID=test-client",
		"MCP_PUBLIC_URL=",
		"CONFIG_TEST_SUBPROCESS=1",
	)
	cmd := exec.Command("go", "test", "-run", "TestConfigLoad_MissingPublicURL_Fatal", "-v", "github.com/thamwangjun/generate-visuals-mcp/internal/config")
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() == 0 {
		t.Logf("output: %s", out)
		t.Error("expected non-zero exit code when MCP_PUBLIC_URL is missing")
	}
}

func TestConfigLoad_MissingPublicURL_Fatal(t *testing.T) {
	if os.Getenv("CONFIG_TEST_SUBPROCESS") != "1" {
		t.Skip("subprocess-only test")
	}
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("AUTHELIA_URL", "https://authelia.example.com")
	t.Setenv("AUTHELIA_CLIENT_ID", "test-client")
	t.Setenv("MCP_PUBLIC_URL", "")
	Load()
}

func TestConfigLoad_TrailingSlashStripped(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("AUTHELIA_URL", "https://authelia.example.com/")
	t.Setenv("AUTHELIA_CLIENT_ID", "test-client")
	t.Setenv("MCP_PUBLIC_URL", "https://mcp.example.com")
	cfg := Load()
	if cfg.AutheliaBaseURL != "https://authelia.example.com" {
		t.Errorf("AutheliaBaseURL = %q, want %q", cfg.AutheliaBaseURL, "https://authelia.example.com")
	}
}

func TestConfigLoad_EnvWinsOverDotenv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("GEMINI_API_KEY=from-dotenv\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Change into the temp dir so godotenv.Load() picks up our .env
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	t.Setenv("GEMINI_API_KEY", "from-env")
	t.Setenv("AUTHELIA_URL", "https://authelia.example.com")
	t.Setenv("AUTHELIA_CLIENT_ID", "test-client")
	t.Setenv("MCP_PUBLIC_URL", "https://mcp.example.com")
	cfg := Load()
	if cfg.GeminiAPIKey != "from-env" {
		t.Errorf("GeminiAPIKey = %q, want %q (env should win over .env)", cfg.GeminiAPIKey, "from-env")
	}
}
