package config

import (
	"ProxyService2/internal/domain"
	"strings"
	"time"
)

func AccessSettingsFromConfig(cfg Config) domain.AccessSettings {
	rules := make([]domain.AccessRule, 0, len(cfg.Access.Rules))
	for _, rule := range cfg.Access.Rules {
		rules = append(rules, domain.AccessRule{
			ID:          rule.ID,
			Type:        domain.AccessRuleType(strings.ToLower(rule.Type)),
			Value:       rule.Value,
			Description: rule.Description,
			CreatedAt:   rule.CreatedAt,
		})
	}
	return domain.AccessSettings{
		DefaultPolicy: cfg.Access.DefaultPolicy,
		CaptchaHeader: cfg.Access.CaptchaHeader,
		CaptchaToken:  cfg.Access.CaptchaToken,
		CacheTTL:      ParseDurationOrDefault(cfg.Access.DecisionCacheTTL, 2*time.Minute),
		CacheSize:     cfg.Access.DecisionCacheSize,
		Rules:         rules,
	}
}

func RateLimitSettingsFromConfig(cfg Config) domain.RateLimitSettings {
	return domain.RateLimitSettings{
		RPS:                    cfg.RateLimit.Limits.RPS,
		RPM:                    cfg.RateLimit.Limits.RPM,
		RPH:                    cfg.RateLimit.Limits.RPH,
		RPD:                    cfg.RateLimit.Limits.RPD,
		ConcurrentConnections:  cfg.RateLimit.Limits.ConcurrentConnections,
		ConnectionsPerSecond:   cfg.RateLimit.Limits.ConnectionsPerSecond,
		UploadBytesPerSecond:   cfg.RateLimit.Limits.UploadBytesPerSecond,
		DownloadBytesPerSecond: cfg.RateLimit.Limits.DownloadBytesPerSecond,
		TotalBytesPerDay:       cfg.RateLimit.Limits.TotalBytesPerDay,
		SubnetIPv4Mask:         cfg.RateLimit.SubnetIPv4Mask,
		SubnetIPv6Mask:         cfg.RateLimit.SubnetIPv6Mask,
	}
}

func CacheSettingsFromConfig(cfg Config) domain.CacheSettings {
	return domain.CacheSettings{
		Enabled:          cfg.Cache.Enabled,
		Capacity:         cfg.Cache.Capacity,
		MemoryLimitBytes: cfg.Cache.MemoryLimitBytes,
		DefaultRule:      CacheRuleFromConfig(cfg.Cache.DefaultRule),
		Rules:            MapCacheRules(cfg.Cache.Rules),
	}
}

func CacheRuleFromConfig(rule CacheRuleConfig) domain.CacheRule {
	return domain.CacheRule{
		Name:             rule.Name,
		Domains:          append([]string{}, rule.Domains...),
		PathPrefixes:     append([]string{}, rule.PathPrefixes...),
		TTL2xx:           ParseDurationOrDefault(rule.TTL2xx, 5*time.Minute),
		TTL3xx:           ParseDurationOrDefault(rule.TTL3xx, time.Minute),
		TTL4xx:           ParseDurationOrDefault(rule.TTL4xx, 30*time.Second),
		TTL5xx:           ParseDurationOrDefault(rule.TTL5xx, 15*time.Second),
		MinSizeBytes:     rule.MinSizeBytes,
		MaxSizeBytes:     rule.MaxSizeBytes,
		Tags:             append([]string{}, rule.Tags...),
		SensitiveHeaders: append([]string{}, rule.SensitiveHeaders...),
	}
}

func MapCacheRules(rules []CacheRuleConfig) []domain.CacheRule {
	result := make([]domain.CacheRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, CacheRuleFromConfig(rule))
	}
	return result
}

func ObservabilitySettingsFromConfig(cfg Config) domain.ObservabilitySettings {
	return domain.ObservabilitySettings{
		MaxLogs:    cfg.Monitoring.LogBufferSize,
		MaxBuckets: cfg.Monitoring.HistoryBuckets,
	}
}
