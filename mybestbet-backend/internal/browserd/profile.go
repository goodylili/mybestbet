package browserd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

const maxPagesPerProfile = 8

type openPage struct {
	page     *rod.Page
	lastUsed time.Time
}

type profile struct {
	name      string
	launcher  *launcher.Launcher
	browser   *rod.Browser
	userAgent string

	mu             sync.Mutex
	pages          map[string]*openPage
	lastNavigation time.Time

	activeFetches int
	lastUsed      time.Time
}

func launchProfile(name, profileRoot, chromeBin string) (*profile, error) {
	dir := filepath.Join(profileRoot, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create profile dir: %w", err)
	}
	l := launcher.New().
		UserDataDir(dir).
		Headless(true).
		Set("disable-blink-features", "AutomationControlled").
		Set("window-size", "1366,900")
	if chromeBin == "" {
		chromeBin, _ = launcher.LookPath()
	}
	if chromeBin != "" {
		l = l.Bin(chromeBin)
	}
	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch chrome for profile %s: %w", name, err)
	}
	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		l.Kill()
		return nil, fmt.Errorf("connect chrome for profile %s: %w", name, err)
	}
	version, err := proto.BrowserGetVersion{}.Call(browser)
	if err != nil {
		_ = browser.Close()
		l.Kill()
		return nil, fmt.Errorf("read chrome version for profile %s: %w", name, err)
	}
	return &profile{
		name:      name,
		launcher:  l,
		browser:   browser,
		userAgent: strings.Replace(version.UserAgent, "HeadlessChrome", "Chrome", 1),
		pages:     map[string]*openPage{},
	}, nil
}

func (p *profile) pageFor(url string, now time.Time) (page *rod.Page, isReused bool, err error) {
	if open, ok := p.pages[url]; ok {
		open.lastUsed = now
		return open.page, true, nil
	}
	p.evictOldestPageIfFull()
	page, err = stealth.Page(p.browser)
	if err != nil {
		return nil, false, fmt.Errorf("open stealth page: %w", err)
	}
	if err := (proto.NetworkSetUserAgentOverride{UserAgent: p.userAgent, AcceptLanguage: "en-GB,en"}).Call(page); err != nil {
		_ = page.Close()
		return nil, false, fmt.Errorf("override user agent: %w", err)
	}
	p.pages[url] = &openPage{page: page, lastUsed: now}
	return page, false, nil
}

func (p *profile) evictOldestPageIfFull() {
	if len(p.pages) < maxPagesPerProfile {
		return
	}
	var oldestURL string
	var oldest time.Time
	for url, open := range p.pages {
		if oldestURL == "" || open.lastUsed.Before(oldest) {
			oldestURL, oldest = url, open.lastUsed
		}
	}
	p.dropPage(oldestURL)
}

func (p *profile) dropPage(url string) {
	if open, ok := p.pages[url]; ok {
		_ = open.page.Close()
		delete(p.pages, url)
	}
}

func (p *profile) close() {
	_ = p.browser.Close()
	p.launcher.Kill()
}
