package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"drift-sentinel/pkg/admission"
	"drift-sentinel/pkg/config"
	"drift-sentinel/pkg/health"
	"drift-sentinel/pkg/metrics"
	"drift-sentinel/pkg/rules"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := metrics.NewRegistry()
	ruleStore := rules.NewStore()
	kubernetesClient, err := config.NewKubernetesClient(cfg.KubeconfigPath)
	if err != nil {
		logger.Error("failed to create kubernetes client", "error", err)
		os.Exit(1)
	}

	ruleController := rules.NewController(kubernetesClient, ruleStore, logger, registry, cfg.WatchResync)
	if err := ruleController.Start(ctx, cfg.StartupSyncTimeout); err != nil {
		logger.Error("failed to start kubernetes caches", "error", err)
		os.Exit(1)
	}

	validator := admission.NewValidator(ruleStore, ruleController.NamespaceModeResolver())

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.HealthPath, health.Handler)
	mux.Handle(cfg.MetricsPath, registry.Handler())
	mux.Handle(cfg.ValidatePath, admission.NewHandler(logger, validator, registry))

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "address", cfg.Address, "tls_enabled", cfg.TLSEnabled())

		var serveErr error
		if cfg.TLSEnabled() {
			serveErr = server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			serveErr = server.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case serveErr := <-errCh:
		if serveErr != nil {
			logger.Error("server stopped unexpectedly", "error", serveErr)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
