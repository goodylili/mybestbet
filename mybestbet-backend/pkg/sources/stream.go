package sources

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"mybestbet/pkg/canon"
)

const (
	StreamReconcileInterval = 5 * time.Minute
	streamInitialBackoff    = time.Second
	streamMaxBackoff        = 30 * time.Second
	streamReadLimit         = 8 << 20
)

type FrameApplier func(ctx context.Context, conn *websocket.Conn, frame []byte, now time.Time) ([]canon.RawMarket, error)

type StreamSession struct {
	Resync    canon.Snapshot
	URL       string
	Header    http.Header
	Subscribe func(ctx context.Context, conn *websocket.Conn) error
	Apply     FrameApplier
	KeepAlive func(ctx context.Context, conn *websocket.Conn)
}

type SessionOpener func(ctx context.Context) (StreamSession, error)

func (d Deps) RunStream(ctx context.Context, source string, out chan<- canon.Snapshot, open SessionOpener) error {
	backoff := streamInitialBackoff
	for {
		err := d.runReconcilingSession(ctx, source, out, open)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			backoff = streamInitialBackoff
			continue
		}
		d.Log().Warn("stream session failed", "source", source, "error", err, "retry_in", backoff)
		if err := Sleep(ctx, backoff); err != nil {
			return err
		}
		backoff = min(backoff*2, streamMaxBackoff)
	}
}

func (d Deps) runReconcilingSession(ctx context.Context, source string, out chan<- canon.Snapshot, open SessionOpener) error {
	sessionCtx, cancel := context.WithTimeout(ctx, StreamReconcileInterval)
	defer cancel()
	err := d.streamUntilClosed(sessionCtx, source, out, open)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if sessionCtx.Err() != nil {
		return nil
	}
	return err
}

func (d Deps) streamUntilClosed(ctx context.Context, source string, out chan<- canon.Snapshot, open SessionOpener) error {
	session, err := open(ctx)
	if err != nil {
		return fmt.Errorf("resync from rest: %w", err)
	}
	if err := Emit(ctx, out, session.Resync); err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, session.URL, &websocket.DialOptions{HTTPClient: d.HTTP, HTTPHeader: session.Header})
	if err != nil {
		return fmt.Errorf("dial stream: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(streamReadLimit)
	if err := session.Subscribe(ctx, conn); err != nil {
		return err
	}
	if session.KeepAlive != nil {
		pingCtx, stopPing := context.WithCancel(ctx)
		var pinger sync.WaitGroup
		pinger.Go(func() { session.KeepAlive(pingCtx, conn) })
		defer func() {
			stopPing()
			pinger.Wait()
		}()
	}
	return d.relayFrames(ctx, source, conn, session.Apply, out)
}

func (d Deps) relayFrames(ctx context.Context, source string, conn *websocket.Conn, apply FrameApplier, out chan<- canon.Snapshot) error {
	for {
		_, frame, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read stream: %w", err)
		}
		now := d.Clock()
		changed, err := apply(ctx, conn, frame, now)
		if err != nil {
			d.Log().Debug("stream frame skipped", "source", source, "error", err)
			continue
		}
		if len(changed) == 0 {
			continue
		}
		snap := NewSnapshot(source, now)
		snap.Markets = changed
		if err := Emit(ctx, out, snap); err != nil {
			return err
		}
	}
}

func Emit(ctx context.Context, out chan<- canon.Snapshot, snap canon.Snapshot) error {
	select {
	case out <- snap:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d Deps) ExtraOr(key, fallback string) string {
	if v := d.Config.Extra[key]; v != "" {
		return v
	}
	return fallback
}
