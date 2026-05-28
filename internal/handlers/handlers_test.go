package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/handlers"
	"ProxyService2/internal/mocks"
	"ProxyService2/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newAdminUCMocked(t *testing.T) (
	*usecase.AdminUseCase,
	*mocks.MockConfigStore,
	*mocks.MockAccessService,
	*mocks.MockRateLimiter,
	*mocks.MockCacheService,
	*mocks.MockObservabilityService,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockConfigStore(ctrl)
	access := mocks.NewMockAccessService(ctrl)
	rate := mocks.NewMockRateLimiter(ctrl)
	cache := mocks.NewMockCacheService(ctrl)
	obs := mocks.NewMockObservabilityService(ctrl)
	return usecase.NewAdminUseCase(store, access, rate, cache, obs), store, access, rate, cache, obs
}

// ─── AccessHandler ────────────────────────────────────────────────────────────

func TestAccessHandler_List(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUCMocked(t)
	access.EXPECT().List().Return([]domain.AccessRule{{ID: "1", Type: domain.AccessRuleAllow, Value: "10.0.0.1"}})

	r := gin.New()
	r.GET("/access/rules", handlers.NewAccessHandler(uc).List)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/access/rules", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var result []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Len(t, result, 1)
}

func TestAccessHandler_Create_Valid(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUCMocked(t)
	store.EXPECT().Update(gomock.Any()).Return(nil)

	r := gin.New()
	r.POST("/access/rules", handlers.NewAccessHandler(uc).Create)

	body, _ := json.Marshal(map[string]string{"value": "192.168.1.1", "type": "allow"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/access/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestAccessHandler_Create_InvalidJSON(t *testing.T) {
	uc, _, _, _, _, _ := newAdminUCMocked(t)

	r := gin.New()
	r.POST("/access/rules", handlers.NewAccessHandler(uc).Create)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/access/rules", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccessHandler_Create_InvalidIP(t *testing.T) {
	uc, _, _, _, _, _ := newAdminUCMocked(t)

	r := gin.New()
	r.POST("/access/rules", handlers.NewAccessHandler(uc).Create)

	body, _ := json.Marshal(map[string]string{"value": "not-an-ip", "type": "allow"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/access/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccessHandler_Delete_Found(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUCMocked(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		cfg := &config.Config{}
		cfg.Access.Rules = []config.AccessRuleConfig{{ID: "abc", Value: "1.1.1.1", Type: "allow"}}
		return fn(cfg)
	})

	r := gin.New()
	r.DELETE("/access/rules/:id", handlers.NewAccessHandler(uc).Delete)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/access/rules/abc", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAccessHandler_Delete_NotFound(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUCMocked(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		return fn(&config.Config{})
	})

	r := gin.New()
	r.DELETE("/access/rules/:id", handlers.NewAccessHandler(uc).Delete)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/access/rules/missing", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAccessHandler_Check(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUCMocked(t)
	access.EXPECT().Check("1.2.3.4", "").Return(domain.AccessDecision{Allowed: true, Reason: "whitelist"})

	r := gin.New()
	r.GET("/access/check", handlers.NewAccessHandler(uc).Check)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/access/check?ip=1.2.3.4", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, true, result["allowed"])
}

// ─── CacheHandler ─────────────────────────────────────────────────────────────

func TestCacheHandler_Snapshot(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUCMocked(t)
	cache.EXPECT().Snapshot(100).Return(domain.CacheSnapshot{Stats: domain.CacheStats{Hits: 5}})

	r := gin.New()
	r.GET("/cache", handlers.NewCacheHandler(uc).Snapshot)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cache", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestCacheHandler_Invalidate_Valid(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUCMocked(t)
	cache.EXPECT().Invalidate(gomock.Any()).Return(3, nil)

	r := gin.New()
	r.POST("/cache/invalidate", handlers.NewCacheHandler(uc).Invalidate)

	body, _ := json.Marshal(domain.CacheInvalidationRequest{Prefix: "/api"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cache/invalidate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, float64(3), result["removed"])
}

func TestCacheHandler_Invalidate_BadJSON(t *testing.T) {
	uc, _, _, _, _, _ := newAdminUCMocked(t)

	r := gin.New()
	r.POST("/cache/invalidate", handlers.NewCacheHandler(uc).Invalidate)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cache/invalidate", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCacheHandler_Clear(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUCMocked(t)
	cache.EXPECT().InvalidateAll().Return(12)

	r := gin.New()
	r.DELETE("/cache", handlers.NewCacheHandler(uc).Clear)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/cache", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, float64(12), result["removed"])
}

// ─── RateLimitHandler ─────────────────────────────────────────────────────────

func TestRateLimitHandler_Get(t *testing.T) {
	uc, _, _, rate, _, obs := newAdminUCMocked(t)
	rate.EXPECT().Settings().Return(domain.RateLimitSettings{RPS: 100})
	obs.EXPECT().TopClients(10).Return(nil)
	rate.EXPECT().TopViolators(50).Return(nil)

	r := gin.New()
	r.GET("/rate-limits", handlers.NewRateLimitHandler(uc).Get)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rate-limits", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimitHandler_Update_Valid(t *testing.T) {
	uc, store, _, rate, _, _ := newAdminUCMocked(t)
	store.EXPECT().Update(gomock.Any()).Return(nil)
	rate.EXPECT().Settings().Return(domain.RateLimitSettings{RPS: 500})

	r := gin.New()
	r.PUT("/rate-limits", handlers.NewRateLimitHandler(uc).Update)

	body, _ := json.Marshal(usecase.RateLimitUpdateRequest{RPS: 500})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/rate-limits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimitHandler_Update_BadJSON(t *testing.T) {
	uc, _, _, _, _, _ := newAdminUCMocked(t)

	r := gin.New()
	r.PUT("/rate-limits", handlers.NewRateLimitHandler(uc).Update)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/rate-limits", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRateLimitHandler_Violations(t *testing.T) {
	uc, _, _, rate, _, _ := newAdminUCMocked(t)
	rate.EXPECT().TopViolators(100).Return([]domain.RateLimitViolation{{IP: "1.1.1.1"}})

	r := gin.New()
	r.GET("/rate-limits/violations", handlers.NewRateLimitHandler(uc).Violations)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rate-limits/violations", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── MonitoringHandler ────────────────────────────────────────────────────────

func TestMonitoringHandler_Monitoring(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUCMocked(t)
	obs.EXPECT().Monitoring(10).Return(domain.MonitoringSnapshot{})

	r := gin.New()
	r.GET("/monitoring", handlers.NewMonitoringHandler(uc).Monitoring)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/monitoring", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── DashboardHandler ─────────────────────────────────────────────────────────

func TestDashboardHandler_Overview(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUCMocked(t)
	obs.EXPECT().Overview().Return(domain.DashboardOverview{RPS: 10})

	r := gin.New()
	r.GET("/dashboard/overview", handlers.NewDashboardHandler(uc).Overview)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── LogsHandler ──────────────────────────────────────────────────────────────

func TestLogsHandler_Logs(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUCMocked(t)
	obs.EXPECT().Logs("", 0, 200).Return([]domain.RequestLog{{IP: "1.2.3.4", Status: 200}})

	r := gin.New()
	r.GET("/logs", handlers.NewLogsHandler(uc).Logs)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestLogsHandler_Logs_WithStatus(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUCMocked(t)
	obs.EXPECT().Logs("", 404, 200).Return(nil)

	r := gin.New()
	r.GET("/logs", handlers.NewLogsHandler(uc).Logs)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs?status=404", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestLogsHandler_Export(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUCMocked(t)
	obs.EXPECT().Logs("", 0, 500).Return([]domain.RequestLog{{IP: "9.9.9.9"}})

	r := gin.New()
	r.GET("/logs/export", handlers.NewLogsHandler(uc).Export)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs/export", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Disposition"), "proxy-logs.json")
}

// ─── HealthHandler ────────────────────────────────────────────────────────────

func TestHealthHandler_Handle(t *testing.T) {
	r := gin.New()
	r.GET("/health", handlers.NewHealthHandler().Handle)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── ProxyHandler ─────────────────────────────────────────────────────────────

func TestProxyHandler_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	store := mocks.NewMockConfigStore(ctrl)
	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cache := mocks.NewMockCacheService(ctrl)
	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{}, false, "disabled")
	cache.EXPECT().BuildKey(gomock.Any(), gomock.Any()).Return("key").AnyTimes()

	obs := mocks.NewMockObservabilityService(ctrl)
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())

	proxySvc := usecase.NewProxyUseCase(store,
		mocks.NewMockAccessService(ctrl),
		mocks.NewMockRateLimiter(ctrl),
		cache, obs, upstream.Client())

	r := gin.New()
	r.Any("/proxy/*path", handlers.NewProxyHandler(proxySvc).Proxy)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/anything", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestProxyHandler_UpstreamError(t *testing.T) {
	ctrl := gomock.NewController(t)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	store := mocks.NewMockConfigStore(ctrl)
	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = dead.URL
	cfg.Proxy.Timeout = "1s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cache := mocks.NewMockCacheService(ctrl)
	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{}, false, "")
	cache.EXPECT().BuildKey(gomock.Any(), gomock.Any()).Return("key").AnyTimes()

	obs := mocks.NewMockObservabilityService(ctrl)
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())

	proxySvc := usecase.NewProxyUseCase(store,
		mocks.NewMockAccessService(ctrl),
		mocks.NewMockRateLimiter(ctrl),
		cache, obs, dead.Client())

	r := gin.New()
	r.Any("/proxy/*path", handlers.NewProxyHandler(proxySvc).Proxy)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/fail", nil))
	require.Equal(t, http.StatusBadGateway, rec.Code)
}
