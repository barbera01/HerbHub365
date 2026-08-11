package autowatering

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrStoreFaulted      = errors.New("autowatering: store faulted")
	ErrRevisionMismatch  = errors.New("autowatering: revision mismatch")
	ErrUnknownPlant      = errors.New("autowatering: unknown plant")
	ErrInvalidStateFile  = errors.New("autowatering: invalid state file")
	ErrStatePermissions  = errors.New("autowatering: insecure state file permissions")
	ErrStateVersion      = errors.New("autowatering: unsupported state version")
	ErrStateLockConflict = errors.New("autowatering: state lock already held")
)

type Store struct {
	mu       sync.Mutex
	fenceMu  sync.Mutex
	path     string
	lockPath string
	lockFile *os.File
	now      func() time.Time

	doc      Document
	faulted  bool
	faultMsg string

	changes chan struct{}
}

type Bootstrap struct {
	Path   string
	Config Config
	Actor  string
}

func NewStore(b Bootstrap) (*Store, error) {
	path := strings.TrimSpace(b.Path)
	if path == "" {
		path = DefaultStatePath
	}
	if err := ValidateConfig(b.Config); err != nil {
		return nil, err
	}
	st := &Store{
		path:     path,
		lockPath: path + ".lock",
		now:      time.Now,
		changes:  make(chan struct{}, 1),
	}
	if err := st.acquireLock(); err != nil {
		return nil, err
	}
	if err := st.loadOrCreate(b.Config, b.Actor); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) acquireLock() error {
	lf, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lf.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return ErrStateLockConflict
		}
		return fmt.Errorf("flock: %w", err)
	}
	s.lockFile = lf
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockFile == nil {
		return nil
	}
	err := s.lockFile.Close()
	s.lockFile = nil
	return err
}

func (s *Store) Fault() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.faulted, s.faultMsg
}

func (s *Store) Snapshot() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneDocument(s.doc)
}

func (s *Store) Changes() <-chan struct{} {
	return s.changes
}

func (s *Store) ReplaceConfig(expectedRevision int64, cfg Config, actor string) (Document, error) {
	s.fenceMu.Lock()
	defer s.fenceMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.faulted {
		return Document{}, ErrStoreFaulted
	}
	if err := ValidateConfig(cfg); err != nil {
		return Document{}, err
	}
	if expectedRevision != s.doc.ConfigRevision {
		return Document{}, ErrRevisionMismatch
	}
	if strings.TrimSpace(actor) == "" {
		actor = "system"
	}

	next := cloneDocument(s.doc)
	next.Config = cfg
	next.ConfigRevision++
	next.ConfigUpdatedAt = s.now().UTC()
	next.ConfigUpdatedBy = actor

	if err := s.persistLocked(next); err != nil {
		s.fault("persist config: " + err.Error())
		return Document{}, err
	}
	s.doc = next
	s.signalChange()
	return cloneDocument(s.doc), nil
}

func (s *Store) WithPublicationFence(fn func() error) error {
	s.fenceMu.Lock()
	defer s.fenceMu.Unlock()
	if fn == nil {
		return nil
	}
	return fn()
}

func (s *Store) UpdateRuntime(mutator func(*Document) error) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.faulted {
		return Document{}, ErrStoreFaulted
	}
	next := cloneDocument(s.doc)
	if err := mutator(&next); err != nil {
		return Document{}, err
	}
	if err := s.persistLocked(next); err != nil {
		s.fault("persist runtime: " + err.Error())
		return Document{}, err
	}
	s.doc = next
	return cloneDocument(s.doc), nil
}

func (s *Store) loadOrCreate(initial Config, actor string) error {
	st, err := os.Stat(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat state file: %w", err)
		}
		doc := DefaultDocument(s.now(), actor, initial)
		if err := s.persistLocked(doc); err != nil {
			return fmt.Errorf("write initial state: %w", err)
		}
		s.doc = doc
		return nil
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%w: state path is not regular file", ErrInvalidStateFile)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return ErrStatePermissions
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read state file: %w", err)
	}
	doc, err := decodeDocument(raw)
	if err != nil {
		return err
	}
	s.doc = doc
	return nil
}

func decodeDocument(raw []byte) (Document, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("%w: decode: %v", ErrInvalidStateFile, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Document{}, fmt.Errorf("%w: trailing json", ErrInvalidStateFile)
	}
	if doc.Version != StateVersion {
		return Document{}, ErrStateVersion
	}
	if doc.ConfigRevision < 1 {
		return Document{}, fmt.Errorf("%w: config_revision must be >= 1", ErrInvalidStateFile)
	}
	if strings.TrimSpace(doc.ConfigUpdatedBy) == "" {
		return Document{}, fmt.Errorf("%w: config_updated_by required", ErrInvalidStateFile)
	}
	if doc.ConfigUpdatedAt.IsZero() {
		return Document{}, fmt.Errorf("%w: config_updated_at required", ErrInvalidStateFile)
	}
	if err := ValidateConfig(doc.Config); err != nil {
		return Document{}, err
	}
	if doc.Plants == nil {
		doc.Plants = map[string]PlantRuntimeState{}
	}
	for name := range doc.Plants {
		if !isFixedPlant(name) {
			return Document{}, fmt.Errorf("%w: unknown runtime plant %s", ErrInvalidStateFile, name)
		}
	}
	for _, plant := range fixedPlants {
		if _, ok := doc.Plants[plant]; !ok {
			doc.Plants[plant] = PlantRuntimeState{}
		}
	}
	return doc, nil
}

func (s *Store) persistLocked(doc Document) error {
	if doc.Version == 0 {
		doc.Version = StateVersion
	}
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	buf = append(buf, '\n')

	dir := filepath.Dir(s.path)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("state directory unavailable: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".automatic-watering-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp state: %w", err)
	}
	if info, err := tmp.Stat(); err != nil {
		cleanup()
		return fmt.Errorf("stat temp state: %w", err)
	} else if info.Mode().Perm()&0o077 != 0 {
		cleanup()
		return ErrStatePermissions
	}
	if _, err := tmp.Write(buf); err != nil {
		cleanup()
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("fsync temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp state: %w", err)
	}

	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp state: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod state: %w", err)
	}
	df, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open state dir: %w", err)
	}
	defer df.Close()
	if err := df.Sync(); err != nil {
		return fmt.Errorf("fsync state dir: %w", err)
	}
	return nil
}

func (s *Store) fault(message string) {
	s.faulted = true
	s.faultMsg = strings.TrimSpace(message)
	if s.faultMsg == "" {
		s.faultMsg = "store faulted"
	}
}

func (s *Store) signalChange() {
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

func cloneDocument(in Document) Document {
	out := in
	out.Config = cloneConfig(in.Config)
	out.Plants = map[string]PlantRuntimeState{}
	for k, v := range in.Plants {
		out.Plants[k] = v
	}
	return out
}

func cloneConfig(in Config) Config {
	out := in
	out.Plants = map[string]PlantConfig{}
	for k, v := range in.Plants {
		out.Plants[k] = v
	}
	return out
}
