package usecase_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/mocks"
	"ProxyService2/internal/usecase"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newProxyUC(t *testing.T, client *http.Client) (
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
	return usecase.NewProxyUseCase(store, access, rate, cache, obs, client), store, access, rate, cache, obs
}

func TestProxyUseCase_CaptchaHeader(t *testing.T) {
	uc, _, access, _, _, _ := newProxyUC(t, nil)
	access.EXPECT().Settings().Return(domain.AccessSettings{CaptchaHeader: "X-My-Captcha"})
	require.Equal(t, "X-My-Captcha", uc.CaptchaHeader())
}

func TestProxyUseCase_EvaluateAccess(t *testing.T) {
	uc, _, access, _, _, _ := newProxyUC(t, nil)
	want := domain.AccessDecision{Allowed: true, Reason: "whitelist"}
	access.EXPECT().Check("1.2.3.4", "token").Return(want)
	require.Equal(t, want, uc.EvaluateAccess("1.2.3.4", "token"))
}

func TestProxyUseCase_AcquireRateLease_OK(t *testing.T) {
	uc, _, _, rate, _, _ := newProxyUC(t, nil)
	now := time.Now()
	lease := &domain.RateLimitLease{Keys: []string{"ip:1.2.3.4"}, Now: now}
	rate.EXPECT().Acquire("1.2.3.4", int64(0), gomock.Any()).Return(lease, nil)
	got, violation := uc.AcquireRateLease("1.2.3.4", 0, now)
	require.Nil(t, violation)
	require.Equal(t, lease, got)
}

func TestProxyUseCase_AcquireRateLease_Exceeded(t *testing.T) {
	uc, _, _, rate, _, _ := newProxyUC(t, nil)
	now := time.Now()
	violation := &domain.RateLimitViolation{IP: "1.2.3.4", Limit: "RPS"}
	rate.EXPECT().Acquire("1.2.3.4", int64(0), gomock.Any()).Return(nil, violation)
	lease, got := uc.AcquireRateLease("1.2.3.4", 0, now)
	require.Nil(t, lease)
	require.Equal(t, violation, got)
}

func TestProxyUseCase_ReleaseRateLease(t *testing.T) {
	uc, _, _, rate, _, _ := newProxyUC(t, nil)
	lease := &domain.RateLimitLease{Keys: []string{"ip:1.2.3.4"}}
	rate.EXPECT().Release(lease, int64(100), int64(200))
	uc.ReleaseRateLease(lease, 100, 200)
}

func TestProxyUseCase_ReleaseRateLease_Nil(t *testing.T) {
	uc, _, _, rate, _, _ := newProxyUC(t, nil)
	rate.EXPECT().Release((*domain.RateLimitLease)(nil), int64(0), int64(0))
	uc.ReleaseRateLease(nil, 0, 0)
}

func TestProxyUseCase_Forward_CacheHit(t *testing.T) {
	uc, store, _, _, cache, _ := newProxyUC(t, nil)

	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = "http://upstream.test"
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cached := &domain.CachedResponse{
		Status: http.StatusOK,
		Header: http.Header{},
		Body:   []byte(`{"cached":true}`),
	}
	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{}, true, "")
	cache.EXPECT().BuildKey(http.MethodGet, gomock.Any()).Return("hit-key")
	cache.EXPECT().Get("hit-key").Return(cached, true)

	resp, err := uc.Forward(usecase.ForwardRequest{
		Context:     context.Background(),
		Method:      http.MethodGet,
		RequestPath: "/foo",
	})
	require.NoError(t, err)
	require.Equal(t, "HIT", resp.CacheStatus)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestProxyUseCase_Forward_CacheMiss(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	uc, store, _, _, cache, obs := newProxyUC(t, upstream.Client())

	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{TTL2xx: 5 * time.Minute}, true, "")
	cache.EXPECT().BuildKey(http.MethodGet, gomock.Any()).Return("miss-key").AnyTimes()
	cache.EXPECT().Get("miss-key").Return(nil, false)
	cache.EXPECT().Store(gomock.Any(), gomock.Any(), "miss-key", http.StatusOK, gomock.Any(), gomock.Any()).Return(true)
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())

	resp, err := uc.Forward(usecase.ForwardRequest{
		Context:     context.Background(),
		Method:      http.MethodGet,
		RequestPath: "/anything",
	})
	require.NoError(t, err)
	require.Equal(t, "MISS", resp.CacheStatus)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestProxyUseCase_Forward_NoCache(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	uc, store, _, _, cache, obs := newProxyUC(t, upstream.Client())

	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{}, false, "disabled")
	cache.EXPECT().BuildKey(http.MethodGet, gomock.Any()).Return("key").AnyTimes()
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())

	resp, err := uc.Forward(usecase.ForwardRequest{
		Context:     context.Background(),
		Method:      http.MethodGet,
		RequestPath: "/data",
	})
	require.NoError(t, err)
	require.Equal(t, "BYPASS", resp.CacheStatus)
}

