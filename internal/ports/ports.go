package ports

import (
	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
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
	ApplyConfig(domain.AccessSettings) error
	List() []domain.AccessRule
	Settings() domain.AccessSettings
	Check(ipValue, captchaValue string) domain.AccessDecision
}

type RateLimiter interface {
	UpdateSettings(domain.RateLimitSettings)
	Settings() domain.RateLimitSettings
	Acquire(ip string, uploadHint int64, now time.Time) (*domain.RateLimitLease, *domain.RateLimitViolation)
	Release(lease *domain.RateLimitLease, uploadedBytes, downloadedBytes int64)
	TopViolators(limit int) []domain.RateLimitViolation
}

type CacheService interface {
	UpdateSettings(domain.CacheSettings)
	BuildKey(method, url string) string
	Get(key string) (*domain.CachedResponse, bool)
	ShouldUse(host, path string, req *http.Request) (domain.CacheRule, bool, string)
	Store(host, path, key string, status int, header http.Header, body []byte) bool
	Invalidate(req domain.CacheInvalidationRequest) (int, error)
	InvalidateAll() int
	InvalidateForMutation(path string) int
	Snapshot(limit int) domain.CacheSnapshot
}

type ObservabilityService interface {
	UpdateSettings(domain.ObservabilitySettings)
	IncActive()
	DecActive()
	Record(domain.RequestEvent)
	Logs(ip string, status int, limit int) []domain.RequestLog
	Overview() domain.DashboardOverview
	Monitoring(topN int) domain.MonitoringSnapshot
	TopClients(limit int) []domain.TopClient
	UpdateUpstreamStatus(domain.UpstreamStatus)
	UpstreamStatuses() []domain.UpstreamStatus
}
