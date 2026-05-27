package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GeminiAPIKey     string
	ListenAddr       string
	PublicBaseURL    string
	AutheliaBaseURL  string
	AutheliaClientID string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		ListenAddr:       getenv("LISTEN_ADDR", ":8080"),
		PublicBaseURL:    os.Getenv("MCP_PUBLIC_URL"),
		AutheliaBaseURL:  os.Getenv("AUTHELIA_URL"),
		AutheliaClientID: os.Getenv("AUTHELIA_CLIENT_ID"),
	}

	if cfg.GeminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required but not set. Set it in environment or .env file.")
	}

	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
