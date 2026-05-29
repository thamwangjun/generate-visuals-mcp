package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/thamwangjun/generate-visuals-mcp/internal/auth"
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

	httpServer := server.NewStreamableHTTPServer(mcpServer, server.WithEndpointPath("/mcp"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	validator, err := auth.NewValidatorAsync(ctx, cfg)
	if err != nil {
		log.Fatalf("auth: failed to initialize validator: %v", err)
	}

	prmURL := cfg.PublicBaseURL + "/.well-known/oauth-protected-resource"

	prmHandler := server.NewProtectedResourceMetadataHandler(server.ProtectedResourceMetadataConfig{
		Resource:               cfg.PublicBaseURL,
		AuthorizationServers:   []string{cfg.AutheliaBaseURL},
		ScopesSupported:        []string{"openid", "profile"},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "generate-visuals-mcp",
	})

	mux := http.NewServeMux()
	protected := validator.Middleware(prmURL)(httpServer)
	mux.Handle("/mcp", protected)
	mux.Handle("/mcp/", protected)
	mux.Handle("/.well-known/oauth-protected-resource", prmHandler)

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		stop() // release signal resources
		log.Printf("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Printf("generate-visuals-mcp listening on %s", cfg.ListenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
