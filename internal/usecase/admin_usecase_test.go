package usecase_test

import (
	"errors"
	"testing"
	"time"

	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/mocks"
	"ProxyService2/internal/usecase"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newAdminUC(t *testing.T) (
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

func TestAdminUseCase_DashboardOverview(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUC(t)
	want := domain.DashboardOverview{RPS: 42, ActiveConnections: 5}
	obs.EXPECT().Overview().Return(want)
	require.Equal(t, want, uc.DashboardOverview())
}

func TestAdminUseCase_ListAccessRules_NoFilter(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUC(t)
	now := time.Now()
	access.EXPECT().List().Return([]domain.AccessRule{
		{ID: "1", Type: domain.AccessRuleAllow, Value: "10.0.0.1", CreatedAt: now},
		{ID: "2", Type: domain.AccessRuleDeny, Value: "1.2.3.4", CreatedAt: now},
	})
	require.Len(t, uc.ListAccessRules(""), 2)
}

func TestAdminUseCase_ListAccessRules_WithFilter(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUC(t)
	now := time.Now()
	access.EXPECT().List().Return([]domain.AccessRule{
		{ID: "1", Type: domain.AccessRuleAllow, Value: "10.0.0.1", CreatedAt: now},
		{ID: "2", Type: domain.AccessRuleDeny, Value: "1.2.3.4", CreatedAt: now},
	})
	views := uc.ListAccessRules("deny")
	require.Len(t, views, 1)
	require.Equal(t, "1.2.3.4", views[0].IP)
}

func TestAdminUseCase_CreateAccessRule_Valid(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		return fn(&config.Config{})
	})
	view, err := uc.CreateAccessRule(usecase.AccessRuleRequest{Value: "192.168.1.1", Type: "allow"})
	require.NoError(t, err)
	require.Equal(t, "192.168.1.1", view.IP)
	require.Equal(t, "allow", view.Type)
	require.NotEmpty(t, view.ID)
}

func TestAdminUseCase_CreateAccessRule_CIDR(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		return fn(&config.Config{})
	})
	view, err := uc.CreateAccessRule(usecase.AccessRuleRequest{Value: "10.0.0.0/8", Type: "deny"})
	require.NoError(t, err)
	require.Equal(t, "deny", view.Type)
}

func TestAdminUseCase_CreateAccessRule_Range(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		return fn(&config.Config{})
	})
	view, err := uc.CreateAccessRule(usecase.AccessRuleRequest{Value: "10.0.0.1-10.0.0.50", Type: "grey"})
	require.NoError(t, err)
	require.Equal(t, "grey", view.Type)
}

func TestAdminUseCase_CreateAccessRule_InvalidIP(t *testing.T) {
	uc, _, _, _, _, _ := newAdminUC(t)
	_, err := uc.CreateAccessRule(usecase.AccessRuleRequest{Value: "not-an-ip", Type: "allow"})
	require.Error(t, err)
}

func TestAdminUseCase_CreateAccessRule_StoreError(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).Return(errors.New("disk full"))
	_, err := uc.CreateAccessRule(usecase.AccessRuleRequest{Value: "10.0.0.1", Type: "allow"})
	require.Error(t, err)
}

func TestAdminUseCase_DeleteAccessRule_Found(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		cfg := &config.Config{}
		cfg.Access.Rules = []config.AccessRuleConfig{{ID: "abc", Value: "1.1.1.1", Type: "allow"}}
		return fn(cfg)
	})
	removed, err := uc.DeleteAccessRule("abc")
	require.NoError(t, err)
	require.True(t, removed)
}

func TestAdminUseCase_DeleteAccessRule_NotFound(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		return fn(&config.Config{})
	})
	removed, err := uc.DeleteAccessRule("unknown")
	require.NoError(t, err)
	require.False(t, removed)
}

func TestAdminUseCase_DeleteAccessRule_EmptyID(t *testing.T) {
	uc, _, _, _, _, _ := newAdminUC(t)
	_, err := uc.DeleteAccessRule("")
	require.Error(t, err)
}

func TestAdminUseCase_CheckAccess_WithIP(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUC(t)
	want := domain.AccessDecision{Allowed: true, Reason: "whitelist"}
	access.EXPECT().Check("1.2.3.4", "").Return(want)
	require.Equal(t, want, uc.CheckAccess("1.2.3.4", "", "10.0.0.1"))
}

func TestAdminUseCase_CheckAccess_FallbackIP(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUC(t)
	want := domain.AccessDecision{Allowed: false, Reason: "blacklist"}
	access.EXPECT().Check("10.0.0.1", "").Return(want)
	require.Equal(t, want, uc.CheckAccess("", "", "10.0.0.1"))
}

