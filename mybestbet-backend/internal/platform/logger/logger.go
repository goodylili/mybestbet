package logger

import (
	"log/slog"
	"os"
)

func New(service, env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if env == "dev" {
		opts.Level = slog.LevelDebug
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	log := slog.New(handler).With("service", service)
	slog.SetDefault(log)
	return log
}
