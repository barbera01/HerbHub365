package autowatering

import "testing"

func TestParseIfMatchRevision(t *testing.T) {
	if _, err := ParseIfMatchRevision(""); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := ParseIfMatchRevision("abc"); err == nil {
		t.Fatalf("expected parse error")
	}
	for _, bad := range []string{"1", `W/"1"`, `"*"`, `"1","2"`, `"01"`, `"\"1\""`, `"1""`} {
		if _, err := ParseIfMatchRevision(bad); err == nil {
			t.Fatalf("expected invalid If-Match for %q", bad)
		}
	}
	for _, overflow := range []string{`"9223372036854775808"`, `"18446744073709551616"`, `"999999999999999999999999999999"`} {
		if _, err := ParseIfMatchRevision(overflow); err == nil {
			t.Fatalf("expected overflow rejection for %q", overflow)
		}
	}
	rev, err := ParseIfMatchRevision(`"12"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rev != 12 {
		t.Fatalf("rev=%d", rev)
	}
}

func TestReplaceConfigEnableConfirmation(t *testing.T) {
	st := newTestStore(t, DefaultConfig())
	m := NewManager(st, nil)
	cfg := st.Snapshot().Config
	cfg.Enabled = true
	if _, _, err := m.ReplaceConfig(1, cfg, "alice", false); err == nil {
		t.Fatalf("expected confirm_enable requirement")
	}
	view, changed, err := m.ReplaceConfig(1, cfg, "alice", true)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if view.ConfigRevision != 2 {
		t.Fatalf("revision=%d", view.ConfigRevision)
	}
	if len(changed) == 0 {
		t.Fatalf("expected changed settings")
	}
}
