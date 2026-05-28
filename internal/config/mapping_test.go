package config_test

import (
	"testing"
	"time"

	"ProxyService2/internal/config"

	"github.com/stretchr/testify/require"
)

func TestAccessSettingsFromConfig(t *testing.T) {
	cfg := config.Config{}
	cfg.Access.DefaultPolicy = "deny"
	cfg.Access.CaptchaHeader = "X-Captcha"
	cfg.Access.CaptchaToken = "secret"
	cfg.Access.DecisionCacheTTL = "2m"
	cfg.Access.DecisionCacheSize = 512
	cfg.Access.Rules = []config.AccessRuleConfig{
		{ID: "r1", Type: "allow", Value: "10.0.0.1", CreatedAt: time.Now()},
		{ID: "r2", Type: "DENY", Value: "1.2.3.4", CreatedAt: time.Now()},
	}

	s := config.AccessSettingsFromConfig(cfg)
	require.Equal(t, "deny", s.DefaultPolicy)
	require.Equal(t, "X-Captcha", s.CaptchaHeader)
	require.Equal(t, "secret", s.CaptchaToken)
	require.Equal(t, 512, s.CacheSize)
	require.Len(t, s.Rules, 2)
	require.Equal(t, "allow", string(s.Rules[0].Type))
	require.Equal(t, "deny", string(s.Rules[1].Type))
}

func TestAccessSettingsFromConfig_DefaultTTL(t *testing.T) {
	cfg := config.Config{}
	s := config.AccessSettingsFromConfig(cfg)
	require.Equal(t, 2*time.Minute, s.CacheTTL)
}

func TestRateLimitSettingsFromConfig(t *testing.T) {
	cfg := config.Config{}
	cfg.RateLimit.Limits.RPS = 100
	cfg.RateLimit.Limits.RPM = 5000
	cfg.RateLimit.Limits.RPH = 200000
	cfg.RateLimit.Limits.RPD = 1000000
	cfg.RateLimit.Limits.ConcurrentConnections = 50
	cfg.RateLimit.Limits.ConnectionsPerSecond = 10
	cfg.RateLimit.Limits.UploadBytesPerSecond = 1024
	cfg.RateLimit.Limits.DownloadBytesPerSecond = 2048
	cfg.RateLimit.Limits.TotalBytesPerDay = 100000
	cfg.RateLimit.SubnetIPv4Mask = 24
	cfg.RateLimit.SubnetIPv6Mask = 64

	s := config.RateLimitSettingsFromConfig(cfg)
	require.Equal(t, 100, s.RPS)
	require.Equal(t, 5000, s.RPM)
	require.Equal(t, 24, s.SubnetIPv4Mask)
	require.Equal(t, int64(2048), s.DownloadBytesPerSecond)
}

func TestCacheSettingsFromConfig(t *testing.T) {
	cfg := config.Config{}
	cfg.Cache.Enabled = true
	cfg.Cache.Capacity = 256
	cfg.Cache.MemoryLimitBytes = 64 * 1024 * 1024
	cfg.Cache.DefaultRule.Name = "default"
	cfg.Cache.DefaultRule.TTL2xx = "5m"
	cfg.Cache.DefaultRule.TTL3xx = "1m"
	cfg.Cache.DefaultRule.MinSizeBytes = 1
	cfg.Cache.DefaultRule.MaxSizeBytes = 1024 * 1024
	cfg.Cache.Rules = []config.CacheRuleConfig{
		{Name: "api", PathPrefixes: []string{"/api"}, TTL2xx: "10m"},
	}

	s := config.CacheSettingsFromConfig(cfg)
	require.True(t, s.Enabled)
	require.Equal(t, 256, s.Capacity)
	require.Equal(t, 5*time.Minute, s.DefaultRule.TTL2xx)
	require.Len(t, s.Rules, 1)
	require.Equal(t, "api", s.Rules[0].Name)
}

func TestCacheRuleFromConfig_Defaults(t *testing.T) {
	rule := config.CacheRuleFromConfig(config.CacheRuleConfig{Name: "test"})
	require.Equal(t, "test", rule.Name)
	require.Equal(t, 5*time.Minute, rule.TTL2xx)
	require.Equal(t, time.Minute, rule.TTL3xx)
	require.Equal(t, 30*time.Second, rule.TTL4xx)
	require.Equal(t, 15*time.Second, rule.TTL5xx)
}

func TestMapCacheRules_Empty(t *testing.T) {
	result := config.MapCacheRules(nil)
	require.Empty(t, result)
}

func TestMapCacheRules_Multiple(t *testing.T) {
	rules := []config.CacheRuleConfig{
		{Name: "a", TTL2xx: "1m"},
		{Name: "b", TTL2xx: "2m"},
	}
	result := config.MapCacheRules(rules)
	require.Len(t, result, 2)
	require.Equal(t, "a", result[0].Name)
}

func TestObservabilitySettingsFromConfig(t *testing.T) {
	cfg := config.Config{}
	cfg.Monitoring.LogBufferSize = 300
	cfg.Monitoring.HistoryBuckets = 60

	s := config.ObservabilitySettingsFromConfig(cfg)
	require.Equal(t, 300, s.MaxLogs)
	require.Equal(t, 60, s.MaxBuckets)
}

func TestParseDurationOrDefault(t *testing.T) {
	require.Equal(t, 5*time.Minute, config.ParseDurationOrDefault("5m", time.Second))
	require.Equal(t, 10*time.Second, config.ParseDurationOrDefault("bad", 10*time.Second))
	require.Equal(t, 30*time.Second, config.ParseDurationOrDefault("", 30*time.Second))
}
