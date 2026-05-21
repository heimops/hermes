package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hermes.io/migrator/internal/config"
	"hermes.io/migrator/internal/migrator"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	m := migrator.New(cfg)
	if err := m.Run(ctx); err != nil {
		slog.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Migration completed successfully")
}
