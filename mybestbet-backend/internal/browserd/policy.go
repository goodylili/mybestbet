package browserd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"mybestbet/pkg/sources"
)

const (
	DefaultProfile      = "default"
	defaultWait         = 3 * time.Second
	maxCapturePaths     = 16
	maxCapturePathBytes = 256
)

var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$`)

type RequestPolicy struct {
	AllowedHostSuffixes []string
	MaxWait             time.Duration
}

func (p RequestPolicy) Normalize(req sources.BrowserRequest) (sources.BrowserRequest, error) {
	if req.Profile == "" {
		req.Profile = DefaultProfile
	}
	if !profileNamePattern.MatchString(req.Profile) {
		return req, fmt.Errorf("%w: profile %q", ErrInvalidRequest, req.Profile)
	}
	if err := p.checkTarget(req.URL); err != nil {
		return req, err
	}
	if err := checkCapturePaths(req.CapturePath); err != nil {
		return req, err
	}
	wait := time.Duration(req.WaitMillis) * time.Millisecond
	switch {
	case wait < 0 || wait > p.MaxWait:
		return req, fmt.Errorf("%w: waitMillis %d outside 0..%d", ErrInvalidRequest, req.WaitMillis, p.MaxWait.Milliseconds())
	case wait == 0:
		req.WaitMillis = int(defaultWait.Milliseconds())
	}
	return req, nil
}

func (p RequestPolicy) checkTarget(raw string) error {
	target, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: url: %w", ErrInvalidRequest, err)
	}
	if target.Scheme != "https" && target.Scheme != "http" {
		return fmt.Errorf("%w: url scheme %q", ErrInvalidRequest, target.Scheme)
	}
	if !p.isHostAllowed(target.Hostname()) {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, target.Hostname())
	}
	return nil
}

func (p RequestPolicy) isHostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, suffix := range p.AllowedHostSuffixes {
		suffix = strings.ToLower(strings.TrimSpace(suffix))
		if suffix != "" && (host == suffix || strings.HasSuffix(host, "."+suffix)) {
			return true
		}
	}
	return false
}

func checkCapturePaths(paths []string) error {
	if len(paths) > maxCapturePaths {
		return fmt.Errorf("%w: %d capture paths, max %d", ErrInvalidRequest, len(paths), maxCapturePaths)
	}
	for _, path := range paths {
		if path == "" || len(path) > maxCapturePathBytes {
			return fmt.Errorf("%w: capture path length %d", ErrInvalidRequest, len(path))
		}
	}
	return nil
}
