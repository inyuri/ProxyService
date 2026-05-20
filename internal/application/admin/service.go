package admin

import (
	"ProxyService2/internal/access"
	"ProxyService2/internal/cache"
	"ProxyService2/internal/config"
	"ProxyService2/internal/observability"
	"ProxyService2/internal/ports"
	"ProxyService2/internal/rate_limit"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AccessRuleRequest struct {
	Value       string
	Type        string
	Description string
}

type AccessRuleView struct {
	ID          string `json:"id"`
	IP          string `json:"ip"`
	Type        string `json:"type"`
	AddedDate   string `json:"addedDate"`
	Description string `json:"description"`
}

type RateLimitUpdateRequest struct {
	RPS                    int   `json:"rps"`
	RPM                    int   `json:"rpm"`
	RPH                    int   `json:"rph"`
	RPD                    int   `json:"rpd"`
	ConcurrentConnections  int   `json:"concurrentConnections"`
	ConnectionsPerSecond   int   `json:"connectionsPerSecond"`
	UploadBytesPerSecond   int64 `json:"uploadBytesPerSecond"`
	DownloadBytesPerSecond int64 `json:"downloadBytesPerSecond"`
	TotalBytesPerDay       int64 `json:"totalBytesPerDay"`
}

type RateLimitResponse struct {
	Limits     rate_limit.RateLimitSettings    `json:"limits"`
	TopClients []observability.TopClient       `json:"topClients"`
	Violations []rate_limit.RateLimitViolation `json:"violations"`
}

type LogsResponse struct {
	Items        []observability.RequestLog `json:"items"`
	BlockedCount int                        `json:"blockedCount"`
}

type Service struct {
	store   ports.ConfigStore
	access  ports.AccessService
	rate    ports.RateLimiter
	cache   ports.CacheService
	metrics ports.ObservabilityService
}

func NewService(
	store ports.ConfigStore,
	access ports.AccessService,
	rate ports.RateLimiter,
	cache ports.CacheService,
	metrics ports.ObservabilityService,
) *Service {
	return &Service{
		store:   store,
		access:  access,
		rate:    rate,
		cache:   cache,
		metrics: metrics,
	}
}

func (s *Service) DashboardOverview() observability.DashboardOverview {
	return s.metrics.Overview()
}

func (s *Service) ListAccessRules(filterType string) []AccessRuleView {
	filterType = strings.ToLower(strings.TrimSpace(filterType))
	rules := s.access.List()
	views := make([]AccessRuleView, 0, len(rules))
	for _, rule := range rules {
		if filterType != "" && filterType != string(rule.Type) {
			continue
		}
		views = append(views, AccessRuleView{
			ID:          rule.ID,
			IP:          rule.Value,
			Type:        string(rule.Type),
			AddedDate:   rule.CreatedAt.Format("2006-01-02"),
			Description: rule.Description,
		})
	}
	return views
}

func (s *Service) CreateAccessRule(request AccessRuleRequest) (AccessRuleView, error) {
	rule := access.AccessRule{
		ID:          uuid.NewString(),
		Type:        normalizeRuleType(request.Type),
		Value:       strings.TrimSpace(request.Value),
		Description: request.Description,
		CreatedAt:   time.Now().UTC(),
	}
	if err := access.ValidateAccessRule(rule); err != nil {
		return AccessRuleView{}, err
	}

	if err := s.store.Update(func(cfg *config.Config) error {
		cfg.Access.Rules = append(cfg.Access.Rules, config.AccessRuleConfig{
			ID:          rule.ID,
			Type:        string(rule.Type),
			Value:       rule.Value,
			Description: rule.Description,
			CreatedAt:   rule.CreatedAt,
		})
		return nil
	}); err != nil {
		return AccessRuleView{}, err
	}

	return AccessRuleView{
		ID:          rule.ID,
		IP:          rule.Value,
		Type:        string(rule.Type),
		AddedDate:   rule.CreatedAt.Format("2006-01-02"),
		Description: rule.Description,
	}, nil
}

func (s *Service) DeleteAccessRule(ruleID string) (bool, error) {
	if ruleID == "" {
		return false, errors.New("id is required")
	}

	removed := false
	err := s.store.Update(func(cfg *config.Config) error {
		filtered := make([]config.AccessRuleConfig, 0, len(cfg.Access.Rules))
		for _, rule := range cfg.Access.Rules {
			if rule.ID == ruleID {
				removed = true
				continue
			}
			filtered = append(filtered, rule)
		}
		cfg.Access.Rules = filtered
		return nil
	})
	return removed, err
}

func (s *Service) CheckAccess(ip, captcha, fallbackIP string) access.AccessDecision {
	if strings.TrimSpace(ip) == "" {
		ip = fallbackIP
	}
	return s.access.Check(ip, captcha)
}

func (s *Service) GetRateLimits() RateLimitResponse {
	return RateLimitResponse{
		Limits:     s.rate.Settings(),
		TopClients: s.metrics.TopClients(10),
		Violations: s.rate.TopViolators(50),
	}
}

func (s *Service) UpdateRateLimits(request RateLimitUpdateRequest) (rate_limit.RateLimitSettings, error) {
	if err := s.store.Update(func(cfg *config.Config) error {
		cfg.RateLimit.Limits.RPS = request.RPS
		cfg.RateLimit.Limits.RPM = request.RPM
		cfg.RateLimit.Limits.RPH = request.RPH
		cfg.RateLimit.Limits.RPD = request.RPD
		cfg.RateLimit.Limits.ConcurrentConnections = request.ConcurrentConnections
		cfg.RateLimit.Limits.ConnectionsPerSecond = request.ConnectionsPerSecond
		cfg.RateLimit.Limits.UploadBytesPerSecond = request.UploadBytesPerSecond
		cfg.RateLimit.Limits.DownloadBytesPerSecond = request.DownloadBytesPerSecond
		cfg.RateLimit.Limits.TotalBytesPerDay = request.TotalBytesPerDay
		return nil
	}); err != nil {
		return rate_limit.RateLimitSettings{}, err
	}

	return s.rate.Settings(), nil
}

func (s *Service) RateLimitViolations(limit int) []rate_limit.RateLimitViolation {
	return s.rate.TopViolators(limit)
}

func (s *Service) CacheSnapshot(limit int) cache.CacheSnapshot {
	return s.cache.Snapshot(limit)
}

func (s *Service) InvalidateCache(request cache.CacheInvalidationRequest) (int, error) {
	return s.cache.Invalidate(request)
}

func (s *Service) ClearCache() int {
	return s.cache.InvalidateAll()
}

func (s *Service) Monitoring(topN int) observability.MonitoringSnapshot {
	return s.metrics.Monitoring(topN)
}

func (s *Service) Logs(ip string, status int, limit int) LogsResponse {
	items := s.metrics.Logs(ip, status, limit)
	blocked := 0
	for _, item := range items {
		if item.Blocked {
			blocked++
		}
	}
	return LogsResponse{Items: items, BlockedCount: blocked}
}

func (s *Service) ExportLogs(ip string, status int, limit int) ([]byte, error) {
	return json.MarshalIndent(s.metrics.Logs(ip, status, limit), "", "  ")
}

func normalizeRuleType(value string) access.AccessRuleType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "blacklist", "deny", "denied":
		return access.AccessRuleDeny
	case "grey", "gray", "greylist", "graylist":
		return access.AccessRuleGrey
	default:
		return access.AccessRuleAllow
	}
}
