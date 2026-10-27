package engine

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"mybestbet/pkg/canon"
)

const (
	spoolSegmentSuffix   = ".seg"
	spoolActiveSuffix    = ".open"
	spoolSegmentMaxRows  = 200_000
	spoolDirPermissions  = 0o750
	spoolFilePermissions = 0o600
)

type spooledRow struct {
	MarketKey       string
	EventID         string
	Code            canon.MarketCode
	Line            *float64
	PlayerID        string
	Period          string
	Cell            canon.BoardCell
	Rules           canon.SettleRule
	RawChanged      bool
	SourceMarketKey string
}

func spooledFrom(r priceRow) spooledRow {
	return spooledRow{MarketKey: r.marketKey, EventID: r.eventID, Code: r.code, Line: r.line, PlayerID: r.playerID, Period: r.period, Cell: r.cell, Rules: r.rules, RawChanged: r.isRawChanged, SourceMarketKey: r.sourceMarketKey}
}

func (s spooledRow) priceRow() priceRow {
	return priceRow{marketKey: s.MarketKey, eventID: s.EventID, code: s.Code, line: s.Line, playerID: s.PlayerID, period: s.Period, cell: s.Cell, rules: s.Rules, isRawChanged: s.RawChanged, sourceMarketKey: s.SourceMarketKey}
}

type spoolSegment struct {
	file    *os.File
	buf     *bufio.Writer
	encoder *gob.Encoder
	rows    int
}

type spool struct {
	dir string

	mu       sync.Mutex
	active   *spoolSegment
	sealed   []string
	claimed  map[string]bool
	nextSeq  int
	isClosed bool
}

func openSpool(dir string) (*spool, error) {
	if err := os.MkdirAll(dir, spoolDirPermissions); err != nil {
		return nil, fmt.Errorf("create spool dir %s: %w", dir, err)
	}
	s := &spool{dir: dir, claimed: map[string]bool{}}
	if err := s.adoptLeftoverSegments(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *spool) adoptLeftoverSegments() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read spool dir %s: %w", s.dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(s.dir, name)
		if strings.HasSuffix(name, spoolActiveSuffix) {
			sealedPath := strings.TrimSuffix(path, spoolActiveSuffix) + spoolSegmentSuffix
			if err := os.Rename(path, sealedPath); err != nil {
				return fmt.Errorf("seal leftover spool segment %s: %w", name, err)
			}
			path, name = sealedPath, filepath.Base(sealedPath)
		}
		if seq, ok := segmentSeq(name); ok {
			s.sealed = append(s.sealed, path)
			s.nextSeq = max(s.nextSeq, seq+1)
		}
	}
	slices.Sort(s.sealed)
	return nil
}

func segmentSeq(name string) (int, bool) {
	stem, ok := strings.CutSuffix(name, spoolSegmentSuffix)
	if !ok {
		return 0, false
	}
	seq, err := strconv.Atoi(stem)
	return seq, err == nil
}

func (s *spool) segmentPath(seq int, suffix string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%012d%s", seq, suffix))
}

func (s *spool) append(rows []priceRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isClosed {
		return errors.New("spool closed")
	}
	if s.active == nil {
		if err := s.openActiveSegment(); err != nil {
			return err
		}
	}
	batch := make([]spooledRow, len(rows))
	for i, r := range rows {
		batch[i] = spooledFrom(r)
	}
	if err := s.active.encoder.Encode(batch); err != nil {
		return fmt.Errorf("encode spool rows: %w", err)
	}
	s.active.rows += len(rows)
	if s.active.rows >= spoolSegmentMaxRows {
		return s.sealActiveSegment()
	}
	return nil
}

func (s *spool) openActiveSegment() error {
	path := s.segmentPath(s.nextSeq, spoolActiveSuffix)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, spoolFilePermissions)
	if err != nil {
		return fmt.Errorf("open spool segment %s: %w", path, err)
	}
	buf := bufio.NewWriter(f)
	s.active = &spoolSegment{file: f, buf: buf, encoder: gob.NewEncoder(buf)}
	s.nextSeq++
	return nil
}

func (s *spool) sealActiveSegment() error {
	seg := s.active
	s.active = nil
	if err := seg.buf.Flush(); err != nil {
		return fmt.Errorf("flush spool segment: %w", err)
	}
	if err := seg.file.Close(); err != nil {
		return fmt.Errorf("close spool segment: %w", err)
	}
	sealedPath := strings.TrimSuffix(seg.file.Name(), spoolActiveSuffix) + spoolSegmentSuffix
	if err := os.Rename(seg.file.Name(), sealedPath); err != nil {
		return fmt.Errorf("seal spool segment: %w", err)
	}
	s.sealed = append(s.sealed, sealedPath)
	return nil
}

func (s *spool) claimOldest() (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sealed) == 0 && s.active != nil && s.active.rows > 0 {
		if err := s.sealActiveSegment(); err != nil {
			return "", false, err
		}
	}
	for _, path := range s.sealed {
		if !s.claimed[path] {
			s.claimed[path] = true
			return path, true, nil
		}
	}
	return "", false, nil
}

func (s *spool) release(path string, isWritten bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claimed, path)
	if !isWritten {
		return nil
	}
	s.sealed = slices.DeleteFunc(s.sealed, func(p string) bool { return p == path })
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove drained spool segment %s: %w", path, err)
	}
	return nil
}

func (s *spool) pendingSegments() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.sealed)
	if s.active != nil && s.active.rows > 0 {
		n++
	}
	return n
}

func (s *spool) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isClosed = true
	if s.active == nil {
		return nil
	}
	return s.sealActiveSegment()
}

func readSpoolSegment(path string) ([]priceRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open spool segment %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	decoder := gob.NewDecoder(bufio.NewReader(f))
	var rows []priceRow
	for {
		var batch []spooledRow
		err := decoder.Decode(&batch)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return rows, nil
		}
		if err != nil {
			return rows, fmt.Errorf("decode spool segment %s: %w", path, err)
		}
		for _, r := range batch {
			rows = append(rows, r.priceRow())
		}
	}
}
