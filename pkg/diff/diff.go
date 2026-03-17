package diff

import "sort"

type DiffResult struct {
	Identical        bool
	ChangedPaths     []string
	MutableChanged   []string
	ImmutableChanged []string
}

func Compare(oldObj, newObj map[string]any) DiffResult {
	paths := make([]string, 0)
	compareValue(oldObj, newObj, nil, &paths)
	sort.Strings(paths)

	return DiffResult{
		Identical:    len(paths) == 0,
		ChangedPaths: paths,
	}
}

func compareValue(oldValue, newValue any, current []Segment, changedPaths *[]string) {
	oldMap, oldIsMap := oldValue.(map[string]any)
	newMap, newIsMap := newValue.(map[string]any)
	if oldIsMap || newIsMap {
		if !oldIsMap || !newIsMap {
			appendChangedPath(current, changedPaths)
			return
		}
		compareMap(oldMap, newMap, current, changedPaths)
		return
	}

	oldList, oldIsList := oldValue.([]any)
	newList, newIsList := newValue.([]any)
	if oldIsList || newIsList {
		if !oldIsList || !newIsList {
			appendChangedPath(current, changedPaths)
			return
		}
		compareList(oldList, newList, current, changedPaths)
		return
	}

	if !valuesEqual(oldValue, newValue) {
		appendChangedPath(current, changedPaths)
	}
}

func compareMap(oldMap, newMap map[string]any, current []Segment, changedPaths *[]string) {
	keys := make(map[string]struct{}, len(oldMap)+len(newMap))
	for key := range oldMap {
		keys[key] = struct{}{}
	}
	for key := range newMap {
		keys[key] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		childPath := appendPath(current, Segment{Kind: SegmentField, Key: key})
		oldChild, oldOK := oldMap[key]
		newChild, newOK := newMap[key]

		if !oldOK || !newOK {
			appendChangedPath(childPath, changedPaths)
			continue
		}

		compareValue(oldChild, newChild, childPath, changedPaths)
	}
}

func compareList(oldList, newList []any, current []Segment, changedPaths *[]string) {
	maxLen := len(oldList)
	if len(newList) > maxLen {
		maxLen = len(newList)
	}

	for index := 0; index < maxLen; index++ {
		childPath := appendPath(current, Segment{Kind: SegmentIndex, Index: index})
		if index >= len(oldList) || index >= len(newList) {
			appendChangedPath(childPath, changedPaths)
			continue
		}
		compareValue(oldList[index], newList[index], childPath, changedPaths)
	}
}

func appendChangedPath(path []Segment, changedPaths *[]string) {
	*changedPaths = append(*changedPaths, RenderPath(path))
}

func matchesAnyPath(current []Segment, patterns []PathPattern) bool {
	actual := PathPattern{Segments: current}
	for _, pattern := range patterns {
		if matchesPath(pattern, actual) {
			return true
		}
	}

	return false
}

func hasAnyDescendantMatch(current []Segment, patterns []PathPattern) bool {
	for _, pattern := range patterns {
		if hasDescendantMatch(pattern, current) {
			return true
		}
	}

	return false
}

func appendPath(current []Segment, next Segment) []Segment {
	path := make([]Segment, 0, len(current)+1)
	path = append(path, current...)
	path = append(path, next)
	return path
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, child := range typed {
			cloned[key] = cloneValue(child)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, child := range typed {
			cloned[index] = cloneValue(child)
		}
		return cloned
	default:
		return typed
	}
}

func valuesEqual(left, right any) bool {
	switch leftTyped := left.(type) {
	case string:
		rightTyped, ok := right.(string)
		return ok && leftTyped == rightTyped
	case bool:
		rightTyped, ok := right.(bool)
		return ok && leftTyped == rightTyped
	case nil:
		return right == nil
	case float64:
		rightTyped, ok := right.(float64)
		return ok && leftTyped == rightTyped
	case float32:
		rightTyped, ok := right.(float32)
		return ok && leftTyped == rightTyped
	case int:
		rightTyped, ok := right.(int)
		return ok && leftTyped == rightTyped
	case int8:
		rightTyped, ok := right.(int8)
		return ok && leftTyped == rightTyped
	case int16:
		rightTyped, ok := right.(int16)
		return ok && leftTyped == rightTyped
	case int32:
		rightTyped, ok := right.(int32)
		return ok && leftTyped == rightTyped
	case int64:
		rightTyped, ok := right.(int64)
		return ok && leftTyped == rightTyped
	case uint:
		rightTyped, ok := right.(uint)
		return ok && leftTyped == rightTyped
	case uint8:
		rightTyped, ok := right.(uint8)
		return ok && leftTyped == rightTyped
	case uint16:
		rightTyped, ok := right.(uint16)
		return ok && leftTyped == rightTyped
	case uint32:
		rightTyped, ok := right.(uint32)
		return ok && leftTyped == rightTyped
	case uint64:
		rightTyped, ok := right.(uint64)
		return ok && leftTyped == rightTyped
	default:
		return left == right
	}
}
