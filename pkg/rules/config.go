package rules

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

type rawRuleSpec struct {
	Mode       string
	Priority   int
	Namespaces []string
	Selectors  []ResourceSelector
	Exclude    []string
	Include    []string
	Mutable    []string
	Bypass     string
}

func IsRuleConfigMap(configMap ConfigMap) bool {
	return strings.EqualFold(strings.TrimSpace(configMap.Annotations[RuleAnnotationKey]), "true")
}

func ParseConfigMap(configMap ConfigMap) (Rule, error) {
	if !IsRuleConfigMap(configMap) {
		return Rule{}, fmt.Errorf("configmap %s/%s is not annotated as a drift sentinel rule", configMap.Namespace, configMap.Name)
	}

	specText := strings.TrimSpace(configMap.Data["spec"])
	if specText == "" {
		return Rule{}, fmt.Errorf("configmap %s/%s is missing data.spec", configMap.Namespace, configMap.Name)
	}

	spec, err := parseRuleSpec(specText)
	if err != nil {
		return Rule{}, fmt.Errorf("parse configmap %s/%s: %w", configMap.Namespace, configMap.Name, err)
	}

	mode := Mode(spec.Mode)
	if !isValidMode(mode) {
		return Rule{}, fmt.Errorf("invalid mode %q", spec.Mode)
	}
	if len(spec.Namespaces) == 0 {
		return Rule{}, fmt.Errorf("namespaces must contain at least one pattern")
	}
	if len(spec.Selectors) == 0 {
		return Rule{}, fmt.Errorf("selectors must contain at least one selector")
	}
	for _, pattern := range spec.Namespaces {
		if _, err := path.Match(pattern, ""); err != nil {
			return Rule{}, fmt.Errorf("invalid namespace pattern %q: %w", pattern, err)
		}
	}

	bypass := spec.Bypass
	if bypass == "" {
		bypass = DefaultBypassAnnotation
	}

	return Rule{
		Name:       configMap.Name,
		Namespace:  configMap.Namespace,
		Priority:   spec.Priority,
		Mode:       mode,
		Namespaces: append([]string(nil), spec.Namespaces...),
		Selectors:  append([]ResourceSelector(nil), spec.Selectors...),
		Exclude:    append([]string(nil), spec.Exclude...),
		Include:    append([]string(nil), spec.Include...),
		Mutable:    append([]string(nil), spec.Mutable...),
		Bypass:     bypass,
	}, nil
}

func isValidMode(mode Mode) bool {
	switch mode {
	case ModeEnforce, ModeWarn, ModeDryRun, ModeOff:
		return true
	default:
		return false
	}
}

func parseRuleSpec(spec string) (rawRuleSpec, error) {
	lines := strings.Split(spec, "\n")
	var parsed rawRuleSpec

	for i := 0; i < len(lines); i++ {
		line := sanitizeLine(lines[i])
		if line == "" {
			continue
		}

		if strings.ContainsRune(line, '\t') {
			return rawRuleSpec{}, fmt.Errorf("line %d: tabs are not supported", i+1)
		}

		indent := countIndent(line)
		if indent != 0 {
			return rawRuleSpec{}, fmt.Errorf("line %d: expected top-level key", i+1)
		}

		key, value, ok := splitKeyValue(strings.TrimSpace(line))
		if !ok {
			return rawRuleSpec{}, fmt.Errorf("line %d: expected key/value pair", i+1)
		}

		if value != "" {
			if err := assignInlineValue(&parsed, key, value, i+1); err != nil {
				return rawRuleSpec{}, err
			}
			continue
		}

		block, next := gatherIndentedBlock(lines, i+1, indent)
		if err := assignBlockValue(&parsed, key, block, i+1); err != nil {
			return rawRuleSpec{}, err
		}
		i = next - 1
	}

	return parsed, nil
}

