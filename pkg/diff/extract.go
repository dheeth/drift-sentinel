package diff

func Extract(obj map[string]any, patterns []PathPattern) map[string]any {
	value, keep := extractValue(obj, patterns, nil)
	if !keep {
		return map[string]any{}
	}

	extracted, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	return extracted
}

func extractValue(value any, patterns []PathPattern, current []Segment) (any, bool) {
	if matchesAnyPath(current, patterns) {
		return cloneValue(value), true
	}
	if !hasAnyDescendantMatch(current, patterns) {
		return nil, false
	}

	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, child := range typed {
			childPath := appendPath(current, Segment{Kind: SegmentField, Key: key})
			extracted, keep := extractValue(child, patterns, childPath)
			if keep {
				result[key] = extracted
			}
		}
		if len(result) == 0 {
			return nil, false
		}
		return result, true
	case []any:
		result := make([]any, len(typed))
		kept := false
		for index, child := range typed {
			childPath := appendPath(current, Segment{Kind: SegmentIndex, Index: index})
			extracted, keep := extractValue(child, patterns, childPath)
			if keep {
				result[index] = extracted
				kept = true
			}
		}
		if !kept {
			return nil, false
		}
		return result, true
	default:
		return cloneValue(value), true
	}
}
