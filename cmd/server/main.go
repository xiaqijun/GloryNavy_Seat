package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/app"
	"glorynavy.local/seat/internal/config"
	"glorynavy.local/seat/internal/platform/dbobserve"
)

var version = "development"

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = 0
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.ConnConfig.ConnectTimeout = 3 * time.Second
	if cfg.DBSlowQuery > 0 {
		poolConfig.ConnConfig.Tracer = dbobserve.New(logger, cfg.DBSlowQuery)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return errors.New("database pool initialization failed")
	}
	defer pool.Close()
	handler, err := app.New(pool, logger, version, cfg.Modules, app.AuthConfig{Origin: cfg.PublicOrigin, ClientID: cfg.EVEClientID, ClientSecret: cfg.EVEClientSecret, TokenKey: cfg.EVETokenKey, ESIUserAgent: cfg.ESIUserAgent, SDEAutoUpdate: cfg.SDEAutoUpdate, SDEWorkDir: cfg.SDEWorkDir, SDECheckInterval: cfg.SDECheckInterval, WinterCoPAPURL: cfg.WinterCoPAPURL, WinterCoPAPAuthFile: cfg.WinterCoPAPAuth, QQBotAppID: cfg.QQBotAppID, QQBotAppSecret: cfg.QQBotAppSecret, QQBotAPIBase: cfg.QQBotAPIBase, QQBotGroupOpenIDs: cfg.QQBotGroupOpenIDs, SentryIntegrationURL: cfg.SentryIntegrationURL, SentryIntegrationToken: cfg.SentryIntegrationToken, AlertConsumptionEnabled: cfg.AlertConsumptionEnabled, AlertPriceVersion: cfg.AlertPriceVersion, AlertUnitSeconds: cfg.AlertUnitSeconds, AlertUnitPriceMinor: cfg.AlertUnitPriceMinor, AlertMaxGrantSeconds: cfg.AlertMaxGrantSeconds, AlertGrantTTL: cfg.AlertGrantTTL, ApprovalDualRead: cfg.ApprovalDualRead, ApprovalIndexAccounts: cfg.ApprovalIndexAccounts})
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("HTTP listener could not start")
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	workerCtx, stopWorker := context.WithCancel(stop)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); handler.Run(workerCtx) }()
	defer func() { stopWorker(); <-workerDone }()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	logger.Info("server started", "address", cfg.HTTPAddr, "environment", cfg.Environment, "version", version)
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server stopped unexpectedly")
		}
	case <-stop.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
			return errors.New("HTTP shutdown deadline exceeded")
		}
	}
	logger.Info("server stopped")
	return nil
}
