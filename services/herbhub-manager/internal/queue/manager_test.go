package queue

import "testing"

func TestItemsReturnsValueCopies(t *testing.T) {
	m := &Manager{}
	m.items = []*Item{{ID: "1", Slug: "a", Title: "A"}}

	snapshot := m.Items()
	if len(snapshot) != 1 {
		t.Fatalf("expected snapshot length 1, got %d", len(snapshot))
	}

	snapshot[0].Title = "Changed"
	if m.items[0].Title != "A" {
		t.Fatalf("manager item mutated through snapshot; got %q", m.items[0].Title)
	}
}
