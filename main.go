package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"aka-webgui/internal/config"
	"aka-webgui/internal/logger"
	"aka-webgui/internal/server"
)

func main() {
	// Flags for systemd service management could be added here if using a library like kardianos/service
	// For now, we focus on the core requirement of being systemd-compatible (running in foreground/background)

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Setup(cfg)

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Info("Shutting down...")
		os.Exit(0)
	}()

	server.Run(cfg)
}
