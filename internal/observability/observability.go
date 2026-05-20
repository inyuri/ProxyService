package observability

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

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

type clientAggregate struct {
	requests int64
	bytes    int64
}

type metricsBucket struct {
	bucketTime time.Time
	requests   int64
	bandwidth  int64
	errors4xx  int64
	errors5xx  int64
	latencies  []int64
}

type Observability struct {
	settings    ObservabilitySettings
	startedAt   time.Time
	activeConns atomic.Int64
	totalErrors atomic.Int64
	totalBytes  atomic.Int64

	mu        sync.RWMutex
	logs      []RequestLog
	logSeq    int
	buckets   map[int64]*metricsBucket
	clients   map[string]*clientAggregate
	upstreams map[string]UpstreamStatus
	totalReqs int64
}

func NewObservability(settings ObservabilitySettings) *Observability {
	if settings.MaxLogs <= 0 {
		settings.MaxLogs = 500
	}
	if settings.MaxBuckets <= 0 {
		settings.MaxBuckets = 60
	}

	return &Observability{
		settings:  settings,
		startedAt: time.Now(),
		buckets:   make(map[int64]*metricsBucket),
		clients:   make(map[string]*clientAggregate),
		upstreams: make(map[string]UpstreamStatus),
	}
}

func (o *Observability) UpdateSettings(settings ObservabilitySettings) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if settings.MaxLogs > 0 {
		o.settings.MaxLogs = settings.MaxLogs
	}
	if settings.MaxBuckets > 0 {
		o.settings.MaxBuckets = settings.MaxBuckets
	}
}

func (o *Observability) IncActive() {
	o.activeConns.Add(1)
}

func (o *Observability) DecActive() {
	o.activeConns.Add(-1)
}

func (o *Observability) Record(event RequestEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.totalReqs++
	if event.Status >= 400 || event.Blocked {
		o.totalErrors.Add(1)
	}
	totalBytes := event.RequestBytes + event.ResponseBytes
	o.totalBytes.Add(totalBytes)

	o.logSeq++
	o.logs = append(o.logs, RequestLog{
		ID:          o.logSeq,
		Timestamp:   event.Timestamp.Format("2006-01-02 15:04:05"),
		IP:          event.IP,
		Method:      event.Method,
		Path:        event.Path,
		Status:      event.Status,
		Latency:     event.Latency.Milliseconds(),
		Blocked:     event.Blocked,
		Reason:      event.Reason,
		CacheStatus: event.CacheStatus,
	})
	if len(o.logs) > o.settings.MaxLogs {
		o.logs = o.logs[len(o.logs)-o.settings.MaxLogs:]
	}

	client := o.clients[event.IP]
	if client == nil {
		client = &clientAggregate{}
		o.clients[event.IP] = client
	}
	client.requests++
	client.bytes += totalBytes

	bucketTime := event.Timestamp.Truncate(time.Minute)
	key := bucketTime.Unix()
	bucket := o.buckets[key]
	if bucket == nil {
		bucket = &metricsBucket{bucketTime: bucketTime}
		o.buckets[key] = bucket
	}
	bucket.requests++
	bucket.bandwidth += totalBytes
	if event.Status >= 400 && event.Status < 500 {
		bucket.errors4xx++
	}
	if event.Status >= 500 {
		bucket.errors5xx++
	}
	bucket.latencies = append(bucket.latencies, event.Latency.Milliseconds())

	o.trimBuckets()
}

