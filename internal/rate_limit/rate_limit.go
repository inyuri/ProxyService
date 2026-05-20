package rate_limit

import (
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"
)

type RateLimitSettings struct {
	RPS                    int   `json:"rps"`
	RPM                    int   `json:"rpm"`
	RPH                    int   `json:"rph"`
	RPD                    int   `json:"rpd"`
	ConcurrentConnections  int   `json:"concurrentConnections"`
	ConnectionsPerSecond   int   `json:"connectionsPerSecond"`
	UploadBytesPerSecond   int64 `json:"uploadBytesPerSecond"`
	DownloadBytesPerSecond int64 `json:"downloadBytesPerSecond"`
	TotalBytesPerDay       int64 `json:"totalBytesPerDay"`
	SubnetIPv4Mask         int   `json:"subnetIPv4Mask"`
	SubnetIPv6Mask         int   `json:"subnetIPv6Mask"`
}

type RateLimitViolation struct {
	IP       string    `json:"ip"`
	Scope    string    `json:"scope"`
	Limit    string    `json:"limit"`
	Exceeded string    `json:"exceeded"`
	Time     time.Time `json:"time"`
	Reason   string    `json:"reason"`
}

type RateLimitLease struct {
	limiter *RateLimiter
	keys    []string
	now     time.Time
}

type RateLimiter struct {
	mu             sync.Mutex
	settings       RateLimitSettings
	states         map[string]*rateState
	violations     []RateLimitViolation
	violationLimit int
}

type rateState struct {
	secondWindow      time.Time
	secondRequests    int
	minuteWindow      time.Time
	minuteRequests    int
	hourWindow        time.Time
	hourRequests      int
	dayWindow         time.Time
	dayRequests       int
	connWindow        time.Time
	newConnections    int
	uploadWindow      time.Time
	uploadedBytes     int64
	downloadWindow    time.Time
	downloadedBytes   int64
	totalBytesWindow  time.Time
	totalBytes        int64
	activeConnections int
	lastSeen          time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		settings: RateLimitSettings{
			RPS:                   1000,
			RPM:                   50000,
			RPH:                   2000000,
			RPD:                   20000000,
			ConcurrentConnections: 200,
			ConnectionsPerSecond:  200,
			SubnetIPv4Mask:        24,
			SubnetIPv6Mask:        64,
		},
		states:         make(map[string]*rateState),
		violationLimit: 200,
	}
}

func (r *RateLimiter) UpdateSettings(settings RateLimitSettings) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if settings.SubnetIPv4Mask <= 0 {
		settings.SubnetIPv4Mask = 24
	}
	if settings.SubnetIPv6Mask <= 0 {
		settings.SubnetIPv6Mask = 64
	}
	r.settings = settings
}

func (r *RateLimiter) Settings() RateLimitSettings {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settings
}

func (r *RateLimiter) Acquire(ip string, uploadHint int64, now time.Time) (*RateLimitLease, *RateLimitViolation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := r.scopeKeys(ip)
	states := make([]*rateState, 0, len(keys))
	for _, key := range keys {
		state := r.states[key]
		if state == nil {
			state = &rateState{}
			r.states[key] = state
		}
		r.rollWindows(state, now)
		state.lastSeen = now
		states = append(states, state)
	}

	for index, state := range states {
		scope := "ip"
		if index > 0 {
			scope = "subnet"
		}
		if violation := r.checkViolation(ip, scope, state, uploadHint, now); violation != nil {
			r.recordViolation(*violation)
			return nil, violation
		}
	}

	for _, state := range states {
		state.secondRequests++
		state.minuteRequests++
		state.hourRequests++
		state.dayRequests++
		state.newConnections++
		state.activeConnections++
	}

	r.cleanup(now)
	return &RateLimitLease{limiter: r, keys: keys, now: now}, nil
}

func (r *RateLimiter) Release(lease *RateLimitLease, uploadedBytes, downloadedBytes int64) {
	if lease == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, key := range lease.keys {
		state := r.states[key]
		if state == nil {
			continue
		}
		r.rollWindows(state, lease.now)
		if state.activeConnections > 0 {
			state.activeConnections--
		}
		state.uploadedBytes += uploadedBytes
		state.downloadedBytes += downloadedBytes
		state.totalBytes += uploadedBytes + downloadedBytes
		state.lastSeen = time.Now()
	}
}

func (r *RateLimiter) Violations(limit int) []RateLimitViolation {
	r.mu.Lock()
	defer r.mu.Unlock()

	if limit <= 0 || limit > len(r.violations) {
		limit = len(r.violations)
	}
	result := append([]RateLimitViolation{}, r.violations[:limit]...)
	return result
}

