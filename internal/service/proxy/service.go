package proxy

import (
	"ProxyService2/internal/config"
	"ProxyService2/internal/domain"
	"ProxyService2/internal/ports"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type ForwardRequest struct {
	Context       context.Context
	Method        string
	RequestPath   string
	RawQuery      string
	Header        http.Header
	Body          []byte
	ContentLength int64
}

type ForwardResponse struct {
	StatusCode   int
	Header       http.Header
	Body         []byte
	CacheStatus  string
	RequestBytes int64
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

type Service struct {
	store   ports.ConfigStore
	access  ports.AccessService
	rate    ports.RateLimiter
	cache   ports.CacheService
	metrics ports.ObservabilityService
	client  *http.Client
}

func NewService(
	store ports.ConfigStore,
	access ports.AccessService,
	rate ports.RateLimiter,
	cache ports.CacheService,
	metrics ports.ObservabilityService,
	client *http.Client,
) *Service {
	if client == nil {
		client = &http.Client{}
	}
	return &Service{
		store:   store,
		access:  access,
		rate:    rate,
		cache:   cache,
		metrics: metrics,
		client:  client,
	}
}

func (s *Service) CaptchaHeader() string {
	return s.access.Settings().CaptchaHeader
}

func (s *Service) EvaluateAccess(clientIP, captchaValue string) domain.AccessDecision {
	return s.access.Check(clientIP, captchaValue)
}

func (s *Service) AcquireRateLease(clientIP string, uploadHint int64, now time.Time) (*domain.RateLimitLease, *domain.RateLimitViolation) {
	return s.rate.Acquire(clientIP, uploadHint, now)
}

func (s *Service) ReleaseRateLease(lease *domain.RateLimitLease, uploadedBytes, downloadedBytes int64) {
	s.rate.Release(lease, uploadedBytes, downloadedBytes)
}

func (s *Service) Forward(request ForwardRequest) (ForwardResponse, error) {
	cfg := s.store.Current()
	targetURL, err := joinProxyTarget(cfg.Proxy.UpstreamBaseURL, request.RequestPath, request.RawQuery)
	if err != nil {
		return ForwardResponse{}, &HTTPError{StatusCode: http.StatusBadGateway, Message: err.Error()}
	}

	httpRequest := &http.Request{
		Method: request.Method,
		Header: request.Header.Clone(),
	}

	_, shouldCache, _ := s.cache.ShouldUse(targetURL.Host, targetURL.Path, httpRequest)
	if shouldCache {
		cacheKey := s.cache.BuildKey(request.Method, targetURL.String())
		if cached, ok := s.cache.Get(cacheKey); ok {
			return ForwardResponse{
				StatusCode:   cached.Status,
				Header:       cached.Header,
				Body:         cached.Body,
				CacheStatus:  "HIT",
				RequestBytes: maxInt64(request.ContentLength, int64(len(request.Body))),
			}, nil
		}
	}

	timeout := config.ParseDurationOrDefault(cfg.Proxy.Timeout, 15*time.Second)
	reqCtx, cancel := context.WithTimeout(request.Context, timeout)
	defer cancel()

	outboundRequest, err := http.NewRequestWithContext(reqCtx, request.Method, targetURL.String(), bytes.NewReader(request.Body))
	if err != nil {
		return ForwardResponse{}, &HTTPError{StatusCode: http.StatusBadGateway, Message: err.Error()}
	}
	copyRequestHeaders(outboundRequest.Header, request.Header)
	outboundRequest.Host = targetURL.Host

	startedAt := time.Now()
	response, err := s.client.Do(outboundRequest)
	if err != nil {
		s.metrics.UpdateUpstreamStatus(domain.UpstreamStatus{
			Name:      targetURL.Host,
			URL:       cfg.Proxy.UpstreamBaseURL,
			Healthy:   false,
			LatencyMs: time.Since(startedAt).Milliseconds(),
			Errors:    1,
			CheckedAt: time.Now().UTC(),
		})
		return ForwardResponse{}, &HTTPError{StatusCode: http.StatusBadGateway, Message: "upstream unavailable"}
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return ForwardResponse{}, &HTTPError{StatusCode: http.StatusBadGateway, Message: err.Error()}
	}

	s.metrics.UpdateUpstreamStatus(domain.UpstreamStatus{
		Name:      targetURL.Host,
		URL:       cfg.Proxy.UpstreamBaseURL,
		Healthy:   response.StatusCode < http.StatusInternalServerError,
		LatencyMs: time.Since(startedAt).Milliseconds(),
		CheckedAt: time.Now().UTC(),
	})

	cacheStatus := "BYPASS"
	if shouldCache {
		cacheStatus = "MISS"
	}

	cacheKey := s.cache.BuildKey(request.Method, targetURL.String())
	if response.StatusCode >= 200 && response.StatusCode < 400 &&
		(request.Method != http.MethodGet && request.Method != http.MethodHead) {
		s.cache.InvalidateForMutation(targetURL.Path)
	}
	if shouldCache {
		_ = s.cache.Store(targetURL.Host, targetURL.Path, cacheKey, response.StatusCode, response.Header, responseBody)
	}

	return ForwardResponse{
		StatusCode:   response.StatusCode,
		Header:       response.Header.Clone(),
		Body:         responseBody,
		CacheStatus:  cacheStatus,
		RequestBytes: int64(len(request.Body)),
	}, nil
}

func (s *Service) RunUpstreamChecks(ctx context.Context) {
	ticker := time.NewTicker(config.ParseDurationOrDefault(s.store.Current().Monitoring.UpstreamCheckInterval, 30*time.Second))
	defer ticker.Stop()

	for {
		s.CheckUpstream()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) CheckUpstream() {
	cfg := s.store.Current()
	timeout := config.ParseDurationOrDefault(cfg.Proxy.Timeout, 15*time.Second)
	target, err := url.Parse(cfg.Proxy.UpstreamBaseURL)
	if err != nil {
		s.metrics.UpdateUpstreamStatus(domain.UpstreamStatus{
			Name:      "upstream",
			URL:       cfg.Proxy.UpstreamBaseURL,
			Healthy:   false,
			LatencyMs: 0,
			Errors:    1,
			CheckedAt: time.Now().UTC(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startedAt := time.Now()
	request, _ := http.NewRequestWithContext(ctx, http.MethodHead, target.String(), nil)
	response, err := s.client.Do(request)
	status := domain.UpstreamStatus{
		Name:      target.Host,
		URL:       target.String(),
		Healthy:   err == nil,
		LatencyMs: time.Since(startedAt).Milliseconds(),
		CheckedAt: time.Now().UTC(),
	}
	if err == nil && response != nil {
		status.Healthy = response.StatusCode < http.StatusInternalServerError
		_ = response.Body.Close()
	} else if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			status.Healthy = false
		}
		status.Errors = 1
	}
	s.metrics.UpdateUpstreamStatus(status)
}

func joinProxyTarget(baseURL, requestPath, rawQuery string) (*url.URL, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	base.Path = singleJoiningSlash(base.Path, strings.TrimPrefix(requestPath, "/"))
	base.RawQuery = rawQuery
	return base, nil
}

func singleJoiningSlash(a, b string) string {
	if a == "" {
		return "/" + strings.TrimPrefix(b, "/")
	}
	return path.Join(a, b)
}

func copyRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func maxInt64(value, fallback int64) int64 {
	if value >= 0 {
		return value
	}
	return fallback
}
