package main

import (
	"ProxyService2/internal/access"
	adminapp "ProxyService2/internal/application/admin"
	proxyapp "ProxyService2/internal/application/proxy"
	"ProxyService2/internal/cache"
	"ProxyService2/internal/config"
	httpapi "ProxyService2/internal/httpapi"
	"ProxyService2/internal/logger"
	"ProxyService2/internal/observability"
	"ProxyService2/internal/rate_limit"
	"ProxyService2/internal/runtimeconfig"
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	configPath := os.Getenv("PROXY_CONFIG_PATH")
	if configPath == "" {
		configPath = filepath.Join("configs", "proxy.yaml")
	}

	if err := config.EnsureDefaultConfig(configPath); err != nil {
		panic(err)
	}

	store, err := config.NewStore(configPath)
	if err != nil {
		panic(err)
	}

	asyncLogger, err := logger.NewAsyncLogger(store.Current().Logging.Level)
	if err != nil {
		panic(err)
	}
	defer asyncLogger.Close()

	accessService := access.NewAccessService()
	rateLimiter := rate_limit.NewRateLimiter()
	cacheService := cache.NewCacheService()
	metrics := observability.NewObservability(runtimeconfig.ObservabilitySettingsFromConfig(store.Current()))

	applyRuntimeConfig := func(cfg config.Config) {
		asyncLogger.SetLevel(cfg.Logging.Level)
		metrics.UpdateSettings(runtimeconfig.ObservabilitySettingsFromConfig(cfg))
		if err := accessService.ApplyConfig(runtimeconfig.AccessSettingsFromConfig(cfg)); err != nil {
			asyncLogger.Error("failed to apply access configuration", map[string]any{"error": err.Error()})
		}
		rateLimiter.UpdateSettings(runtimeconfig.RateLimitSettingsFromConfig(cfg))
		cacheService.UpdateSettings(runtimeconfig.CacheSettingsFromConfig(cfg))
	}

	applyRuntimeConfig(store.Current())
	store.Subscribe(applyRuntimeConfig)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := store.Watch(ctx); err != nil {
		asyncLogger.Error("failed to watch config file", map[string]any{"error": err.Error()})
	}

	adminService := adminapp.NewService(store, accessService, rateLimiter, cacheService, metrics)
	proxyService := proxyapp.NewService(store, accessService, rateLimiter, cacheService, metrics, &http.Client{})
	httpAPI := httpapi.NewServer(store, adminService, proxyService, metrics, asyncLogger)
	httpAPI.StartBackgroundWorkers(ctx)

	cfg := store.Current()
	httpServer := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      httpAPI.Engine(),
		ReadTimeout:  config.ParseDurationOrDefault(cfg.Server.ReadTimeout, 15*time.Second),
		WriteTimeout: config.ParseDurationOrDefault(cfg.Server.WriteTimeout, 15*time.Second),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ParseDurationOrDefault(store.Current().Server.ShutdownTimeout, 5*time.Second))
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	asyncLogger.Info("proxy service starting", map[string]any{
		"addr":     cfg.Server.Address,
		"upstream": cfg.Proxy.UpstreamBaseURL,
	})

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		asyncLogger.Error("proxy service stopped unexpectedly", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}
