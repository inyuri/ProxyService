package ratelimit

import (
	"ProxyService2/internal/domain"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimiterRejectsRPSViolations(t *testing.T) {
	limiter := NewRateLimiter()
	limiter.UpdateSettings(domain.RateLimitSettings{
		RPS:                   1,
		RPM:                   10,
		RPH:                   10,
		RPD:                   10,
		ConcurrentConnections: 10,
		ConnectionsPerSecond:  10,
		SubnetIPv4Mask:        24,
		SubnetIPv6Mask:        64,
	})

	lease, violation := limiter.Acquire("198.51.100.10", 0, time.Now())
	require.NotNil(t, lease)
	require.Nil(t, violation)

	leaseTwo, violation := limiter.Acquire("198.51.100.10", 0, time.Now())
	require.Nil(t, leaseTwo)
	require.NotNil(t, violation)
	require.Equal(t, "RPS", violation.Limit)
}

func TestRateLimiterAppliesSubnetLimits(t *testing.T) {
	limiter := NewRateLimiter()
	limiter.UpdateSettings(domain.RateLimitSettings{
		RPS:                   10,
		RPM:                   10,
		RPH:                   10,
		RPD:                   10,
		ConcurrentConnections: 10,
		ConnectionsPerSecond:  1,
		SubnetIPv4Mask:        24,
		SubnetIPv6Mask:        64,
	})

	lease, violation := limiter.Acquire("10.0.0.5", 0, time.Now())
	require.NotNil(t, lease)
	require.Nil(t, violation)

	leaseTwo, violation := limiter.Acquire("10.0.0.77", 0, time.Now())
	require.Nil(t, leaseTwo)
	require.NotNil(t, violation)
	require.Equal(t, "subnet", violation.Scope)
	require.Equal(t, "CONNECTIONS_PER_SECOND", violation.Limit)
}
