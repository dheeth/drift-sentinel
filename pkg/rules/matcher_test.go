package rules

import "testing"

func TestMatchPrefersHighestPriority(t *testing.T) {
	rules := []Rule{
		{
			Name:       "low",
			Namespace:  "team-a",
			Priority:   100,
			Mode:       ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
		},
		{
			Name:       "high",
			Namespace:  "team-b",
			Priority:   200,
			Mode:       ModeWarn,
			Namespaces: []string{"prod-*"},
			Selectors: []ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
		},
	}

	rule, ok := Match(rules, MatchInput{
		Namespace: "prod-core",
		APIGroup:  "apps",
		Kind:      "Deployment",
	})
	if !ok {
		t.Fatal("expected a matching rule")
	}
	if rule.Name != "high" {
		t.Fatalf("unexpected matched rule: %q", rule.Name)
	}
}

func TestMatchUsesDeterministicTieBreaker(t *testing.T) {
	rules := []Rule{
		{
			Name:       "zzz",
			Namespace:  "team-b",
			Priority:   100,
			Mode:       ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
		},
		{
			Name:       "aaa",
			Namespace:  "team-a",
			Priority:   100,
			Mode:       ModeWarn,
			Namespaces: []string{"prod-*"},
			Selectors: []ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
		},
	}

	rule, ok := Match(rules, MatchInput{
		Namespace: "prod-core",
		APIGroup:  "apps",
		Kind:      "Deployment",
	})
	if !ok {
		t.Fatal("expected a matching rule")
	}
	if rule.Namespace != "team-a" || rule.Name != "aaa" {
		t.Fatalf("unexpected matched rule: %s/%s", rule.Namespace, rule.Name)
	}
}

func TestStoreSnapshotAndMatch(t *testing.T) {
	store := NewStore()
	store.Replace([]Rule{
		{
			Name:       "match-me",
			Namespace:  "drift-system",
			Priority:   10,
			Mode:       ModeEnforce,
			Namespaces: []string{"staging"},
			Selectors: []ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
		},
	})

	snapshot := store.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("unexpected snapshot size: %d", len(snapshot))
	}

	snapshot[0].Name = "mutated"

	rule, ok := store.Match(MatchInput{
		Namespace: "staging",
		APIGroup:  "apps",
		Kind:      "Deployment",
	})
	if !ok {
		t.Fatal("expected a matching rule")
	}
	if rule.Name != "match-me" {
		t.Fatalf("store snapshot leaked mutation: %q", rule.Name)
	}
}
