package trends

import (
	"testing"
	"time"
)

func TestTopReturnsQueriesFromLastWindow(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	store := NewStore(5*time.Minute, 5, nil)

	store.Add(SearchEvent{Query: "phone", UserID: "1", Timestamp: now.Add(-6 * time.Minute)}, now)
	store.Add(SearchEvent{Query: "laptop", UserID: "2", Timestamp: now.Add(-1 * time.Minute)}, now)
	store.Add(SearchEvent{Query: "laptop", UserID: "3", Timestamp: now.Add(-1 * time.Minute)}, now)
	store.Add(SearchEvent{Query: "book", UserID: "4", Timestamp: now.Add(-1 * time.Minute)}, now)
	store.PruneAndRebuild(now)

	top := store.Top(10)
	if len(top) != 2 {
		t.Fatalf("expected 2 items, got %d", len(top))
	}
	if top[0].Query != "laptop" || top[0].Count != 2 {
		t.Fatalf("unexpected first item: %+v", top[0])
	}
	if top[1].Query != "book" || top[1].Count != 1 {
		t.Fatalf("unexpected second item: %+v", top[1])
	}
}

func TestStopWordsAreHidden(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	store := NewStore(5*time.Minute, 5, []string{"bad"})

	store.Add(SearchEvent{Query: "bad", UserID: "1"}, now)
	store.Add(SearchEvent{Query: "good", UserID: "2"}, now)

	top := store.Top(10)
	if len(top) != 1 || top[0].Query != "good" {
		t.Fatalf("unexpected top: %+v", top)
	}
}

func TestSourceLimitProtectsFromInflation(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	store := NewStore(5*time.Minute, 2, nil)

	store.Add(SearchEvent{Query: "boots", UserID: "bot"}, now)
	store.Add(SearchEvent{Query: "boots", UserID: "bot"}, now)
	store.Add(SearchEvent{Query: "boots", UserID: "bot"}, now)
	store.Add(SearchEvent{Query: "boots", UserID: "real-user"}, now)

	top := store.Top(10)
	if len(top) != 1 {
		t.Fatalf("expected one item, got %+v", top)
	}
	if top[0].Count != 3 {
		t.Fatalf("expected capped count 3, got %d", top[0].Count)
	}
}

func TestTopLimit(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	store := NewStore(5*time.Minute, 5, nil)

	store.Add(SearchEvent{Query: "a", UserID: "1"}, now)
	store.Add(SearchEvent{Query: "b", UserID: "2"}, now)

	top := store.Top(1)
	if len(top) != 1 {
		t.Fatalf("expected one item, got %d", len(top))
	}
}
