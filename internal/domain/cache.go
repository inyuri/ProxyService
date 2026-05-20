package domain

import (
	"net/http"
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
