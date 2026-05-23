package trends

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type SearchEvent struct {
	Query     string    `json:"query"`
	UserID    string    `json:"user_id,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

type TrendItem struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

type storedEvent struct {
	query  string
	source string
	at     time.Time
}

type Store struct {
	mu                   sync.RWMutex
	window               time.Duration
	maxPerSourceInWindow int
	stopWords            map[string]struct{}
	events               []storedEvent
	top                  []TrendItem
}

func NewStore(window time.Duration, maxPerSourceInWindow int, stopWords []string) *Store {
	if maxPerSourceInWindow <= 0 {
		maxPerSourceInWindow = 5
	}

	s := &Store{
		window:               window,
		maxPerSourceInWindow: maxPerSourceInWindow,
		stopWords:            make(map[string]struct{}),
	}
	s.SetStopWords(stopWords)

	return s
}

func (s *Store) Add(event SearchEvent, now time.Time) {
	query := normalize(event.Query)
	if query == "" {
		return
	}

	source := event.UserID
	if source == "" {
		source = event.IP
	}
	if source == "" {
		source = "unknown"
	}

	at := event.Timestamp
	if at.IsZero() {
		at = now
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isStopWordLocked(query) {
		return
	}

	s.pruneLocked(now)
	if s.sourceCountLocked(query, source) >= s.maxPerSourceInWindow {
		return
	}

	s.events = append(s.events, storedEvent{query: query, source: source, at: at})
	s.rebuildTopLocked()
}

func (s *Store) Top(limit int) []TrendItem {
	if limit <= 0 {
		limit = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit > len(s.top) {
		limit = len(s.top)
	}

	result := make([]TrendItem, limit)
	copy(result, s.top[:limit])

	return result
}

func (s *Store) SetStopWords(words []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopWords = make(map[string]struct{}, len(words))
	for _, word := range words {
		word = normalize(word)
		if word != "" {
			s.stopWords[word] = struct{}{}
		}
	}

	filtered := s.events[:0]
	for _, event := range s.events {
		if !s.isStopWordLocked(event.query) {
			filtered = append(filtered, event)
		}
	}
	s.events = filtered
	s.rebuildTopLocked()
}

func (s *Store) PruneAndRebuild(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(now)
	s.rebuildTopLocked()
}

func (s *Store) pruneLocked(now time.Time) {
	from := now.Add(-s.window)
	kept := s.events[:0]
	for _, event := range s.events {
		if !event.at.Before(from) {
			kept = append(kept, event)
		}
	}
	s.events = kept
}

func (s *Store) sourceCountLocked(query string, source string) int {
	count := 0
	for _, event := range s.events {
		if event.query == query && event.source == source {
			count++
		}
	}
	return count
}

func (s *Store) rebuildTopLocked() {
	counts := make(map[string]int)
	for _, event := range s.events {
		counts[event.query]++
	}

	top := make([]TrendItem, 0, len(counts))
	for query, count := range counts {
		top = append(top, TrendItem{Query: query, Count: count})
	}

	sort.Slice(top, func(i, j int) bool {
		if top[i].Count == top[j].Count {
			return top[i].Query < top[j].Query
		}
		return top[i].Count > top[j].Count
	})

	s.top = top
}

func (s *Store) isStopWordLocked(query string) bool {
	_, ok := s.stopWords[query]
	return ok
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
