package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultLedgerMaxEntries = 20000
	defaultLedgerRetention  = 30 * 24 * time.Hour
)

type fileCompletionLedger struct {
	mu            sync.Mutex
	path          string
	now           func() time.Time
	maxEntries    int
	retention     time.Duration
	completedByID map[string]time.Time
}

type completionLedgerFile struct {
	Version int                     `json:"version"`
	Entries []completionLedgerEntry `json:"entries"`
}

type completionLedgerEntry struct {
	MessageID   string `json:"message_id"`
	CompletedAt string `json:"completed_at"`
}

func newFileCompletionLedger(path string) (*fileCompletionLedger, error) {
	return newFileCompletionLedgerWithOptions(path, time.Now, defaultLedgerMaxEntries, defaultLedgerRetention)
}

func newFileCompletionLedgerWithOptions(path string, now func() time.Time, maxEntries int, retention time.Duration) (*fileCompletionLedger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("empty dedupe file path")
	}
	if now == nil {
		return nil, errors.New("nil clock")
	}
	if maxEntries <= 0 {
		return nil, fmt.Errorf("invalid max entries: %d", maxEntries)
	}
	if retention <= 0 {
		return nil, fmt.Errorf("invalid retention: %s", retention)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create ledger directory: %w", err)
	}

	ledger := &fileCompletionLedger{
		path:          path,
		now:           now,
		maxEntries:    maxEntries,
		retention:     retention,
		completedByID: make(map[string]time.Time),
	}

	if err := ledger.loadOrInitialize(); err != nil {
		return nil, err
	}
	return ledger, nil
}

func (l *fileCompletionLedger) IsComplete(messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	completedAt, ok := l.completedByID[messageID]
	if !ok {
		return false
	}
	if completedAt.Before(l.cutoff()) {
		delete(l.completedByID, messageID)
		return false
	}
	return true
}

func (l *fileCompletionLedger) MarkComplete(messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("empty message_id")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	next := cloneCompletionMap(l.completedByID)
	next[messageID] = l.now().UTC()
	next = l.pruneMap(next)
	if err := l.persistMap(next); err != nil {
		return err
	}
	l.completedByID = next
	return nil
}

func (l *fileCompletionLedger) loadOrInitialize() error {
	info, err := os.Stat(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return l.persistMap(l.completedByID)
		}
		return fmt.Errorf("stat ledger file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("ledger path is not a regular file: %s", l.path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("ledger file permissions too broad: %s (%#o)", l.path, info.Mode().Perm())
	}

	if err := l.loadLocked(); err != nil {
		return err
	}

	if l.pruneLocked() {
		if err := l.persistMap(l.completedByID); err != nil {
			return fmt.Errorf("persist pruned ledger: %w", err)
		}
	}

	return nil
}

func (l *fileCompletionLedger) loadLocked() error {
	raw, err := os.ReadFile(l.path)
	if err != nil {
		return fmt.Errorf("read ledger file: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var file completionLedgerFile
	if err := dec.Decode(&file); err != nil {
		return fmt.Errorf("decode ledger file: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode ledger file: trailing data")
	}
	if file.Version != 1 {
		return fmt.Errorf("unsupported ledger version: %d", file.Version)
	}

	loaded := make(map[string]time.Time, len(file.Entries))
	for i, e := range file.Entries {
		id := strings.TrimSpace(e.MessageID)
		if id == "" {
			return fmt.Errorf("ledger entry %d has empty message_id", i)
		}
		if _, exists := loaded[id]; exists {
			return fmt.Errorf("ledger has duplicate message_id %q", id)
		}
		ts, err := time.Parse(time.RFC3339Nano, e.CompletedAt)
		if err != nil {
			return fmt.Errorf("ledger entry %d invalid completed_at: %w", i, err)
		}
		loaded[id] = ts.UTC()
	}

	l.completedByID = loaded
	return nil
}

func (l *fileCompletionLedger) pruneLocked() bool {
	pruned := l.pruneMap(l.completedByID)
	if len(pruned) == len(l.completedByID) {
		same := true
		for id, ts := range l.completedByID {
			if pruned[id] != ts {
				same = false
				break
			}
		}
		if same {
			return false
		}
	}
	l.completedByID = pruned
	return true
}

func (l *fileCompletionLedger) pruneMap(entries map[string]time.Time) map[string]time.Time {
	cutoff := l.cutoff()
	for id, ts := range entries {
		if ts.Before(cutoff) {
			delete(entries, id)
		}
	}

	if len(entries) <= l.maxEntries {
		return entries
	}

	type item struct {
		id string
		ts time.Time
	}
	items := make([]item, 0, len(entries))
	for id, ts := range entries {
		items = append(items, item{id: id, ts: ts})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ts.Equal(items[j].ts) {
			return items[i].id < items[j].id
		}
		return items[i].ts.After(items[j].ts)
	})

	keep := make(map[string]time.Time, l.maxEntries)
	for i := 0; i < l.maxEntries; i++ {
		keep[items[i].id] = items[i].ts
	}
	return keep
}

func (l *fileCompletionLedger) persistMap(completedByID map[string]time.Time) error {
	entries := make([]completionLedgerEntry, 0, len(completedByID))
	for id, ts := range completedByID {
		entries = append(entries, completionLedgerEntry{
			MessageID:   id,
			CompletedAt: ts.UTC().Format(time.RFC3339Nano),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CompletedAt == entries[j].CompletedAt {
			return entries[i].MessageID < entries[j].MessageID
		}
		return entries[i].CompletedAt < entries[j].CompletedAt
	})

	payload, err := json.Marshal(completionLedgerFile{Version: 1, Entries: entries})
	if err != nil {
		return fmt.Errorf("marshal ledger: %w", err)
	}
	payload = append(payload, '\n')

	dir := filepath.Dir(l.path)
	tmp, err := os.CreateTemp(dir, ".completed.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create temp ledger file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp ledger file: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp ledger file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp ledger file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp ledger file: %w", err)
	}

	if err := os.Rename(tmpPath, l.path); err != nil {
		return fmt.Errorf("rename ledger file: %w", err)
	}
	cleanup = false

	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync ledger directory: %w", err)
	}
	return nil
}

func (l *fileCompletionLedger) cutoff() time.Time {
	return l.now().UTC().Add(-l.retention)
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Sync(); err != nil {
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
			return nil
		}
		return err
	}
	return nil
}

func cloneCompletionMap(src map[string]time.Time) map[string]time.Time {
	dst := make(map[string]time.Time, len(src))
	for id, ts := range src {
		dst[id] = ts
	}
	return dst
}
