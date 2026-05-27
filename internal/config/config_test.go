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
	cfg := Load()
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
}

func TestConfigLoad_MissingKey(t *testing.T) {
	cmd := exec.Command("go", "test", "-run", "TestConfigLoad_MissingKey_Fatal", "-v", ".")
	cmd.Dir = filepath.Join("..")
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
	cfg := Load()
	if cfg.GeminiAPIKey != "from-env" {
		t.Errorf("GeminiAPIKey = %q, want %q (env should win over .env)", cfg.GeminiAPIKey, "from-env")
	}
}