func (o *Observability) Logs(ip string, status int, limit int) []RequestLog {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if limit <= 0 {
		limit = 200
	}

	result := make([]RequestLog, 0, limit)
	for index := len(o.logs) - 1; index >= 0; index-- {
		log := o.logs[index]
		if ip != "" && !strings.Contains(log.IP, ip) {
			continue
		}
		if status > 0 && log.Status != status {
			continue
		}
		result = append(result, log)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func (o *Observability) Overview() DashboardOverview {
	return DashboardOverview{
		RPS:               o.currentRPS(),
		ActiveConnections: o.activeConns.Load(),
		Traffic:           humanBytes(o.totalBytes.Load()),
		Errors:            o.totalErrors.Load(),
		RPSData:           o.rpsHistory(),
		LatencyData:       o.latencyHistory(),
		Upstreams:         o.UpstreamStatuses(),
	}
}

func (o *Observability) Monitoring(topN int) MonitoringSnapshot {
	return MonitoringSnapshot{
		TrafficData: o.trafficHistory(),
		ErrorData:   o.errorHistory(),
		TopClients:  o.TopClients(topN),
		Upstreams:   o.UpstreamStatuses(),
	}
}

func (o *Observability) TopClients(limit int) []TopClient {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	type item struct {
		ip    string
		stats *clientAggregate
	}
	items := make([]item, 0, len(o.clients))
	for ip, stats := range o.clients {
		items = append(items, item{ip: ip, stats: stats})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].stats.requests > items[j].stats.requests
	})

	result := make([]TopClient, 0, min(limit, len(items)))
	total := o.totalReqs
	if total == 0 {
		total = 1
	}
	for _, item := range items {
		result = append(result, TopClient{
			IP:           item.ip,
			Requests:     item.stats.requests,
			Percentage:   float64(item.stats.requests) / float64(total) * 100,
			Bandwidth:    humanBytes(item.stats.bytes),
			BandwidthRaw: item.stats.bytes,
		})
		if len(result) >= limit {
			break
		}
	}
	return result
}

func (o *Observability) UpdateUpstreamStatus(status UpstreamStatus) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.upstreams[status.Name] = status
}

func (o *Observability) UpstreamStatuses() []UpstreamStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]UpstreamStatus, 0, len(o.upstreams))
	for _, status := range o.upstreams {
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (o *Observability) currentRPS() int64 {
	o.mu.RLock()
	defer o.mu.RUnlock()

	bucket := o.buckets[time.Now().Truncate(time.Minute).Unix()]
	if bucket == nil {
		return 0
	}
	return bucket.requests
}

func (o *Observability) rpsHistory() []RPSPoint {
	o.mu.RLock()
	defer o.mu.RUnlock()

	keys := o.sortedBucketKeys()
	points := make([]RPSPoint, 0, len(keys))
	for _, key := range keys {
		bucket := o.buckets[key]
		points = append(points, RPSPoint{
			Time:  bucket.bucketTime.Format("15:04"),
			Value: bucket.requests,
		})
	}
	return points
}

func (o *Observability) latencyHistory() []LatencyPoint {
	o.mu.RLock()
	defer o.mu.RUnlock()

	keys := o.sortedBucketKeys()
	points := make([]LatencyPoint, 0, len(keys))
	for _, key := range keys {
		bucket := o.buckets[key]
		p50, p95, p99 := latencyPercentiles(bucket.latencies)
		points = append(points, LatencyPoint{
			Time: bucket.bucketTime.Format("15:04"),
			P50:  p50,
			P95:  p95,
			P99:  p99,
		})
	}
	return points
}

func (o *Observability) trafficHistory() []TrafficPoint {
	o.mu.RLock()
	defer o.mu.RUnlock()

	keys := o.sortedBucketKeys()
	points := make([]TrafficPoint, 0, len(keys))
	for _, key := range keys {
		bucket := o.buckets[key]
		points = append(points, TrafficPoint{
			Time:      bucket.bucketTime.Format("15:04"),
			Requests:  bucket.requests,
			Bandwidth: bucket.bandwidth,
		})
	}
	return points
}

func (o *Observability) errorHistory() []ErrorPoint {
	o.mu.RLock()
	defer o.mu.RUnlock()

	keys := o.sortedBucketKeys()
	points := make([]ErrorPoint, 0, len(keys))
	for _, key := range keys {
		bucket := o.buckets[key]
		points = append(points, ErrorPoint{
			Time: bucket.bucketTime.Format("15:04"),
			Four: bucket.errors4xx,
			Five: bucket.errors5xx,
		})
	}
	return points
}

func (o *Observability) sortedBucketKeys() []int64 {
	keys := make([]int64, 0, len(o.buckets))
	for key := range o.buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func (o *Observability) trimBuckets() {
	if len(o.buckets) <= o.settings.MaxBuckets {
		return
	}
	keys := o.sortedBucketKeys()
	for len(keys) > o.settings.MaxBuckets {
		delete(o.buckets, keys[0])
		keys = keys[1:]
	}
}

func latencyPercentiles(latencies []int64) (int64, int64, int64) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}
	sorted := append([]int64{}, latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return percentile(sorted, 0.50), percentile(sorted, 0.95), percentile(sorted, 0.99)
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return values[index]
}

func humanBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return strconv.FormatInt(value, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(value)/float64(div), 'f', 1, 64) + " " + string("KMGTPE"[exp]) + "B"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
