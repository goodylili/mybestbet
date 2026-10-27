package browserd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"mybestbet/pkg/sources"
)

type Options struct {
	ProfileRoot string
	Size        int
	ChromeBin   string
}

type Pool struct {
	opts  Options
	log   *slog.Logger
	slots chan struct{}

	mu       sync.Mutex
	profiles map[string]*profile
	isClosed bool
}

func NewPool(opts Options, log *slog.Logger) *Pool {
	if opts.Size < 1 {
		opts.Size = 1
	}
	return &Pool{opts: opts, log: log, slots: make(chan struct{}, opts.Size), profiles: map[string]*profile{}}
}

func (p *Pool) Fetch(ctx context.Context, req sources.BrowserRequest) (sources.BrowserResult, error) {
	started := time.Now()
	result, err := p.fetchInSlot(ctx, req)
	recordFetch(req.Profile, started, err)
	return result, err
}

func (p *Pool) fetchInSlot(ctx context.Context, req sources.BrowserRequest) (sources.BrowserResult, error) {
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return sources.BrowserResult{}, fmt.Errorf("wait for browser slot: %w", ctx.Err())
	}
	defer func() { <-p.slots }()

	prof, err := p.acquireProfile(req.Profile)
	if err != nil {
		return sources.BrowserResult{}, err
	}
	defer p.releaseProfile(prof)
	return fetchWithProfile(ctx, prof, req)
}

func (p *Pool) acquireProfile(name string) (*profile, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isClosed {
		return nil, ErrPoolClosed
	}
	prof, ok := p.profiles[name]
	if !ok {
		p.evictIdleProfileIfFull()
		launched, err := launchProfile(name, p.opts.ProfileRoot, p.opts.ChromeBin)
		if err != nil {
			return nil, err
		}
		p.log.Info("browser profile launched", "profile", name)
		prof = launched
		p.profiles[name] = prof
	}
	prof.activeFetches++
	prof.lastUsed = time.Now()
	return prof, nil
}

func (p *Pool) releaseProfile(prof *profile) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prof.activeFetches--
}

func (p *Pool) evictIdleProfileIfFull() {
	if len(p.profiles) < p.opts.Size {
		return
	}
	var victim *profile
	for _, prof := range p.profiles {
		if prof.activeFetches == 0 && (victim == nil || prof.lastUsed.Before(victim.lastUsed)) {
			victim = prof
		}
	}
	if victim == nil {
		return
	}
	delete(p.profiles, victim.name)
	victim.close()
	p.log.Info("browser profile evicted", "profile", victim.name)
}

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isClosed = true
	for name, prof := range p.profiles {
		prof.close()
		delete(p.profiles, name)
	}
}
