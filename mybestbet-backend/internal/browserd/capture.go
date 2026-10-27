package browserd

import (
	"maps"
	"net/http"
	"strings"
	"sync"

	"github.com/go-rod/rod/lib/proto"

	"mybestbet/pkg/sources"
)

const (
	maxCapturedBytes  = 24 << 20
	maxCapturedFrames = 2000
)

var forwardedRequestHeaders = map[string]bool{
	"user-agent":      true,
	"accept":          true,
	"accept-language": true,
	"referer":         true,
	"origin":          true,
}

func matchesCapturePath(url string, capturePaths []string) bool {
	for _, path := range capturePaths {
		if strings.Contains(url, path) {
			return true
		}
	}
	return false
}

func isForwardableHeader(name string) bool {
	name = strings.ToLower(name)
	return forwardedRequestHeaders[name] || strings.HasPrefix(name, "x-")
}

type matchedResponse struct {
	requestID proto.NetworkRequestID
	url       string
	status    int
}

type collector struct {
	capturePaths []string
	captureWS    bool

	mu             sync.Mutex
	responses      map[proto.NetworkRequestID]matchedResponse
	finished       []proto.NetworkRequestID
	socketURLs     map[proto.NetworkRequestID]string
	frames         []sources.Captured
	requestHeaders map[string]string
	frameBytes     int
}

func newCollector(req sources.BrowserRequest) *collector {
	return &collector{
		capturePaths:   req.CapturePath,
		captureWS:      req.CaptureWS,
		responses:      map[proto.NetworkRequestID]matchedResponse{},
		socketURLs:     map[proto.NetworkRequestID]string{},
		requestHeaders: map[string]string{},
	}
}

func (c *collector) onRequestWillBeSent(e *proto.NetworkRequestWillBeSent) {
	if e.Request == nil || !matchesCapturePath(e.Request.URL, c.capturePaths) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requestHeaders) > 0 {
		return
	}
	for name, value := range e.Request.Headers {
		if isForwardableHeader(name) {
			c.requestHeaders[http.CanonicalHeaderKey(name)] = value.Str()
		}
	}
}

func (c *collector) onResponseReceived(e *proto.NetworkResponseReceived) {
	if e.Response == nil || !matchesCapturePath(e.Response.URL, c.capturePaths) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responses[e.RequestID] = matchedResponse{requestID: e.RequestID, url: e.Response.URL, status: e.Response.Status}
}

func (c *collector) onLoadingFinished(e *proto.NetworkLoadingFinished) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.responses[e.RequestID]; ok {
		c.finished = append(c.finished, e.RequestID)
	}
}

func (c *collector) onWebSocketCreated(e *proto.NetworkWebSocketCreated) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.socketURLs[e.RequestID] = e.URL
}

func (c *collector) onWebSocketFrame(e *proto.NetworkWebSocketFrameReceived) {
	if !c.captureWS || e.Response == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.frames) >= maxCapturedFrames || c.frameBytes+len(e.Response.PayloadData) > maxCapturedBytes {
		return
	}
	c.frameBytes += len(e.Response.PayloadData)
	c.frames = append(c.frames, sources.Captured{URL: c.socketURLs[e.RequestID], Body: []byte(e.Response.PayloadData), WS: true})
}

func (c *collector) finishedResponses() []matchedResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]matchedResponse, len(c.finished))
	for i, id := range c.finished {
		out[i] = c.responses[id]
	}
	return out
}

func (c *collector) capturedFrames() []sources.Captured {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sources.Captured(nil), c.frames...)
}

func (c *collector) forwardedHeaders() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.requestHeaders)
}
