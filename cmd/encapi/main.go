// Command encapi is the ENC HTTP server: it serves Puppet's node classification
// documents and the read/write API backing the CLI and Terraform provider.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/IUCLabs/encapi/internal/config"
	"github.com/IUCLabs/encapi/internal/database"
	"github.com/IUCLabs/encapi/internal/distro"
	"github.com/IUCLabs/encapi/internal/server"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Info("starting encapi", "version", version)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.DatabaseDSN())
	if err != nil {
		slog.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if cfg.WriteToken == "" {
		slog.Warn("ENCAPI_WRITE_TOKEN is not set; write endpoints are disabled")
	}

	srv := server.New(db, distro.New(cfg.DistroAPIURL), cfg.WriteToken)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.ListenAndServe(ctx, cfg.ListenAddr); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
