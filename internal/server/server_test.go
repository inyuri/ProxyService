package server

import (
	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/repository"
	"ProxyService2/internal/usecase"
	"ProxyService2/pkg/logger"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestServerProxyCachesResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Access.DefaultPolicy = "allow"
	cfg.Access.Rules = nil

	dir := t.TempDir()
	configPath := filepath.Join(dir, "proxy.yaml")
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, raw, 0o644))

	store, err := config.NewStore(configPath)
	require.NoError(t, err)

	log, err := logger.NewAsyncLogger("error")
	require.NoError(t, err)
	defer log.Close()

	accessService := repository.NewAccessService()
	require.NoError(t, accessService.ApplyConfig(domain.AccessSettings{
		DefaultPolicy: "allow",
		CacheTTL:      time.Minute,
		CacheSize:     64,
	}))
	rate := repository.NewRateLimiter()
	cacheService := repository.NewCacheService()
	metrics := repository.NewObservability(domain.ObservabilitySettings{MaxLogs: 50, MaxBuckets: 10})
	adminSvc := usecase.NewAdminUseCase(store, accessService, rate, cacheService, metrics)
	proxySvc := usecase.NewProxyUseCase(store, accessService, rate, cacheService, metrics, upstream.Client())

	srv := NewServer(store, adminSvc, proxySvc, metrics, log)

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/proxy/anything/cacheable", nil)
	firstReq.RemoteAddr = "127.0.0.1:12345"
	srv.Engine().ServeHTTP(first, firstReq)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "MISS", first.Header().Get("X-Cache-Status"))

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "/proxy/anything/cacheable", nil)
	secondReq.RemoteAddr = "127.0.0.1:12345"
	srv.Engine().ServeHTTP(second, secondReq)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "HIT", second.Header().Get("X-Cache-Status"))
	require.Equal(t, int64(1), upstreamCalls.Load())
}

func TestServerAccessCheckEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.DefaultConfig()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "proxy.yaml")
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, raw, 0o644))

	store, err := config.NewStore(configPath)
	require.NoError(t, err)

	log, err := logger.NewAsyncLogger("error")
	require.NoError(t, err)
	defer log.Close()

	accessService := repository.NewAccessService()
	require.NoError(t, accessService.ApplyConfig(domain.AccessSettings{
		DefaultPolicy: "deny",
		CacheTTL:      time.Minute,
		CacheSize:     64,
		Rules: []domain.AccessRule{
			{ID: "allow-local", Type: domain.AccessRuleAllow, Value: "127.0.0.1"},
		},
	}))

	rate := repository.NewRateLimiter()
	cacheService := repository.NewCacheService()
	metrics := repository.NewObservability(domain.ObservabilitySettings{})
	adminSvc := usecase.NewAdminUseCase(store, accessService, rate, cacheService, metrics)
	proxySvc := usecase.NewProxyUseCase(store, accessService, rate, cacheService, metrics, nil)
	srv := NewServer(store, adminSvc, proxySvc, metrics, log)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/access/check?ip=127.0.0.1", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	srv.Engine().ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(strings.NewReader(recorder.Body.String())).Decode(&payload))
	require.Equal(t, true, payload["allowed"])
}
