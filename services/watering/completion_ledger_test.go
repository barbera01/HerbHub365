package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileCompletionLedgerRoundTripReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completed.json")

	baseNow := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	now := baseNow
	ledger, err := newFileCompletionLedgerWithOptions(path, func() time.Time { return now }, 20000, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}

	if err := ledger.MarkComplete("id-1"); err != nil {
		t.Fatalf("mark id-1: %v", err)
	}
	now = now.Add(1 * time.Second)
	if err := ledger.MarkComplete("id-2"); err != nil {
		t.Fatalf("mark id-2: %v", err)
	}

	reloaded, err := newFileCompletionLedgerWithOptions(path, func() time.Time { return now }, 20000, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("reload ledger: %v", err)
	}

	if !reloaded.IsComplete("id-1") || !reloaded.IsComplete("id-2") {
		t.Fatalf("expected both ids complete after reload")
	}
}

func TestFileCompletionLedgerPrunesByAgeAndMaxEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completed.json")

	baseNow := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	now := baseNow
	ledger, err := newFileCompletionLedgerWithOptions(path, func() time.Time { return now }, 2, 10*time.Second)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}

	if err := ledger.MarkComplete("old"); err != nil {
		t.Fatalf("mark old: %v", err)
	}
	now = now.Add(6 * time.Second)
	if err := ledger.MarkComplete("mid"); err != nil {
		t.Fatalf("mark mid: %v", err)
	}
	now = now.Add(6 * time.Second)
	if err := ledger.MarkComplete("new"); err != nil {
		t.Fatalf("mark new: %v", err)
	}

	if ledger.IsComplete("old") {
		t.Fatalf("expected old entry pruned by retention")
	}
	if !ledger.IsComplete("mid") || !ledger.IsComplete("new") {
		t.Fatalf("expected mid/new to remain complete")
	}

	now = now.Add(1 * time.Second)
	if err := ledger.MarkComplete("newer"); err != nil {
		t.Fatalf("mark newer: %v", err)
	}

	if ledger.IsComplete("mid") {
		t.Fatalf("expected mid pruned by max entries")
	}
	if !ledger.IsComplete("new") || !ledger.IsComplete("newer") {
		t.Fatalf("expected newest entries retained")
	}
}

func TestFileCompletionLedgerStrictLoadFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completed.json")

	bad := `{"version":1,"entries":[{"message_id":"id-1","completed_at":"2026-08-11T12:00:00Z","extra":true}]}`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatalf("write bad ledger: %v", err)
	}

	_, err := newFileCompletionLedgerWithOptions(path, time.Now, 20000, 30*24*time.Hour)
	if err == nil {
		t.Fatalf("expected strict load failure")
	}
	if !strings.Contains(err.Error(), "decode ledger file") {
		t.Fatalf("expected decode ledger file error, got %v", err)
	}
}

func TestFileCompletionLedgerRejectsBroadPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completed.json")

	good := `{"version":1,"entries":[]}`
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	_, err := newFileCompletionLedgerWithOptions(path, time.Now, 20000, 30*24*time.Hour)
	if err == nil {
		t.Fatalf("expected permission failure")
	}
	if !strings.Contains(err.Error(), "permissions too broad") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileCompletionLedgerAtomicPersistFailureDoesNotCorruptExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completed.json")

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	ledger, err := newFileCompletionLedgerWithOptions(path, func() time.Time { return now }, 20000, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	if err := ledger.MarkComplete("first"); err != nil {
		t.Fatalf("mark first: %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})

	err = ledger.MarkComplete("second")
	if err == nil {
		t.Fatalf("expected persist failure")
	}

	reloaded, loadErr := newFileCompletionLedgerWithOptions(path, func() time.Time { return now }, 20000, 30*24*time.Hour)
	if loadErr != nil {
		t.Fatalf("reload after persist failure: %v", loadErr)
	}
	if !reloaded.IsComplete("first") {
		t.Fatalf("expected first entry retained")
	}
	if reloaded.IsComplete("second") {
		t.Fatalf("expected second entry absent after failed persist")
	}
}

func TestSyncDirIgnoresUnsupported(t *testing.T) {
	// Basic guard that helper rejects invalid path with a concrete error type.
	err := syncDir(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatalf("expected syncDir error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
