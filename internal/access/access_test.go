package access

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccessServiceCheckPrecedenceAndCaptcha(t *testing.T) {
	service := NewAccessService()
	err := service.ApplyConfig(AccessSettings{
		DefaultPolicy: "deny",
		CaptchaToken:  "captcha-ok",
		CacheTTL:      time.Minute,
		CacheSize:     64,
		Rules: []AccessRule{
			{ID: "allow-subnet", Type: AccessRuleAllow, Value: "10.0.0.0/24", CreatedAt: time.Now()},
			{ID: "deny-ip", Type: AccessRuleDeny, Value: "10.0.0.5", CreatedAt: time.Now()},
			{ID: "grey-ip", Type: AccessRuleGrey, Value: "192.168.1.50", CreatedAt: time.Now()},
		},
	})
	require.NoError(t, err)

	decision := service.Check("10.0.0.5", "")
	require.False(t, decision.Allowed)
	require.Equal(t, "blacklist", decision.Reason)
	require.Equal(t, "deny-ip", decision.RuleID)

	decision = service.Check("10.0.0.42", "")
	require.True(t, decision.Allowed)
	require.Equal(t, "whitelist", decision.Reason)

	decision = service.Check("192.168.1.50", "")
	require.False(t, decision.Allowed)
	require.True(t, decision.RequiresCaptcha)
	require.Equal(t, "captcha_required", decision.Reason)

	decision = service.Check("192.168.1.50", "captcha-ok")
	require.True(t, decision.Allowed)
	require.Equal(t, "captcha_verified", decision.Reason)

	decision = service.Check("172.16.0.1", "")
	require.False(t, decision.Allowed)
	require.Equal(t, "default_deny", decision.Reason)
}

func TestAccessServiceSupportsIPv4Ranges(t *testing.T) {
	service := NewAccessService()
	err := service.ApplyConfig(AccessSettings{
		DefaultPolicy: "allow",
		CacheTTL:      time.Minute,
		CacheSize:     64,
		Rules: []AccessRule{
			{ID: "deny-range", Type: AccessRuleDeny, Value: "203.0.113.10-203.0.113.20", CreatedAt: time.Now()},
		},
	})
	require.NoError(t, err)

	decision := service.Check("203.0.113.15", "")
	require.False(t, decision.Allowed)
	require.Equal(t, "blacklist", decision.Reason)

	decision = service.Check("203.0.113.25", "")
	require.True(t, decision.Allowed)
	require.Equal(t, "default_allow", decision.Reason)
}
