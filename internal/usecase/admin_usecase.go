package usecase

import (
	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/ports"
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
	Limits     domain.RateLimitSettings    `json:"limits"`
	TopClients []domain.TopClient          `json:"topClients"`
	Violations []domain.RateLimitViolation `json:"violations"`
}

type LogsResponse struct {
	Items        []domain.RequestLog `json:"items"`
	BlockedCount int                 `json:"blockedCount"`
}

type AdminUseCase struct {
	store   ports.ConfigStore
	access  ports.AccessService
	rate    ports.RateLimiter
	cache   ports.CacheService
	metrics ports.ObservabilityService
}

func NewAdminUseCase(
	store ports.ConfigStore,
	access ports.AccessService,
	rate ports.RateLimiter,
	cache ports.CacheService,
	metrics ports.ObservabilityService,
) *AdminUseCase {
	return &AdminUseCase{
		store:   store,
		access:  access,
		rate:    rate,
		cache:   cache,
		metrics: metrics,
	}
}

func (uc *AdminUseCase) DashboardOverview() domain.DashboardOverview {
	return uc.metrics.Overview()
}

func (uc *AdminUseCase) ListAccessRules(filterType string) []AccessRuleView {
	filterType = strings.ToLower(strings.TrimSpace(filterType))
	rules := uc.access.List()
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

func (uc *AdminUseCase) CreateAccessRule(request AccessRuleRequest) (AccessRuleView, error) {
	rule := domain.AccessRule{
		ID:          uuid.NewString(),
		Type:        normalizeRuleType(request.Type),
		Value:       strings.TrimSpace(request.Value),
		Description: request.Description,
		CreatedAt:   time.Now().UTC(),
	}
	if err := domain.ValidateAccessRule(rule); err != nil {
		return AccessRuleView{}, err
	}

	if err := uc.store.Update(func(cfg *config.Config) error {
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

func (uc *AdminUseCase) DeleteAccessRule(ruleID string) (bool, error) {
	if ruleID == "" {
		return false, errors.New("id is required")
	}

	removed := false
	err := uc.store.Update(func(cfg *config.Config) error {
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

func (uc *AdminUseCase) CheckAccess(ip, captcha, fallbackIP string) domain.AccessDecision {
	if strings.TrimSpace(ip) == "" {
		ip = fallbackIP
	}
	return uc.access.Check(ip, captcha)
}

func (uc *AdminUseCase) GetRateLimits() RateLimitResponse {
	return RateLimitResponse{
		Limits:     uc.rate.Settings(),
		TopClients: uc.metrics.TopClients(10),
		Violations: uc.rate.TopViolators(50),
	}
}

func (uc *AdminUseCase) UpdateRateLimits(request RateLimitUpdateRequest) (domain.RateLimitSettings, error) {
	if err := uc.store.Update(func(cfg *config.Config) error {
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
		return domain.RateLimitSettings{}, err
	}

	return uc.rate.Settings(), nil
}

func (uc *AdminUseCase) RateLimitViolations(limit int) []domain.RateLimitViolation {
	return uc.rate.TopViolators(limit)
}

func (uc *AdminUseCase) CacheSnapshot(limit int) domain.CacheSnapshot {
	return uc.cache.Snapshot(limit)
}

func (uc *AdminUseCase) InvalidateCache(request domain.CacheInvalidationRequest) (int, error) {
	return uc.cache.Invalidate(request)
}

func (uc *AdminUseCase) ClearCache() int {
	return uc.cache.InvalidateAll()
}

func (uc *AdminUseCase) Monitoring(topN int) domain.MonitoringSnapshot {
	return uc.metrics.Monitoring(topN)
}

func (uc *AdminUseCase) Logs(ip string, status int, limit int) LogsResponse {
	items := uc.metrics.Logs(ip, status, limit)
	blocked := 0
	for _, item := range items {
		if item.Blocked {
			blocked++
		}
	}
	return LogsResponse{Items: items, BlockedCount: blocked}
}

func (uc *AdminUseCase) ExportLogs(ip string, status int, limit int) ([]byte, error) {
	return json.MarshalIndent(uc.metrics.Logs(ip, status, limit), "", "  ")
}

func normalizeRuleType(value string) domain.AccessRuleType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "blacklist", "deny", "denied":
		return domain.AccessRuleDeny
	case "grey", "gray", "greylist", "graylist":
		return domain.AccessRuleGrey
	default:
		return domain.AccessRuleAllow
	}
}
