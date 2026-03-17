package rules

import "path"

func Match(rules []Rule, input MatchInput) (*Rule, bool) {
	ordered := append([]Rule(nil), rules...)
	sortRules(ordered)

	for _, rule := range ordered {
		if !matchesNamespace(rule.Namespaces, input.Namespace) {
			continue
		}
		if !matchesSelector(rule.Selectors, input.APIGroup, input.Kind) {
			continue
		}

		matched := cloneRule(rule)
		return &matched, true
	}

	return nil, false
}

func matchesNamespace(patterns []string, namespace string) bool {
	for _, pattern := range patterns {
		matched, err := path.Match(pattern, namespace)
		if err == nil && matched {
			return true
		}
	}

	return false
}

func matchesSelector(selectors []ResourceSelector, apiGroup, kind string) bool {
	for _, selector := range selectors {
		if selector.APIGroup == apiGroup && selector.Kind == kind {
			return true
		}
	}

	return false
}
