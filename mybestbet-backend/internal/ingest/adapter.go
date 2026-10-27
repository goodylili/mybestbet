package ingest

import (
	"fmt"
	"log/slog"

	"mybestbet/pkg/sources"
)

func NewAdapter(name string, sc sources.Config, newBrowser BrowserFactory, log *slog.Logger) (sources.Adapter, error) {
	factory, registered := sources.Lookup(name)
	if !registered {
		return nil, fmt.Errorf("source %s: %w", name, sources.ErrNotFound)
	}
	return buildAdapter(name, sc, factory, newBrowser, log)
}

func buildAdapter(name string, sc sources.Config, factory sources.Factory, newBrowser BrowserFactory, log *slog.Logger) (sources.Adapter, error) {
	client, err := newSourceHTTPClient(sc)
	if err != nil {
		return nil, fmt.Errorf("source %s: %w", name, err)
	}
	deps := sources.Deps{
		HTTP:    client,
		Limiter: newLimiter(sc.MaxRPS),
		Config:  sc,
		Logger:  log.With("source", name),
	}
	adapter := factory(deps)
	if adapter.Capabilities().NeedsBrowser && newBrowser != nil {
		deps.Browser = newBrowser()
		adapter = factory(deps)
	}
	return adapter, nil
}
