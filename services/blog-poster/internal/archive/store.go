package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"HerbHub365/services/blog-poster/internal/model"
)

type Store struct {
	baseDir string
	mu      sync.Mutex
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func (s *Store) Append(snapshot model.Snapshot, body []byte) error {
	day := snapshotDay(snapshot)
	path := s.pathForDay(day)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open archive file: %w", err)
	}
	defer file.Close()

	compactBody, err := compactJSON(body)
	if err != nil {
		return err
	}

	if _, err := file.Write(append(compactBody, '\n')); err != nil {
		return fmt.Errorf("append archive file: %w", err)
	}

	return nil
}

func (s *Store) Load(day time.Time) ([]model.Snapshot, error) {
	path := s.pathForDay(day)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open archive file: %w", err)
	}
	defer file.Close()

	seen := make(map[string]struct{})
	var snapshots []model.Snapshot
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var snapshot model.Snapshot
		if err := decoder.Decode(&snapshot); err != nil {
			return nil, fmt.Errorf("decode archived snapshot: %w", err)
		}

		key := snapshot.MessageID
		if key == "" {
			key = snapshot.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + snapshot.Device
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		snapshots = append(snapshots, snapshot)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.Before(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

func (s *Store) pathForDay(day time.Time) string {
	return filepath.Join(s.baseDir, "snapshots", day.UTC().Format("2006-01-02")+".jsonl")
}

func snapshotDay(snapshot model.Snapshot) time.Time {
	timestamp := snapshot.Timestamp.UTC()
	if timestamp.IsZero() && snapshot.CollectedAt != nil {
		timestamp = snapshot.CollectedAt.UTC()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, time.UTC)
}

func compactJSON(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty snapshot body")
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return nil, fmt.Errorf("compact snapshot json: %w", err)
	}

	return compact.Bytes(), nil
}
