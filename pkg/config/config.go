package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

const (
	defaultAddress      = ":8080"
	defaultHealthPath   = "/healthz"
	defaultMetricsPath  = "/metrics"
	defaultValidatePath = "/validate"
)

type Config struct {
	Address            string
	HealthPath         string
	MetricsPath        string
	ValidatePath       string
	KubeconfigPath     string
	TLSCertFile        string
	TLSKeyFile         string
	LogLevel           slog.Level
	WatchResync        time.Duration
	StartupSyncTimeout time.Duration
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
}

func Load() (Config, error) {
	logLevel, err := parseLogLevel(getEnv("DRIFT_SENTINEL_LOG_LEVEL", "INFO"))
	if err != nil {
		return Config{}, err
	}

	readHeaderTimeout, err := parseDurationEnv("DRIFT_SENTINEL_READ_HEADER_TIMEOUT", "5s")
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := parseDurationEnv("DRIFT_SENTINEL_READ_TIMEOUT", "15s")
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := parseDurationEnv("DRIFT_SENTINEL_WRITE_TIMEOUT", "15s")
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := parseDurationEnv("DRIFT_SENTINEL_IDLE_TIMEOUT", "60s")
	if err != nil {
		return Config{}, err
	}

	watchResync, err := parseDurationEnv("DRIFT_SENTINEL_WATCH_RESYNC", "30s")
	if err != nil {
		return Config{}, err
	}

	startupSyncTimeout, err := parseDurationEnv("DRIFT_SENTINEL_STARTUP_SYNC_TIMEOUT", "30s")
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := parseDurationEnv("DRIFT_SENTINEL_SHUTDOWN_TIMEOUT", "10s")
	if err != nil {
		return Config{}, err
	}

	return Config{
		Address:            getEnv("DRIFT_SENTINEL_ADDRESS", defaultAddress),
		HealthPath:         getEnv("DRIFT_SENTINEL_HEALTH_PATH", defaultHealthPath),
		MetricsPath:        getEnv("DRIFT_SENTINEL_METRICS_PATH", defaultMetricsPath),
		ValidatePath:       getEnv("DRIFT_SENTINEL_VALIDATE_PATH", defaultValidatePath),
		KubeconfigPath:     getEnv("DRIFT_SENTINEL_KUBECONFIG", ""),
		TLSCertFile:        getEnv("DRIFT_SENTINEL_TLS_CERT_FILE", ""),
		TLSKeyFile:         getEnv("DRIFT_SENTINEL_TLS_KEY_FILE", ""),
		LogLevel:           logLevel,
		WatchResync:        watchResync,
		StartupSyncTimeout: startupSyncTimeout,
		ReadHeaderTimeout:  readHeaderTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		IdleTimeout:        idleTimeout,
		ShutdownTimeout:    shutdownTimeout,
	}, nil
}

func (c Config) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}

	return fallback
}

func parseDurationEnv(key, fallback string) (time.Duration, error) {
	value := getEnv(key, fallback)
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	return duration, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return 0, fmt.Errorf("parse DRIFT_SENTINEL_LOG_LEVEL: %w", err)
	}

	return level, nil
}
