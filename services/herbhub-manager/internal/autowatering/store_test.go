package autowatering

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStoreInitializeAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auto.json")
	cfg := DefaultConfig()

	st, err := NewStore(Bootstrap{Path: path, Config: cfg, Actor: "bootstrap"})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	doc := st.Snapshot()
	if doc.ConfigRevision != 1 {
		t.Fatalf("revision=%d", doc.ConfigRevision)
	}
	_ = st.Close()

	st2, err := NewStore(Bootstrap{Path: path, Config: cfg, Actor: "ignored"})
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	defer st2.Close()
	if st2.Snapshot().ConfigRevision != 1 {
		t.Fatalf("revision changed unexpectedly")
	}
}

func TestStoreConfigReplaceRevisionRules(t *testing.T) {
	st := newTestStore(t, DefaultConfig())
	cfg := st.Snapshot().Config
	cfg.Enabled = true
	if _, err := st.ReplaceConfig(999, cfg, "alice"); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("expected revision mismatch, got %v", err)
	}
	doc, err := st.ReplaceConfig(1, cfg, "alice")
	if err != nil {
		t.Fatalf("replace config: %v", err)
	}
	if doc.ConfigRevision != 2 {
		t.Fatalf("expected revision 2 got %d", doc.ConfigRevision)
	}
	before := doc.ConfigRevision
	_, err = st.UpdateRuntime(func(doc *Document) error {
		v := doc.Plants[PlantBasil]
		now := time.Now().UTC()
		v.LastEvaluatedAt = &now
		doc.Plants[PlantBasil] = v
		return nil
	})
	if err != nil {
		t.Fatalf("runtime update: %v", err)
	}
	if st.Snapshot().ConfigRevision != before {
		t.Fatalf("runtime update must not change config revision")
	}
}

func TestStoreRejectsCorruptVersionAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auto.json")

	if err := os.WriteFile(path, []byte(`{"version":999}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := NewStore(Bootstrap{Path: path, Config: DefaultConfig(), Actor: "test"}); !errors.Is(err, ErrStateVersion) {
		t.Fatalf("expected version error, got %v", err)
	}

	good := DefaultDocument(time.Now().UTC(), "test", DefaultConfig())
	buf, _ := json.Marshal(good)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := NewStore(Bootstrap{Path: path, Config: DefaultConfig(), Actor: "test"}); !errors.Is(err, ErrStatePermissions) {
		t.Fatalf("expected permissions error, got %v", err)
	}
}

func TestStoreLockConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auto.json")
	st, err := NewStore(Bootstrap{Path: path, Config: DefaultConfig(), Actor: "one"})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()
	if _, err := NewStore(Bootstrap{Path: path, Config: DefaultConfig(), Actor: "two"}); !errors.Is(err, ErrStateLockConflict) {
		t.Fatalf("expected lock conflict got %v", err)
	}
}

func TestStoreConcurrentRuntimeUpdates(t *testing.T) {
	st := newTestStore(t, DefaultConfig())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = st.UpdateRuntime(func(doc *Document) error {
				p := doc.Plants[PlantBasil]
				now := time.Now().UTC()
				p.LastEvaluatedAt = &now
				doc.Plants[PlantBasil] = p
				return nil
			})
		}()
	}
	wg.Wait()
	if st.Snapshot().Plants[PlantBasil].LastEvaluatedAt == nil {
		t.Fatalf("expected runtime updates")
	}
}