func (r *RateLimiter) checkViolation(ip, scope string, state *rateState, uploadHint int64, now time.Time) *RateLimitViolation {
	settings := r.settings

	checks := []struct {
		limitName string
		current   int64
		next      int64
		limit     int64
	}{
		{limitName: "RPS", current: int64(state.secondRequests), next: int64(state.secondRequests + 1), limit: int64(settings.RPS)},
		{limitName: "RPM", current: int64(state.minuteRequests), next: int64(state.minuteRequests + 1), limit: int64(settings.RPM)},
		{limitName: "RPH", current: int64(state.hourRequests), next: int64(state.hourRequests + 1), limit: int64(settings.RPH)},
		{limitName: "RPD", current: int64(state.dayRequests), next: int64(state.dayRequests + 1), limit: int64(settings.RPD)},
		{limitName: "CONCURRENT", current: int64(state.activeConnections), next: int64(state.activeConnections + 1), limit: int64(settings.ConcurrentConnections)},
		{limitName: "CONNECTIONS_PER_SECOND", current: int64(state.newConnections), next: int64(state.newConnections + 1), limit: int64(settings.ConnectionsPerSecond)},
	}

	for _, check := range checks {
		if check.limit > 0 && check.next > check.limit {
			return &RateLimitViolation{
				IP:       ip,
				Scope:    scope,
				Limit:    check.limitName,
				Exceeded: fmt.Sprintf("%d/%d", check.next, check.limit),
				Time:     now.UTC(),
				Reason:   "request rate exceeded",
			}
		}
	}

	if settings.UploadBytesPerSecond > 0 && uploadHint > 0 && state.uploadedBytes+uploadHint > settings.UploadBytesPerSecond {
		return &RateLimitViolation{
			IP:       ip,
			Scope:    scope,
			Limit:    "UPLOAD_BPS",
			Exceeded: fmt.Sprintf("%d/%d", state.uploadedBytes+uploadHint, settings.UploadBytesPerSecond),
			Time:     now.UTC(),
			Reason:   "upload throughput exceeded",
		}
	}

	if settings.DownloadBytesPerSecond > 0 && state.downloadedBytes >= settings.DownloadBytesPerSecond {
		return &RateLimitViolation{
			IP:       ip,
			Scope:    scope,
			Limit:    "DOWNLOAD_BPS",
			Exceeded: fmt.Sprintf("%d/%d", state.downloadedBytes, settings.DownloadBytesPerSecond),
			Time:     now.UTC(),
			Reason:   "download throughput exceeded",
		}
	}

	if settings.TotalBytesPerDay > 0 && uploadHint > 0 && state.totalBytes+uploadHint > settings.TotalBytesPerDay {
		return &RateLimitViolation{
			IP:       ip,
			Scope:    scope,
			Limit:    "TOTAL_BYTES",
			Exceeded: fmt.Sprintf("%d/%d", state.totalBytes+uploadHint, settings.TotalBytesPerDay),
			Time:     now.UTC(),
			Reason:   "daily traffic limit exceeded",
		}
	}

	return nil
}

func (r *RateLimiter) rollWindows(state *rateState, now time.Time) {
	second := now.Truncate(time.Second)
	minute := now.Truncate(time.Minute)
	hour := now.Truncate(time.Hour)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if state.secondWindow != second {
		state.secondWindow = second
		state.secondRequests = 0
		state.connWindow = second
		state.newConnections = 0
		state.uploadWindow = second
		state.uploadedBytes = 0
		state.downloadWindow = second
		state.downloadedBytes = 0
	}
	if state.minuteWindow != minute {
		state.minuteWindow = minute
		state.minuteRequests = 0
	}
	if state.hourWindow != hour {
		state.hourWindow = hour
		state.hourRequests = 0
	}
	if state.dayWindow != day {
		state.dayWindow = day
		state.dayRequests = 0
		state.totalBytesWindow = day
		state.totalBytes = 0
	}
}

func (r *RateLimiter) scopeKeys(ip string) []string {
	keys := []string{"ip:" + ip}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return keys
	}

	mask := r.settings.SubnetIPv4Mask
	if addr.Is6() {
		mask = r.settings.SubnetIPv6Mask
	}
	prefix, err := addr.Prefix(mask)
	if err != nil {
		return keys
	}
	keys = append(keys, "subnet:"+prefix.Masked().String())
	return keys
}

func (r *RateLimiter) recordViolation(violation RateLimitViolation) {
	r.violations = append([]RateLimitViolation{violation}, r.violations...)
	if len(r.violations) > r.violationLimit {
		r.violations = r.violations[:r.violationLimit]
	}
}

func (r *RateLimiter) cleanup(now time.Time) {
	cutoff := now.Add(-24 * time.Hour)
	for key, state := range r.states {
		if state.activeConnections == 0 && state.lastSeen.Before(cutoff) {
			delete(r.states, key)
		}
	}
}

func (r *RateLimiter) TopViolators(limit int) []RateLimitViolation {
	violations := r.Violations(0)
	sort.SliceStable(violations, func(i, j int) bool {
		return violations[i].Time.After(violations[j].Time)
	})
	if limit > 0 && len(violations) > limit {
		return violations[:limit]
	}
	return violations
}
