package browserd

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"

	"mybestbet/pkg/sources"
)

func TestMatchesCapturePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		url   string
		paths []string
		want  bool
	}{
		{name: "substring match", url: "https://www.bet365.com/SportsBook.API/web?lid=1", paths: []string{"SportsBook.API"}, want: true},
		{name: "any of several", url: "https://api.prizepicks.com/projections?league_id=82", paths: []string{"/leagues", "/projections"}, want: true},
		{name: "case sensitive", url: "https://www.bet365.com/sportsbook.api/web", paths: []string{"SportsBook.API"}, want: false},
		{name: "no paths captures nothing", url: "https://www.bet365.com/SportsBook.API/web", paths: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesCapturePath(tt.url, tt.paths); got != tt.want {
				t.Fatalf("matchesCapturePath(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestCollector_KeepsOnlyMatchingFinishedResponses(t *testing.T) {
	t.Parallel()
	c := newCollector(sources.BrowserRequest{CapturePath: []string{"/projections"}})
	c.onRequestWillBeSent(&proto.NetworkRequestWillBeSent{RequestID: "1", Request: &proto.NetworkRequest{
		URL: "https://api.prizepicks.com/projections",
		Headers: proto.NetworkHeaders{
			"user-agent":   gson.New("UA"),
			"x-device-id":  gson.New("dev"),
			"cookie":       gson.New("secret"),
			"Content-Type": gson.New("application/json"),
		},
	}})
	c.onResponseReceived(&proto.NetworkResponseReceived{RequestID: "1", Response: &proto.NetworkResponse{URL: "https://api.prizepicks.com/projections", Status: 200}})
	c.onResponseReceived(&proto.NetworkResponseReceived{RequestID: "2", Response: &proto.NetworkResponse{URL: "https://app.prizepicks.com/main.js", Status: 200}})
	c.onResponseReceived(&proto.NetworkResponseReceived{RequestID: "3", Response: &proto.NetworkResponse{URL: "https://api.prizepicks.com/projections?page=2", Status: 200}})
	c.onLoadingFinished(&proto.NetworkLoadingFinished{RequestID: "1"})
	c.onLoadingFinished(&proto.NetworkLoadingFinished{RequestID: "2"})

	finished := c.finishedResponses()
	if len(finished) != 1 || finished[0].requestID != "1" || finished[0].status != 200 {
		t.Fatalf("finishedResponses() = %+v, want only request 1", finished)
	}
	headers := c.forwardedHeaders()
	want := map[string]string{"User-Agent": "UA", "X-Device-Id": "dev"}
	if len(headers) != len(want) {
		t.Fatalf("forwardedHeaders() = %v, want %v", headers, want)
	}
	for k, v := range want {
		if headers[k] != v {
			t.Fatalf("forwardedHeaders()[%q] = %q, want %q", k, headers[k], v)
		}
	}
}

func TestCollector_WebSocketFramesOnlyWhenRequested(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		captureWS  bool
		wantFrames int
	}{
		{name: "captures frames", captureWS: true, wantFrames: 1},
		{name: "ignores frames", captureWS: false, wantFrames: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newCollector(sources.BrowserRequest{CaptureWS: tt.captureWS})
			c.onWebSocketCreated(&proto.NetworkWebSocketCreated{RequestID: "ws", URL: "wss://premws-pt1.365lpodds.com/zap/"})
			c.onWebSocketFrame(&proto.NetworkWebSocketFrameReceived{RequestID: "ws", Response: &proto.NetworkWebSocketFrame{PayloadData: "F|EV;FI=1|"}})
			frames := c.capturedFrames()
			if len(frames) != tt.wantFrames {
				t.Fatalf("capturedFrames() = %d frames, want %d", len(frames), tt.wantFrames)
			}
			if tt.wantFrames > 0 && (!frames[0].WS || frames[0].URL != "wss://premws-pt1.365lpodds.com/zap/") {
				t.Fatalf("frame = %+v, want ws frame with socket url", frames[0])
			}
		})
	}
}
