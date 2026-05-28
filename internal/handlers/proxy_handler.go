package handlers

import (
	"ProxyService2/internal/server/middleware"
	"ProxyService2/internal/usecase"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type ProxyHandler struct {
	uc *usecase.ProxyUseCase
}

func NewProxyHandler(uc *usecase.ProxyUseCase) *ProxyHandler {
	return &ProxyHandler{uc: uc}
}

func (h *ProxyHandler) Proxy(c *gin.Context) {
	h.forward(c, c.Param("path"))
}

// ProxyNoRoute is used by the NoRoute handler to proxy any unmatched path.
func (h *ProxyHandler) ProxyNoRoute(c *gin.Context) {
	h.forward(c, c.Request.URL.Path)
}

func (h *ProxyHandler) forward(c *gin.Context, path string) {
	requestBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = c.Request.Body.Close()

	response, err := h.uc.Forward(usecase.ForwardRequest{
		Context:       c.Request.Context(),
		Method:        c.Request.Method,
		RequestPath:   path,
		RawQuery:      c.Request.URL.RawQuery,
		Header:        c.Request.Header,
		Body:          requestBody,
		ContentLength: c.Request.ContentLength,
	})
	if err != nil {
		httpErr := &usecase.HTTPError{StatusCode: http.StatusBadGateway, Message: "proxy error"}
		if asProxyError(err, httpErr) {
			c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.Set(middleware.CtxRequestBytesKey, response.RequestBytes)
	c.Set(middleware.CtxCacheStatusKey, response.CacheStatus)
	copyResponseHeaders(c.Writer.Header(), response.Header)
	c.Header("X-Cache-Status", response.CacheStatus)
	c.Status(response.StatusCode)
	if c.Request.Method != http.MethodHead {
		_, _ = c.Writer.Write(response.Body)
	}
}

func asProxyError(err error, target *usecase.HTTPError) bool {
	value, ok := err.(*usecase.HTTPError)
	if !ok {
		return false
	}
	*target = *value
	return true
}

func copyResponseHeaders(dst, src http.Header) {
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
