package diff

func Strip(obj map[string]any, patterns []PathPattern) map[string]any {
	value, keep := stripValue(obj, patterns, nil)
	if !keep {
		return map[string]any{}
	}

	stripped, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	return stripped
}

func stripValue(value any, patterns []PathPattern, current []Segment) (any, bool) {
	if matchesAnyPath(current, patterns) {
		return nil, false
	}

	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, child := range typed {
			childPath := appendPath(current, Segment{Kind: SegmentField, Key: key})
			stripped, keep := stripValue(child, patterns, childPath)
			if keep {
				result[key] = stripped
			}
		}
		if len(result) == 0 {
			return nil, false
		}
		return result, true
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			childPath := appendPath(current, Segment{Kind: SegmentIndex, Index: index})
			stripped, keep := stripValue(child, patterns, childPath)
			if keep {
				result[index] = stripped
			}
		}
		return result, true
	default:
		return cloneValue(value), true
	}
}
