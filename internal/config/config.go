package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Frontend   FrontendConfig   `yaml:"frontend"`
	Logging    LoggingConfig    `yaml:"logging"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
	Access     AccessConfig     `yaml:"access"`
	RateLimit  RateLimitConfig  `yaml:"rateLimit"`
	Cache      CacheConfig      `yaml:"cache"`
}

type ServerConfig struct {
	Address         string `yaml:"address"`
	ReadTimeout     string `yaml:"readTimeout"`
	WriteTimeout    string `yaml:"writeTimeout"`
	ShutdownTimeout string `yaml:"shutdownTimeout"`
}

type ProxyConfig struct {
	UpstreamBaseURL string `yaml:"upstreamBaseURL"`
	Timeout         string `yaml:"timeout"`
}

type FrontendConfig struct {
	AllowedOrigins []string `yaml:"allowedOrigins"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type MonitoringConfig struct {
	HistoryBuckets        int    `yaml:"historyBuckets"`
	LogBufferSize         int    `yaml:"logBufferSize"`
	UpstreamCheckInterval string `yaml:"upstreamCheckInterval"`
}

type AccessConfig struct {
	DefaultPolicy     string             `yaml:"defaultPolicy"`
	CaptchaHeader     string             `yaml:"captchaHeader"`
	CaptchaToken      string             `yaml:"captchaToken"`
	DecisionCacheTTL  string             `yaml:"decisionCacheTTL"`
	DecisionCacheSize int                `yaml:"decisionCacheSize"`
	Rules             []AccessRuleConfig `yaml:"rules"`
}

type AccessRuleConfig struct {
	ID          string    `yaml:"id"`
	Type        string    `yaml:"type"`
	Value       string    `yaml:"value"`
	Description string    `yaml:"description"`
	CreatedAt   time.Time `yaml:"createdAt"`
}

type RateLimitConfig struct {
	SubnetIPv4Mask int                   `yaml:"subnetIPv4Mask"`
	SubnetIPv6Mask int                   `yaml:"subnetIPv6Mask"`
	Limits         RateLimitValuesConfig `yaml:"limits"`
}

type RateLimitValuesConfig struct {
	RPS                    int   `yaml:"rps"`
	RPM                    int   `yaml:"rpm"`
	RPH                    int   `yaml:"rph"`
	RPD                    int   `yaml:"rpd"`
	ConcurrentConnections  int   `yaml:"concurrentConnections"`
	ConnectionsPerSecond   int   `yaml:"connectionsPerSecond"`
	UploadBytesPerSecond   int64 `yaml:"uploadBytesPerSecond"`
	DownloadBytesPerSecond int64 `yaml:"downloadBytesPerSecond"`
	TotalBytesPerDay       int64 `yaml:"totalBytesPerDay"`
}

type CacheConfig struct {
	Enabled          bool              `yaml:"enabled"`
	Capacity         int               `yaml:"capacity"`
	MemoryLimitBytes int64             `yaml:"memoryLimitBytes"`
	DefaultRule      CacheRuleConfig   `yaml:"defaultRule"`
	Rules            []CacheRuleConfig `yaml:"rules"`
}

type CacheRuleConfig struct {
	Name             string   `yaml:"name"`
	Domains          []string `yaml:"domains"`
	PathPrefixes     []string `yaml:"pathPrefixes"`
	TTL2xx           string   `yaml:"ttl2xx"`
	TTL3xx           string   `yaml:"ttl3xx"`
	TTL4xx           string   `yaml:"ttl4xx"`
	TTL5xx           string   `yaml:"ttl5xx"`
	MinSizeBytes     int      `yaml:"minSizeBytes"`
	MaxSizeBytes     int      `yaml:"maxSizeBytes"`
	Tags             []string `yaml:"tags"`
	SensitiveHeaders []string `yaml:"sensitiveHeaders"`
}

type Store struct {
	path        string
	mu          sync.Mutex
	current     atomic.Value
	subscribers []func(Config)
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Address:         ":8080",
			ReadTimeout:     "15s",
			WriteTimeout:    "15s",
			ShutdownTimeout: "5s",
		},
		Proxy: ProxyConfig{
			UpstreamBaseURL: "https://httpbin.org",
			Timeout:         "15s",
		},
		Frontend: FrontendConfig{
			AllowedOrigins: []string{
				"http://localhost:5173",
				"http://127.0.0.1:5173",
				"http://localhost:4173",
			},
		},
		Logging: LoggingConfig{Level: "info"},
		Monitoring: MonitoringConfig{
			HistoryBuckets:        60,
			LogBufferSize:         500,
			UpstreamCheckInterval: "30s",
		},
		Access: AccessConfig{
			DefaultPolicy:     "allow",
			CaptchaHeader:     "X-Captcha-Token",
			CaptchaToken:      "let-me-in",
			DecisionCacheTTL:  "2m",
			DecisionCacheSize: 2048,
			Rules: []AccessRuleConfig{
				{
					ID:          "local-dev",
					Type:        "allow",
					Value:       "127.0.0.1/32",
					Description: "Local development",
					CreatedAt:   time.Now().UTC(),
				},
			},
		},
		RateLimit: RateLimitConfig{
			SubnetIPv4Mask: 24,
			SubnetIPv6Mask: 64,
			Limits: RateLimitValuesConfig{
				RPS:                    1000,
				RPM:                    50000,
				RPH:                    2000000,
				RPD:                    20000000,
				ConcurrentConnections:  200,
				ConnectionsPerSecond:   200,
				UploadBytesPerSecond:   10 * 1024 * 1024,
				DownloadBytesPerSecond: 20 * 1024 * 1024,
				TotalBytesPerDay:       20 * 1024 * 1024 * 1024,
			},
		},
		Cache: CacheConfig{
			Enabled:          true,
			Capacity:         512,
			MemoryLimitBytes: 128 * 1024 * 1024,
			DefaultRule: CacheRuleConfig{
				Name:             "default",
				TTL2xx:           "5m",
				TTL3xx:           "1m",
				TTL4xx:           "30s",
				TTL5xx:           "15s",
				MinSizeBytes:     1,
				MaxSizeBytes:     2 * 1024 * 1024,
				SensitiveHeaders: []string{"Authorization", "Cookie"},
			},
		},
	}
}

func EnsureDefaultConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func NewStore(path string) (*Store, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}

	store := &Store{path: path}
	store.current.Store(cfg)
	return store, nil
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s *Store) Current() Config {
	return s.current.Load().(Config)
}

func (s *Store) Subscribe(fn func(Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers = append(s.subscribers, fn)
}

func (s *Store) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return err
	}

	go func() {
		defer watcher.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != filepath.Base(s.path) {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				_, _ = s.Reload()
			case <-watcher.Errors:
			}
		}
	}()
	return nil
}

func (s *Store) Reload() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := Load(s.path)
	if err != nil {
		return Config{}, err
	}
	s.current.Store(cfg)
	s.broadcastLocked(cfg)
	return cfg, nil
}

func (s *Store) Update(mutator func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.current.Load().(Config)
	if err := mutator(&cfg); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return err
	}

	s.current.Store(cfg)
	s.broadcastLocked(cfg)
	return nil
}

func (s *Store) broadcastLocked(cfg Config) {
	subscribers := append([]func(Config){}, s.subscribers...)
	go func() {
		for _, fn := range subscribers {
			fn(cfg)
		}
	}()
}

func ParseDurationOrDefault(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
