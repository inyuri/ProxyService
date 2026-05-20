package runtimeconfig

import (
	"ProxyService2/internal/access"
	"ProxyService2/internal/cache"
	"ProxyService2/internal/config"
	"ProxyService2/internal/observability"
	"ProxyService2/internal/rate_limit"
	"strings"
	"time"
)

func AccessSettingsFromConfig(cfg config.Config) access.AccessSettings {
	rules := make([]access.AccessRule, 0, len(cfg.Access.Rules))
	for _, rule := range cfg.Access.Rules {
		rules = append(rules, access.AccessRule{
			ID:          rule.ID,
			Type:        access.AccessRuleType(strings.ToLower(rule.Type)),
			Value:       rule.Value,
			Description: rule.Description,
			CreatedAt:   rule.CreatedAt,
		})
	}
	return access.AccessSettings{
		DefaultPolicy: cfg.Access.DefaultPolicy,
		CaptchaHeader: cfg.Access.CaptchaHeader,
		CaptchaToken:  cfg.Access.CaptchaToken,
		CacheTTL:      config.ParseDurationOrDefault(cfg.Access.DecisionCacheTTL, 2*time.Minute),
		CacheSize:     cfg.Access.DecisionCacheSize,
		Rules:         rules,
	}
}

func RateLimitSettingsFromConfig(cfg config.Config) rate_limit.RateLimitSettings {
	return rate_limit.RateLimitSettings{
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

func CacheSettingsFromConfig(cfg config.Config) cache.CacheSettings {
	return cache.CacheSettings{
		Enabled:          cfg.Cache.Enabled,
		Capacity:         cfg.Cache.Capacity,
		MemoryLimitBytes: cfg.Cache.MemoryLimitBytes,
		DefaultRule:      CacheRuleFromConfig(cfg.Cache.DefaultRule),
		Rules:            MapCacheRules(cfg.Cache.Rules),
	}
}

func CacheRuleFromConfig(rule config.CacheRuleConfig) cache.CacheRule {
	return cache.CacheRule{
		Name:             rule.Name,
		Domains:          append([]string{}, rule.Domains...),
		PathPrefixes:     append([]string{}, rule.PathPrefixes...),
		TTL2xx:           config.ParseDurationOrDefault(rule.TTL2xx, 5*time.Minute),
		TTL3xx:           config.ParseDurationOrDefault(rule.TTL3xx, time.Minute),
		TTL4xx:           config.ParseDurationOrDefault(rule.TTL4xx, 30*time.Second),
		TTL5xx:           config.ParseDurationOrDefault(rule.TTL5xx, 15*time.Second),
		MinSizeBytes:     rule.MinSizeBytes,
		MaxSizeBytes:     rule.MaxSizeBytes,
		Tags:             append([]string{}, rule.Tags...),
		SensitiveHeaders: append([]string{}, rule.SensitiveHeaders...),
	}
}

func MapCacheRules(rules []config.CacheRuleConfig) []cache.CacheRule {
	result := make([]cache.CacheRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, CacheRuleFromConfig(rule))
	}
	return result
}

func ObservabilitySettingsFromConfig(cfg config.Config) observability.ObservabilitySettings {
	return observability.ObservabilitySettings{
		MaxLogs:    cfg.Monitoring.LogBufferSize,
		MaxBuckets: cfg.Monitoring.HistoryBuckets,
	}
}
