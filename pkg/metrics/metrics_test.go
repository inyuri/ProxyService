package metrics_test

import (
	"testing"

	"ProxyService2/pkg/metrics"

	"github.com/stretchr/testify/require"
)

func TestCollector_RecordRequest(t *testing.T) {
	c := metrics.NewCollector()
	c.RecordRequest(10, false)
	c.RecordRequest(20, true)

	snap := c.Snapshot()
	require.Equal(t, int64(2), snap.TotalRequests)
	require.Equal(t, int64(1), snap.TotalErrors)
	require.Greater(t, snap.RPS, 0.0)
	require.Greater(t, snap.AvgLatencyMs, 0.0)
}

func TestCollector_RecordTraffic(t *testing.T) {
	c := metrics.NewCollector()
	c.RecordTraffic(1024, 2048)

	snap := c.Snapshot()
	require.Equal(t, int64(1024), snap.TotalBytesIn)
	require.Equal(t, int64(2048), snap.TotalBytesOut)
}

func TestCollector_Connections(t *testing.T) {
	c := metrics.NewCollector()
	c.IncConns()
	c.IncConns()
	require.Equal(t, int64(2), c.Snapshot().ActiveConns)
	c.DecConns()
	require.Equal(t, int64(1), c.Snapshot().ActiveConns)
}

func TestCollector_Snapshot_Uptime(t *testing.T) {
	c := metrics.NewCollector()
	snap := c.Snapshot()
	require.GreaterOrEqual(t, snap.UptimeSec, int64(0))
}

func TestCollector_ZeroState(t *testing.T) {
	c := metrics.NewCollector()
	snap := c.Snapshot()
	require.Equal(t, int64(0), snap.TotalRequests)
	require.Equal(t, int64(0), snap.TotalErrors)
	require.Equal(t, float64(0), snap.RPS)
}
