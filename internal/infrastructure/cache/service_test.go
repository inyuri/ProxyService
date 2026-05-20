package cache

import (
	"ProxyService2/internal/domain"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCacheServiceStoreGetAndInvalidate(t *testing.T) {
	service := NewCacheService()
	service.UpdateSettings(domain.CacheSettings{
		Enabled:          true,
		Capacity:         8,
		MemoryLimitBytes: 1024 * 1024,
		DefaultRule: domain.CacheRule{
			Name:             "default",
			TTL2xx:           time.Minute,
			TTL3xx:           time.Minute,
			TTL4xx:           time.Minute,
			TTL5xx:           time.Minute,
			MinSizeBytes:     1,
			MaxSizeBytes:     1024,
			SensitiveHeaders: []string{"Authorization"},
		},
	})

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/movies", nil)
	_, allowed, reason := service.ShouldUse("example.com", "/movies", req)
	require.True(t, allowed)
	require.Empty(t, reason)

	stored := service.Store("example.com", "/movies", "GET:https://example.com/movies", http.StatusOK, http.Header{}, []byte("payload"))
	require.True(t, stored)

	cached, ok := service.Get("GET:https://example.com/movies")
	require.True(t, ok)
	require.Equal(t, http.StatusOK, cached.Status)
	require.Equal(t, []byte("payload"), cached.Body)

	removed, err := service.Invalidate(domain.CacheInvalidationRequest{Prefix: "/movies"})
	require.NoError(t, err)
	require.Equal(t, 1, removed)

	_, ok = service.Get("GET:https://example.com/movies")
	require.False(t, ok)
}

func TestCacheServiceRejectsSensitiveRequests(t *testing.T) {
	service := NewCacheService()
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/private", nil)
	req.Header.Set("Authorization", "Bearer token")

	_, allowed, reason := service.ShouldUse("example.com", "/private", req)
	require.False(t, allowed)
	require.Equal(t, "sensitive_request", reason)
}