func assignInlineValue(parsed *rawRuleSpec, key, value string, lineNumber int) error {
	switch key {
	case "mode":
		scalar, err := parseScalar(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Mode = scalar
	case "priority":
		number, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("line %d: invalid priority: %w", lineNumber, err)
		}
		parsed.Priority = number
	case "bypass":
		scalar, err := parseScalar(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Bypass = scalar
	case "namespaces":
		items, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Namespaces = items
	case "exclude":
		items, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Exclude = items
	case "include":
		items, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Include = items
	case "mutable":
		items, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Mutable = items
	default:
		return fmt.Errorf("line %d: unsupported key %q", lineNumber, key)
	}

	return nil
}

func assignBlockValue(parsed *rawRuleSpec, key string, block []string, lineNumber int) error {
	switch key {
	case "namespaces":
		items, err := parseStringList(block)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Namespaces = items
	case "exclude":
		items, err := parseStringList(block)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Exclude = items
	case "include":
		items, err := parseStringList(block)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Include = items
	case "mutable":
		items, err := parseStringList(block)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Mutable = items
	case "selectors":
		items, err := parseSelectorList(block)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
		parsed.Selectors = items
	default:
		return fmt.Errorf("line %d: unsupported block key %q", lineNumber, key)
	}

	return nil
}

func gatherIndentedBlock(lines []string, start, parentIndent int) ([]string, int) {
	block := make([]string, 0)
	index := start

	for ; index < len(lines); index++ {
		line := sanitizeLine(lines[index])
		if line == "" {
			continue
		}

		indent := countIndent(line)
		if indent <= parentIndent {
			break
		}

		block = append(block, line)
	}

	return block, index
}

func parseStringList(lines []string) ([]string, error) {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			return nil, fmt.Errorf("expected list item, got %q", trimmed)
		}

		value, err := parseScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}

	return result, nil
}

func parseSelectorList(lines []string) ([]ResourceSelector, error) {
	var selectors []ResourceSelector

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			return nil, fmt.Errorf("expected selector list item, got %q", trimmed)
		}

		itemIndent := countIndent(line)
		selector := ResourceSelector{}

		firstKey, firstValue, ok := splitKeyValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		if !ok {
			return nil, fmt.Errorf("invalid selector item %q", trimmed)
		}
		if err := assignSelectorField(&selector, firstKey, firstValue); err != nil {
			return nil, err
		}

		for i+1 < len(lines) {
			nextLine := lines[i+1]
			if countIndent(nextLine) <= itemIndent {
				break
			}

			i++
			key, value, ok := splitKeyValue(strings.TrimSpace(nextLine))
			if !ok {
				return nil, fmt.Errorf("invalid selector field %q", strings.TrimSpace(nextLine))
			}
			if err := assignSelectorField(&selector, key, value); err != nil {
				return nil, err
			}
		}

		if selector.Kind == "" {
			return nil, fmt.Errorf("selector kind is required")
		}

		selectors = append(selectors, selector)
	}

	return selectors, nil
}

func assignSelectorField(selector *ResourceSelector, key, value string) error {
	scalar, err := parseScalar(value)
	if err != nil {
		return err
	}

	switch key {
	case "apiGroup":
		selector.APIGroup = scalar
	case "kind":
		selector.Kind = scalar
	default:
		return fmt.Errorf("unsupported selector field %q", key)
	}

	return nil
}

func parseInlineList(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "[]" {
		return []string{}, nil
	}
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return nil, fmt.Errorf("expected inline list, got %q", value)
	}

	content := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if content == "" {
		return []string{}, nil
	}

	items := make([]string, 0)
	start := 0
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '\'', '"':
			if !inQuotes {
				inQuotes = true
				quoteChar = content[i]
			} else if quoteChar == content[i] {
				inQuotes = false
				quoteChar = 0
			}
		case ',':
			if !inQuotes {
				item, err := parseScalar(strings.TrimSpace(content[start:i]))
				if err != nil {
					return nil, err
				}
				items = append(items, item)
				start = i + 1
			}
		}
	}

	item, err := parseScalar(strings.TrimSpace(content[start:]))
	if err != nil {
		return nil, err
	}
	items = append(items, item)

	return items, nil
}

func parseScalar(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
			return trimmed[1 : len(trimmed)-1], nil
		}
	}
	return trimmed, nil
}

func sanitizeLine(line string) string {
	line = strings.TrimRight(line, "\r")
	line = stripComment(line)
	if strings.TrimSpace(line) == "" {
		return ""
	}
	return line
}

func stripComment(line string) string {
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'', '"':
			if !inQuotes {
				inQuotes = true
				quoteChar = line[i]
			} else if quoteChar == line[i] {
				inQuotes = false
				quoteChar = 0
			}
		case '#':
			if !inQuotes {
				return strings.TrimRight(line[:i], " ")
			}
		}
	}

	return line
}

func countIndent(line string) int {
	count := 0
	for _, char := range line {
		if char != ' ' {
			break
		}
		count++
	}
	return count
}

func splitKeyValue(line string) (string, string, bool) {
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'', '"':
			if !inQuotes {
				inQuotes = true
				quoteChar = line[i]
			} else if quoteChar == line[i] {
				inQuotes = false
				quoteChar = 0
			}
		case ':':
			if !inQuotes {
				key := strings.TrimSpace(line[:i])
				value := strings.TrimSpace(line[i+1:])
				if key == "" {
					return "", "", false
				}
				return key, value, true
			}
		}
	}

	return "", "", false
}
