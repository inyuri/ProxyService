package cache

import (
	"container/list"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CacheRule struct {
	Name             string   `json:"name"`
	Domains          []string `json:"domains,omitempty"`
	PathPrefixes     []string `json:"pathPrefixes,omitempty"`
	TTL2xx           time.Duration
	TTL3xx           time.Duration
	TTL4xx           time.Duration
	TTL5xx           time.Duration
	MinSizeBytes     int      `json:"minSizeBytes"`
	MaxSizeBytes     int      `json:"maxSizeBytes"`
	Tags             []string `json:"tags,omitempty"`
	SensitiveHeaders []string `json:"sensitiveHeaders,omitempty"`
}

type CacheSettings struct {
	Enabled          bool
	Capacity         int
	MemoryLimitBytes int64
	DefaultRule      CacheRule
	Rules            []CacheRule
}

type CacheEntryView struct {
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Hits      int       `json:"hits"`
	Size      int       `json:"size"`
	SizeHuman string    `json:"sizeHuman"`
	TTL       string    `json:"ttl"`
	Tags      []string  `json:"tags"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CacheStats struct {
	Hits          uint64  `json:"hits"`
	Misses        uint64  `json:"misses"`
	HitRate       float64 `json:"hitRate"`
	TotalKeys     int     `json:"totalKeys"`
	MemoryUsed    int64   `json:"memoryUsed"`
	MemoryUsedUI  string  `json:"memoryUsedUI"`
	MemoryLimit   int64   `json:"memoryLimit"`
	MemoryLimitUI string  `json:"memoryLimitUI"`
}

type CachedResponse struct {
	Status    int
	Header    http.Header
	Body      []byte
	ExpiresAt time.Time
}

type CacheInvalidationRequest struct {
	Key    string   `json:"key"`
	Prefix string   `json:"prefix"`
	Regex  string   `json:"regex"`
	Tags   []string `json:"tags"`
}

type CacheSnapshot struct {
	Stats    CacheStats            `json:"stats"`
	Entries  []CacheEntryView      `json:"entries"`
	Settings CacheSnapshotSettings `json:"settings"`
}

type CacheSnapshotSettings struct {
	Enabled         bool   `json:"enabled"`
	Capacity        int    `json:"capacity"`
	Default2xxTTL   string `json:"default2xxTtl"`
	Default3xxTTL   string `json:"default3xxTtl"`
	Default4xxTTL   string `json:"default4xxTtl"`
	Default5xxTTL   string `json:"default5xxTtl"`
	MemoryLimitText string `json:"memoryLimitText"`
}

type CacheService struct {
	mu       sync.Mutex
	settings CacheSettings
	entries  map[string]*list.Element
	order    *list.List
	tags     map[string]map[string]struct{}
	hits     uint64
	misses   uint64
	memory   int64
}

type cacheEntry struct {
	key       string
	path      string
	host      string
	status    int
	header    http.Header
	body      []byte
	expiresAt time.Time
	createdAt time.Time
	tags      []string
	hits      int
}

func NewCacheService() *CacheService {
	return &CacheService{
		settings: CacheSettings{
			Enabled:          true,
			Capacity:         256,
			MemoryLimitBytes: 128 * 1024 * 1024,
			DefaultRule: CacheRule{
				Name:             "default",
				TTL2xx:           5 * time.Minute,
				TTL3xx:           time.Minute,
				TTL4xx:           30 * time.Second,
				TTL5xx:           15 * time.Second,
				MinSizeBytes:     1,
				MaxSizeBytes:     2 * 1024 * 1024,
				SensitiveHeaders: []string{"Authorization", "Cookie"},
			},
		},
		entries: make(map[string]*list.Element),
		order:   list.New(),
		tags:    make(map[string]map[string]struct{}),
	}
}

func (c *CacheService) UpdateSettings(settings CacheSettings) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if settings.Capacity <= 0 {
		settings.Capacity = 256
	}
	if settings.MemoryLimitBytes <= 0 {
		settings.MemoryLimitBytes = 128 * 1024 * 1024
	}
	c.settings = settings
	c.evictIfNeeded()
}

func (c *CacheService) BuildKey(method, url string) string {
	return method + ":" + url
}

func (c *CacheService) Get(key string) (*CachedResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element := c.entries[key]
	if element == nil {
		c.misses++
		return nil, false
	}

	entry := element.Value.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.removeElement(element)
		c.misses++
		return nil, false
	}

	entry.hits++
	c.order.MoveToFront(element)
	c.hits++
	return &CachedResponse{
		Status:    entry.status,
		Header:    cloneHeader(entry.header),
		Body:      append([]byte{}, entry.body...),
		ExpiresAt: entry.expiresAt,
	}, true
}

func (c *CacheService) ShouldUse(host, path string, req *http.Request) (CacheRule, bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.settings.Enabled {
		return CacheRule{}, false, "disabled"
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return CacheRule{}, false, "method_not_cacheable"
	}
	if hasNoCacheDirective(req.Header.Get("Cache-Control")) || strings.EqualFold(req.Header.Get("Pragma"), "no-cache") {
		return CacheRule{}, false, "no_cache_request"
	}

	rule := c.resolveRule(host, path)
	for _, header := range rule.SensitiveHeaders {
		if req.Header.Get(header) != "" {
			return CacheRule{}, false, "sensitive_request"
		}
	}
	return rule, true, ""
}

func (c *CacheService) Store(host, path, key string, status int, header http.Header, body []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.settings.Enabled {
		return false
	}

	rule := c.resolveRule(host, path)
	ttl := ttlForStatus(rule, status)
	if ttl <= 0 {
		return false
	}
	if len(body) < rule.MinSizeBytes {
		return false
	}
	if rule.MaxSizeBytes > 0 && len(body) > rule.MaxSizeBytes {
		return false
	}
	if hasNoStoreDirective(header.Get("Cache-Control")) {
		return false
	}
	if hasNoCacheDirective(header.Get("Cache-Control")) {
		c.invalidateExactLocked(key)
		return false
	}

	entry := &cacheEntry{
		key:       key,
		path:      path,
		host:      host,
		status:    status,
		header:    cloneHeader(header),
		body:      append([]byte{}, body...),
		expiresAt: time.Now().Add(ttl),
		createdAt: time.Now(),
		tags:      deriveTags(host, path, header, rule.Tags),
	}

	if existing := c.entries[key]; existing != nil {
		c.removeElement(existing)
	}

	element := c.order.PushFront(entry)
	c.entries[key] = element
	c.memory += int64(len(entry.body))
	for _, tag := range entry.tags {
		if c.tags[tag] == nil {
			c.tags[tag] = make(map[string]struct{})
		}
		c.tags[tag][entry.key] = struct{}{}
	}
	c.evictIfNeeded()
	return true
}

func (c *CacheService) Invalidate(req CacheInvalidationRequest) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	if req.Key != "" {
		if c.invalidateExactLocked(req.Key) {
			removed++
		}
	}
	if req.Prefix != "" {
		for key, element := range c.entries {
			entry := element.Value.(*cacheEntry)
			if strings.HasPrefix(key, req.Prefix) || strings.HasPrefix(entry.path, req.Prefix) {
				c.removeElement(element)
				delete(c.entries, key)
				removed++
			}
		}
	}
	if req.Regex != "" {
		pattern, err := regexp.Compile(req.Regex)
		if err != nil {
			return 0, err
		}
		for key, element := range c.entries {
			entry := element.Value.(*cacheEntry)
			if pattern.MatchString(key) || pattern.MatchString(entry.path) {
				c.removeElement(element)
				delete(c.entries, key)
				removed++
			}
		}
	}
	if len(req.Tags) > 0 {
		seen := make(map[string]struct{})
		for _, tag := range req.Tags {
			for key := range c.tags[tag] {
				if _, ok := seen[key]; ok {
					continue
				}
				if element := c.entries[key]; element != nil {
					c.removeElement(element)
					delete(c.entries, key)
					removed++
					seen[key] = struct{}{}
				}
			}
		}
	}
	return removed, nil
}

func (c *CacheService) InvalidateAll() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := len(c.entries)
	c.entries = make(map[string]*list.Element)
	c.order.Init()
	c.tags = make(map[string]map[string]struct{})
	c.memory = 0
	return removed
}

func (c *CacheService) InvalidateForMutation(path string) int {
	return c.invalidateByPrefix(path)
}

func (c *CacheService) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total) * 100
	}

	return CacheStats{
		Hits:          c.hits,
		Misses:        c.misses,
		HitRate:       hitRate,
		TotalKeys:     len(c.entries),
		MemoryUsed:    c.memory,
		MemoryUsedUI:  humanBytes(c.memory),
		MemoryLimit:   c.settings.MemoryLimitBytes,
		MemoryLimitUI: humanBytes(c.settings.MemoryLimitBytes),
	}
}

func (c *CacheService) Entries(limit int) []CacheEntryView {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]CacheEntryView, 0, len(c.entries))
	for element := c.order.Front(); element != nil; element = element.Next() {
		entry := element.Value.(*cacheEntry)
		ttl := time.Until(entry.expiresAt)
		if ttl < 0 {
			ttl = 0
		}
		result = append(result, CacheEntryView{
			Key:       entry.key,
			Path:      entry.path,
			Status:    entry.status,
			Hits:      entry.hits,
			Size:      len(entry.body),
			SizeHuman: humanBytes(int64(len(entry.body))),
			TTL:       ttl.Round(time.Second).String(),
			Tags:      append([]string{}, entry.tags...),
			ExpiresAt: entry.expiresAt,
		})
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Hits > result[j].Hits
	})
	return result
}

func (c *CacheService) Snapshot(limit int) CacheSnapshot {
	c.mu.Lock()
	settings := c.settings
	c.mu.Unlock()

	return CacheSnapshot{
		Stats:   c.Stats(),
		Entries: c.Entries(limit),
		Settings: CacheSnapshotSettings{
			Enabled:         settings.Enabled,
			Capacity:        settings.Capacity,
			Default2xxTTL:   settings.DefaultRule.TTL2xx.String(),
			Default3xxTTL:   settings.DefaultRule.TTL3xx.String(),
			Default4xxTTL:   settings.DefaultRule.TTL4xx.String(),
			Default5xxTTL:   settings.DefaultRule.TTL5xx.String(),
			MemoryLimitText: humanBytes(settings.MemoryLimitBytes),
		},
	}
}

func (c *CacheService) resolveRule(host, path string) CacheRule {
	best := c.settings.DefaultRule
	bestScore := -1
	for _, rule := range c.settings.Rules {
		if len(rule.Domains) > 0 {
			matched := false
			for _, domain := range rule.Domains {
				if strings.EqualFold(domain, host) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		score := 0
		if len(rule.PathPrefixes) > 0 {
			matched := false
			for _, prefix := range rule.PathPrefixes {
				if strings.HasPrefix(path, prefix) {
					if len(prefix) > score {
						score = len(prefix)
					}
					matched = true
				}
			}
			if !matched {
				continue
			}
		}

		if score > bestScore {
			best = rule
			bestScore = score
		}
	}
	return best
}

func (c *CacheService) evictIfNeeded() {
	for (c.settings.Capacity > 0 && len(c.entries) > c.settings.Capacity) ||
		(c.settings.MemoryLimitBytes > 0 && c.memory > c.settings.MemoryLimitBytes) {
		back := c.order.Back()
		if back == nil {
			return
		}
		c.removeElement(back)
	}
}

func (c *CacheService) invalidateByPrefix(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	for key, element := range c.entries {
		entry := element.Value.(*cacheEntry)
		if strings.HasPrefix(entry.path, prefix) || strings.HasPrefix(key, prefix) {
			c.removeElement(element)
			delete(c.entries, key)
			removed++
		}
	}
	return removed
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

func (c *CacheService) invalidateExactLocked(key string) bool {
	element := c.entries[key]
	if element == nil {
		return false
	}
	c.removeElement(element)
	delete(c.entries, key)
	return true
}

func (c *CacheService) removeElement(element *list.Element) {
	entry := element.Value.(*cacheEntry)
	c.order.Remove(element)
	delete(c.entries, entry.key)
	c.memory -= int64(len(entry.body))
	for _, tag := range entry.tags {
		delete(c.tags[tag], entry.key)
		if len(c.tags[tag]) == 0 {
			delete(c.tags, tag)
		}
	}
}

func ttlForStatus(rule CacheRule, status int) time.Duration {
	switch {
	case status >= 200 && status < 300:
		return rule.TTL2xx
	case status >= 300 && status < 400:
		return rule.TTL3xx
	case status >= 400 && status < 500:
		return rule.TTL4xx
	default:
		return rule.TTL5xx
	}
}

func deriveTags(host, path string, header http.Header, staticTags []string) []string {
	tags := append([]string{}, staticTags...)
	tags = append(tags, "host:"+host, "path:"+path)
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) > 0 && segments[0] != "" {
		tags = append(tags, "segment:"+segments[0])
	}
	if headerTag := header.Get("X-Cache-Tags"); headerTag != "" {
		for _, tag := range strings.Split(headerTag, ",") {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
	}
	return tags
}

func hasNoStoreDirective(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "no-store") || strings.Contains(value, "private")
}

func hasNoCacheDirective(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "no-cache")
}

func cloneHeader(header http.Header) http.Header {
	result := make(http.Header, len(header))
	for key, values := range header {
		result[key] = append([]string{}, values...)
	}
	return result
}

func (c *CacheService) DebugState() string {
	stats := c.Stats()
	return fmt.Sprintf("hits=%d misses=%d keys=%d", stats.Hits, stats.Misses, stats.TotalKeys)
}
