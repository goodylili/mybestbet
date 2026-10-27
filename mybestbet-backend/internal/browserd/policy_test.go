package browserd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/sources"
)

func TestRequestPolicy_Normalize(t *testing.T) {
	t.Parallel()
	policy := RequestPolicy{AllowedHostSuffixes: []string{"bet365.com", " PrizePicks.com "}, MaxWait: 10 * time.Second}
	tests := []struct {
		name        string
		req         sources.BrowserRequest
		wantErr     error
		wantProfile string
		wantWait    int
	}{
		{name: "defaults profile and wait", req: sources.BrowserRequest{URL: "https://www.bet365.com/#/AS/B1/"}, wantProfile: DefaultProfile, wantWait: 3000},
		{name: "keeps explicit values", req: sources.BrowserRequest{Profile: "prizepicks", URL: "https://app.prizepicks.com/", WaitMillis: 5000}, wantProfile: "prizepicks", wantWait: 5000},
		{name: "exact host match", req: sources.BrowserRequest{URL: "https://bet365.com/"}, wantProfile: DefaultProfile, wantWait: 3000},
		{name: "host suffix trick rejected", req: sources.BrowserRequest{URL: "https://evilbet365.com/"}, wantErr: ErrHostNotAllowed},
		{name: "unlisted host rejected", req: sources.BrowserRequest{URL: "http://169.254.169.254/latest"}, wantErr: ErrHostNotAllowed},
		{name: "file scheme rejected", req: sources.BrowserRequest{URL: "file:///etc/passwd"}, wantErr: ErrInvalidRequest},
		{name: "profile path traversal rejected", req: sources.BrowserRequest{Profile: "../etc", URL: "https://www.bet365.com/"}, wantErr: ErrInvalidRequest},
		{name: "wait above max rejected", req: sources.BrowserRequest{URL: "https://www.bet365.com/", WaitMillis: 11000}, wantErr: ErrInvalidRequest},
		{name: "negative wait rejected", req: sources.BrowserRequest{URL: "https://www.bet365.com/", WaitMillis: -1}, wantErr: ErrInvalidRequest},
		{name: "empty capture path rejected", req: sources.BrowserRequest{URL: "https://www.bet365.com/", CapturePath: []string{""}}, wantErr: ErrInvalidRequest},
		{name: "oversized capture path rejected", req: sources.BrowserRequest{URL: "https://www.bet365.com/", CapturePath: []string{strings.Repeat("a", 300)}}, wantErr: ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := policy.Normalize(tt.req)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Normalize() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize() unexpected error: %v", err)
			}
			if got.Profile != tt.wantProfile || got.WaitMillis != tt.wantWait {
				t.Fatalf("Normalize() = profile %q wait %d, want %q %d", got.Profile, got.WaitMillis, tt.wantProfile, tt.wantWait)
			}
		})
	}
}