func TestProxyUseCase_Forward_MutationInvalidates(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	uc, store, _, _, cache, obs := newProxyUC(t, upstream.Client())

	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{}, false, "method_not_cacheable")
	cache.EXPECT().BuildKey(http.MethodPost, gomock.Any()).Return("key").AnyTimes()
	cache.EXPECT().InvalidateForMutation(gomock.Any()).Return(0)
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())

	resp, err := uc.Forward(usecase.ForwardRequest{
		Context:     context.Background(),
		Method:      http.MethodPost,
		RequestPath: "/resource",
		Body:        []byte(`{"name":"test"}`),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestProxyUseCase_Forward_UpstreamError(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	uc, store, _, _, cache, obs := newProxyUC(t, dead.Client())

	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = dead.URL
	cfg.Proxy.Timeout = "1s"
	store.EXPECT().Current().Return(cfg).AnyTimes()

	cache.EXPECT().ShouldUse(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.CacheRule{}, false, "")
	cache.EXPECT().BuildKey(gomock.Any(), gomock.Any()).Return("key").AnyTimes()
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())

	_, err := uc.Forward(usecase.ForwardRequest{
		Context:     context.Background(),
		Method:      http.MethodGet,
		RequestPath: "/fail",
	})
	require.Error(t, err)
}

func TestProxyUseCase_CheckUpstream_Healthy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	uc, store, _, _, _, obs := newProxyUC(t, upstream.Client())
	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())
	uc.CheckUpstream()
}

func TestProxyUseCase_CheckUpstream_Unhealthy(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	uc, store, _, _, _, obs := newProxyUC(t, dead.Client())
	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = dead.URL
	cfg.Proxy.Timeout = "1s"
	store.EXPECT().Current().Return(cfg).AnyTimes()
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())
	uc.CheckUpstream()
}

func TestProxyUseCase_CheckUpstream_ServerError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	uc, store, _, _, _, obs := newProxyUC(t, upstream.Client())
	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = upstream.URL
	cfg.Proxy.Timeout = "5s"
	store.EXPECT().Current().Return(cfg).AnyTimes()
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())
	uc.CheckUpstream()
}

func TestProxyUseCase_CheckUpstream_BadURL(t *testing.T) {
	uc, store, _, _, _, obs := newProxyUC(t, nil)
	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = "://bad"
	store.EXPECT().Current().Return(cfg).AnyTimes()
	obs.EXPECT().UpdateUpstreamStatus(gomock.Any())
	uc.CheckUpstream()
}

func TestProxyUseCase_Forward_BadURL(t *testing.T) {
	uc, store, _, _, _, _ := newProxyUC(t, nil)

	cfg := config.Config{}
	cfg.Proxy.UpstreamBaseURL = "http://[::invalid"
	store.EXPECT().Current().Return(cfg)

	_, err := uc.Forward(usecase.ForwardRequest{
		Context:     context.Background(),
		Method:      http.MethodGet,
		RequestPath: "/path",
	})
	require.Error(t, err)
}
