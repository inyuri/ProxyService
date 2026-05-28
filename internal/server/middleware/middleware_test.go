package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	configpkg "ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/mocks"
	"ProxyService2/internal/server/middleware"
	"ProxyService2/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newProxyUCForMiddleware(t *testing.T) (
	*usecase.ProxyUseCase,
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
	return usecase.NewProxyUseCase(store, access, rate, cache, obs, nil), store, access, rate, cache, obs
}

// ─── AccessMiddleware ─────────────────────────────────────────────────────────

func TestAccessMiddleware_Allowed(t *testing.T) {
	proxy, _, access, _, _, _ := newProxyUCForMiddleware(t)
	access.EXPECT().Settings().Return(domain.AccessSettings{CaptchaHeader: "X-Captcha-Token"})
	access.EXPECT().Check(gomock.Any(), "").Return(domain.AccessDecision{Allowed: true, Reason: "whitelist"})

	r := gin.New()
	r.Use(middleware.AccessMiddleware(proxy))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAccessMiddleware_Denied_Blacklist(t *testing.T) {
	proxy, _, access, _, _, _ := newProxyUCForMiddleware(t)
	access.EXPECT().Settings().Return(domain.AccessSettings{CaptchaHeader: "X-Captcha-Token"})
	access.EXPECT().Check(gomock.Any(), "").Return(domain.AccessDecision{Allowed: false, Reason: "blacklist"})

	r := gin.New()
	r.Use(middleware.AccessMiddleware(proxy))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAccessMiddleware_Denied_Captcha(t *testing.T) {
	proxy, _, access, _, _, _ := newProxyUCForMiddleware(t)
	access.EXPECT().Settings().Return(domain.AccessSettings{CaptchaHeader: "X-Captcha-Token"})
	access.EXPECT().Check(gomock.Any(), "").Return(domain.AccessDecision{
		Allowed:         false,
		RequiresCaptcha: true,
		Reason:          "captcha_required",
	})

	r := gin.New()
	r.Use(middleware.AccessMiddleware(proxy))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.50:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	var body map[string]any
	require.NoError(t, parseJSON(rec.Body.Bytes(), &body))
	require.Equal(t, true, body["requiresCaptcha"])
}

func TestAccessMiddleware_Denied_DefaultPolicy(t *testing.T) {
	proxy, _, access, _, _, _ := newProxyUCForMiddleware(t)
	access.EXPECT().Settings().Return(domain.AccessSettings{CaptchaHeader: "X-Captcha"})
	access.EXPECT().Check(gomock.Any(), "").Return(domain.AccessDecision{Allowed: false, Reason: "default_deny"})

	r := gin.New()
	r.Use(middleware.AccessMiddleware(proxy))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// ─── RateLimitMiddleware ──────────────────────────────────────────────────────

func TestRateLimitMiddleware_Allowed(t *testing.T) {
	proxy, _, _, rate, _, _ := newProxyUCForMiddleware(t)
	lease := &domain.RateLimitLease{Keys: []string{"ip:127.0.0.1"}}
	rate.EXPECT().Acquire(gomock.Any(), int64(0), gomock.Any()).Return(lease, nil)

	r := gin.New()
	r.Use(middleware.RateLimitMiddleware(proxy))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimitMiddleware_Exceeded(t *testing.T) {
	proxy, _, _, rate, _, _ := newProxyUCForMiddleware(t)
	violation := &domain.RateLimitViolation{IP: "127.0.0.1", Limit: "RPS", Exceeded: "101/100"}
	rate.EXPECT().Acquire(gomock.Any(), int64(0), gomock.Any()).Return(nil, violation)

	r := gin.New()
	r.Use(middleware.RateLimitMiddleware(proxy))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// ─── TelemetryMiddleware ──────────────────────────────────────────────────────

func TestTelemetryMiddleware_RecordsRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	obs := mocks.NewMockObservabilityService(ctrl)
	logger := mocks.NewMockLogger(ctrl)

	obs.EXPECT().IncActive()
	obs.EXPECT().DecActive()
	obs.EXPECT().Record(gomock.Any())
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())

	proxy := usecase.NewProxyUseCase(
		mocks.NewMockConfigStore(ctrl),
		mocks.NewMockAccessService(ctrl),
		mocks.NewMockRateLimiter(ctrl),
		mocks.NewMockCacheService(ctrl),
		obs,
		nil,
	)

	r := gin.New()
	r.Use(middleware.TelemetryMiddleware(obs, proxy, logger))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTelemetryMiddleware_ServerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	obs := mocks.NewMockObservabilityService(ctrl)
	logger := mocks.NewMockLogger(ctrl)

	obs.EXPECT().IncActive()
	obs.EXPECT().DecActive()
	obs.EXPECT().Record(gomock.Any())
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())

	proxy := usecase.NewProxyUseCase(
		mocks.NewMockConfigStore(ctrl),
		mocks.NewMockAccessService(ctrl),
		mocks.NewMockRateLimiter(ctrl),
		mocks.NewMockCacheService(ctrl),
		obs,
		nil,
	)

	r := gin.New()
	r.Use(middleware.TelemetryMiddleware(obs, proxy, logger))
	r.GET("/err", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestTelemetryMiddleware_WithRateLease(t *testing.T) {
	ctrl := gomock.NewController(t)
	obs := mocks.NewMockObservabilityService(ctrl)
	logger := mocks.NewMockLogger(ctrl)
	rate := mocks.NewMockRateLimiter(ctrl)

	lease := &domain.RateLimitLease{Keys: []string{"ip:127.0.0.1"}}

	obs.EXPECT().IncActive()
	obs.EXPECT().DecActive()
	obs.EXPECT().Record(gomock.Any())
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())
	rate.EXPECT().Release(lease, gomock.Any(), gomock.Any())

	proxy := usecase.NewProxyUseCase(
		mocks.NewMockConfigStore(ctrl),
		mocks.NewMockAccessService(ctrl),
		rate,
		mocks.NewMockCacheService(ctrl),
		obs,
		nil,
	)

	r := gin.New()
	r.Use(middleware.TelemetryMiddleware(obs, proxy, logger))
	r.GET("/test", func(c *gin.Context) {
		c.Set(middleware.CtxRateLeaseKey, lease)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── CorsMiddleware ───────────────────────────────────────────────────────────

func TestCorsMiddleware_AllowedOrigin(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockConfigStore(ctrl)
	store.EXPECT().Current().Return(configWithOrigins([]string{"http://localhost:5173"})).AnyTimes()

	r := gin.New()
	r.Use(middleware.CorsMiddleware(store))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCorsMiddleware_Preflight(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockConfigStore(ctrl)
	store.EXPECT().Current().Return(configWithOrigins([]string{"*"})).AnyTimes()

	r := gin.New()
	r.Use(middleware.CorsMiddleware(store))
	r.OPTIONS("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://anywhere.com")
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCorsMiddleware_DisallowedOrigin(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockConfigStore(ctrl)
	store.EXPECT().Current().Return(configWithOrigins([]string{"http://allowed.com"})).AnyTimes()

	r := gin.New()
	r.Use(middleware.CorsMiddleware(store))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCorsMiddleware_NoOrigin(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := mocks.NewMockConfigStore(ctrl)
	store.EXPECT().Current().Return(configWithOrigins(nil)).AnyTimes()

	r := gin.New()
	r.Use(middleware.CorsMiddleware(store))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func configWithOrigins(origins []string) configpkg.Config {
	cfg := configpkg.Config{}
	cfg.Frontend.AllowedOrigins = origins
	return cfg
}

func parseJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
