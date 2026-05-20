package ports

import (
	"ProxyService2/internal/access"
	"ProxyService2/internal/cache"
	"ProxyService2/internal/config"
	"ProxyService2/internal/observability"
	"ProxyService2/internal/rate_limit"
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type ConfigStore interface {
	Current() config.Config
	Subscribe(func(config.Config))
	Watch(context.Context) error
	Update(func(*config.Config) error) error
}

type Logger interface {
	Log(level zerolog.Level, msg string, fields map[string]any)
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}

type AccessService interface {
	ApplyConfig(access.AccessSettings) error
	List() []access.AccessRule
	Settings() access.AccessSettings
	Check(ipValue, captchaValue string) access.AccessDecision
}

type RateLimiter interface {
	UpdateSettings(rate_limit.RateLimitSettings)
	Settings() rate_limit.RateLimitSettings
	Acquire(ip string, uploadHint int64, now time.Time) (*rate_limit.RateLimitLease, *rate_limit.RateLimitViolation)
	Release(lease *rate_limit.RateLimitLease, uploadedBytes, downloadedBytes int64)
	TopViolators(limit int) []rate_limit.RateLimitViolation
}

type CacheService interface {
	UpdateSettings(cache.CacheSettings)
	BuildKey(method, url string) string
	Get(key string) (*cache.CachedResponse, bool)
	ShouldUse(host, path string, req *http.Request) (cache.CacheRule, bool, string)
	Store(host, path, key string, status int, header http.Header, body []byte) bool
	Invalidate(req cache.CacheInvalidationRequest) (int, error)
	InvalidateAll() int
	InvalidateForMutation(path string) int
	Snapshot(limit int) cache.CacheSnapshot
}

type ObservabilityService interface {
	UpdateSettings(observability.ObservabilitySettings)
	IncActive()
	DecActive()
	Record(observability.RequestEvent)
	Logs(ip string, status int, limit int) []observability.RequestLog
	Overview() observability.DashboardOverview
	Monitoring(topN int) observability.MonitoringSnapshot
	TopClients(limit int) []observability.TopClient
	UpdateUpstreamStatus(observability.UpstreamStatus)
	UpstreamStatuses() []observability.UpstreamStatus
}
