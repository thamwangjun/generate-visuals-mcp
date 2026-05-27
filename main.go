package main

import (
	"log"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/thamwangjun/generate-visuals-mcp/internal/config"
	"github.com/thamwangjun/generate-visuals-mcp/internal/tools"
)

func main() {
	cfg := config.Load()

	mcpServer := server.NewMCPServer(
		"generate-visuals-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	tools.Register(mcpServer, cfg)

	httpHandler := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))
	// Phase 2: wrap httpHandler with auth middleware here — see internal/auth/

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("generate-visuals-mcp listening on %s", cfg.ListenAddr)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