func TestAdminUseCase_CheckAccess_WithCaptcha(t *testing.T) {
	uc, _, access, _, _, _ := newAdminUC(t)
	want := domain.AccessDecision{Allowed: true, Reason: "captcha_verified"}
	access.EXPECT().Check("192.168.1.50", "secret").Return(want)
	require.Equal(t, want, uc.CheckAccess("192.168.1.50", "secret", ""))
}

func TestAdminUseCase_GetRateLimits(t *testing.T) {
	uc, _, _, rate, _, obs := newAdminUC(t)
	settings := domain.RateLimitSettings{RPS: 100, RPM: 5000}
	rate.EXPECT().Settings().Return(settings)
	obs.EXPECT().TopClients(10).Return(nil)
	rate.EXPECT().TopViolators(50).Return(nil)
	resp := uc.GetRateLimits()
	require.Equal(t, settings, resp.Limits)
}

func TestAdminUseCase_UpdateRateLimits_OK(t *testing.T) {
	uc, store, _, rate, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).DoAndReturn(func(fn func(*config.Config) error) error {
		return fn(&config.Config{})
	})
	updated := domain.RateLimitSettings{RPS: 200}
	rate.EXPECT().Settings().Return(updated)
	got, err := uc.UpdateRateLimits(usecase.RateLimitUpdateRequest{
		RPS: 200, RPM: 5000, RPH: 100000, RPD: 1000000,
		ConcurrentConnections: 50, ConnectionsPerSecond: 10,
		UploadBytesPerSecond: 1024, DownloadBytesPerSecond: 2048, TotalBytesPerDay: 100000,
	})
	require.NoError(t, err)
	require.Equal(t, updated, got)
}

func TestAdminUseCase_UpdateRateLimits_StoreError(t *testing.T) {
	uc, store, _, _, _, _ := newAdminUC(t)
	store.EXPECT().Update(gomock.Any()).Return(errors.New("write error"))
	_, err := uc.UpdateRateLimits(usecase.RateLimitUpdateRequest{RPS: 200})
	require.Error(t, err)
}

func TestAdminUseCase_RateLimitViolations(t *testing.T) {
	uc, _, _, rate, _, _ := newAdminUC(t)
	violations := []domain.RateLimitViolation{{IP: "bad-guy", Limit: "RPS"}}
	rate.EXPECT().TopViolators(10).Return(violations)
	require.Equal(t, violations, uc.RateLimitViolations(10))
}

func TestAdminUseCase_CacheSnapshot(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUC(t)
	want := domain.CacheSnapshot{Stats: domain.CacheStats{Hits: 10, Misses: 2}}
	cache.EXPECT().Snapshot(100).Return(want)
	require.Equal(t, want, uc.CacheSnapshot(100))
}

func TestAdminUseCase_InvalidateCache_OK(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUC(t)
	req := domain.CacheInvalidationRequest{Key: "GET:http://example.com/api"}
	cache.EXPECT().Invalidate(req).Return(1, nil)
	removed, err := uc.InvalidateCache(req)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
}

func TestAdminUseCase_InvalidateCache_Error(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUC(t)
	req := domain.CacheInvalidationRequest{Regex: "[invalid"}
	cache.EXPECT().Invalidate(req).Return(0, errors.New("bad regex"))
	_, err := uc.InvalidateCache(req)
	require.Error(t, err)
}

func TestAdminUseCase_ClearCache(t *testing.T) {
	uc, _, _, _, cache, _ := newAdminUC(t)
	cache.EXPECT().InvalidateAll().Return(7)
	require.Equal(t, 7, uc.ClearCache())
}

func TestAdminUseCase_Monitoring(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUC(t)
	want := domain.MonitoringSnapshot{}
	obs.EXPECT().Monitoring(10).Return(want)
	require.Equal(t, want, uc.Monitoring(10))
}

func TestAdminUseCase_Logs_WithBlocked(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUC(t)
	items := []domain.RequestLog{
		{IP: "1.2.3.4", Status: 403, Blocked: true},
		{IP: "1.2.3.4", Status: 200},
	}
	obs.EXPECT().Logs("1.2.3.4", 0, 200).Return(items)
	resp := uc.Logs("1.2.3.4", 0, 200)
	require.Len(t, resp.Items, 2)
	require.Equal(t, 1, resp.BlockedCount)
}

func TestAdminUseCase_Logs_NoBlocked(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUC(t)
	obs.EXPECT().Logs("", 200, 50).Return([]domain.RequestLog{{IP: "5.6.7.8", Status: 200}})
	resp := uc.Logs("", 200, 50)
	require.Equal(t, 0, resp.BlockedCount)
}

func TestAdminUseCase_ExportLogs(t *testing.T) {
	uc, _, _, _, _, obs := newAdminUC(t)
	obs.EXPECT().Logs("", 0, 500).Return([]domain.RequestLog{{IP: "1.2.3.4"}})
	raw, err := uc.ExportLogs("", 0, 500)
	require.NoError(t, err)
	require.Contains(t, string(raw), "1.2.3.4")
}
