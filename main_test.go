package main

import (
	"ProxyService2/internal/config"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigConversions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Access.Rules = []config.AccessRuleConfig{
		{ID: "allow-local", Type: "allow", Value: "127.0.0.1"},
	}

	accessSettings := config.AccessSettingsFromConfig(cfg)
	require.Equal(t, "allow", accessSettings.DefaultPolicy)
	require.Len(t, accessSettings.Rules, 1)
	require.Equal(t, "allow-local", accessSettings.Rules[0].ID)

	rateSettings := config.RateLimitSettingsFromConfig(cfg)
	require.Equal(t, cfg.RateLimit.Limits.RPS, rateSettings.RPS)
	require.Equal(t, cfg.RateLimit.Limits.TotalBytesPerDay, rateSettings.TotalBytesPerDay)

	cacheSettings := config.CacheSettingsFromConfig(cfg)
	require.True(t, cacheSettings.Enabled)
	require.Equal(t, cfg.Cache.Capacity, cacheSettings.Capacity)
	require.Equal(t, cfg.Cache.DefaultRule.Name, cacheSettings.DefaultRule.Name)
}
