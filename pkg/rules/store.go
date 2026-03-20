package rules

import (
	"slices"
	"sort"
	"sync"
)

type Store struct {
	mu    sync.RWMutex
	rules []Rule
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Replace(rules []Rule) {
	cloned := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, cloneRule(rule))
	}
	sortRules(cloned)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.rules = cloned
}

func (s *Store) Snapshot() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make([]Rule, 0, len(s.rules))
	for _, rule := range s.rules {
		snapshot = append(snapshot, cloneRule(rule))
	}
	return snapshot
}

func (s *Store) Match(input MatchInput) (*Rule, bool) {
	return Match(s.Snapshot(), input)
}

func sortRules(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		left := rules[i]
		right := rules[j]

		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		if left.Namespace != right.Namespace {
			return left.Namespace < right.Namespace
		}
		return left.Name < right.Name
	})
}

func cloneRule(rule Rule) Rule {
	cloned := rule
	cloned.Namespaces = slices.Clone(rule.Namespaces)
	cloned.Selectors = slices.Clone(rule.Selectors)
	cloned.Labels = slices.Clone(rule.Labels)
	cloned.Exclude = slices.Clone(rule.Exclude)
	cloned.Include = slices.Clone(rule.Include)
	cloned.Mutable = slices.Clone(rule.Mutable)
	cloned.Users = slices.Clone(rule.Users)
	return cloned
}
