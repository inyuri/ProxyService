package domain

import "time"

type ObservabilitySettings struct {
	MaxLogs    int
	MaxBuckets int
}

type RequestEvent struct {
	Timestamp     time.Time
	IP            string
	Method        string
	Path          string
	Status        int
	Latency       time.Duration
	Blocked       bool
	Reason        string
	RequestBytes  int64
	ResponseBytes int64
	CacheStatus   string
}

type RequestLog struct {
	ID          int    `json:"id"`
	Timestamp   string `json:"timestamp"`
	IP          string `json:"ip"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Status      int    `json:"status"`
	Latency     int64  `json:"latency"`
	Blocked     bool   `json:"blocked"`
	Reason      string `json:"reason,omitempty"`
	CacheStatus string `json:"cacheStatus,omitempty"`
}

type DashboardOverview struct {
	RPS               int64            `json:"rps"`
	ActiveConnections int64            `json:"activeConnections"`
	Traffic           string           `json:"traffic"`
	Errors            int64            `json:"errors"`
	RPSData           []RPSPoint       `json:"rpsData"`
	LatencyData       []LatencyPoint   `json:"latencyData"`
	Upstreams         []UpstreamStatus `json:"upstreams"`
}

type RPSPoint struct {
	Time  string `json:"time"`
	Value int64  `json:"value"`
}

type LatencyPoint struct {
	Time string `json:"time"`
	P50  int64  `json:"p50"`
	P95  int64  `json:"p95"`
	P99  int64  `json:"p99"`
}

type TrafficPoint struct {
	Time      string `json:"time"`
	Requests  int64  `json:"requests"`
	Bandwidth int64  `json:"bandwidth"`
}

type ErrorPoint struct {
	Time string `json:"time"`
	Four int64  `json:"4xx"`
	Five int64  `json:"5xx"`
}

type TopClient struct {
	IP           string  `json:"ip"`
	Requests     int64   `json:"requests"`
	Percentage   float64 `json:"percentage"`
	Bandwidth    string  `json:"bandwidth"`
	BandwidthRaw int64   `json:"bandwidthBytes"`
}

type MonitoringSnapshot struct {
	TrafficData []TrafficPoint   `json:"trafficData"`
	ErrorData   []ErrorPoint     `json:"errorData"`
	TopClients  []TopClient      `json:"topClients"`
	Upstreams   []UpstreamStatus `json:"upstreams"`
}

type UpstreamStatus struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Healthy   bool      `json:"healthy"`
	LatencyMs int64     `json:"latencyMs"`
	Errors    int64     `json:"errors"`
	CheckedAt time.Time `json:"checkedAt"`
}
