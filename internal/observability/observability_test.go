package observability

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObservabilityAggregatesEvents(t *testing.T) {
	metrics := NewObservability(ObservabilitySettings{MaxLogs: 10, MaxBuckets: 10})
	metrics.UpdateUpstreamStatus(UpstreamStatus{Name: "httpbin.org", URL: "https://httpbin.org", Healthy: true, LatencyMs: 42})
	metrics.IncActive()
	metrics.Record(RequestEvent{
		Timestamp:     time.Now(),
		IP:            "192.168.1.10",
		Method:        "GET",
		Path:          "/proxy/movies",
		Status:        200,
		Latency:       45 * time.Millisecond,
		RequestBytes:  120,
		ResponseBytes: 512,
	})
	metrics.Record(RequestEvent{
		Timestamp:     time.Now(),
		IP:            "192.168.1.11",
		Method:        "POST",
		Path:          "/proxy/admin",
		Status:        403,
		Latency:       12 * time.Millisecond,
		Blocked:       true,
		Reason:        "blacklist",
		RequestBytes:  24,
		ResponseBytes: 64,
	})
	metrics.DecActive()

	overview := metrics.Overview()
	require.Equal(t, int64(2), overview.RPS)
	require.Equal(t, int64(0), overview.ActiveConnections)
	require.Equal(t, int64(1), overview.Errors)
	require.NotEmpty(t, overview.Upstreams)

	logs := metrics.Logs("192.168.1.11", 403, 10)
	require.Len(t, logs, 1)
	require.Equal(t, "/proxy/admin", logs[0].Path)

	topClients := metrics.TopClients(2)
	require.Len(t, topClients, 2)
	require.Equal(t, "192.168.1.10", topClients[0].IP)
}
