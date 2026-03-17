package diff

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type SegmentKind int

const (
	SegmentField SegmentKind = iota
	SegmentFieldWildcard
	SegmentIndex
	SegmentIndexWildcard
)

type Segment struct {
	Kind  SegmentKind
	Key   string
	Index int
}

type PathPattern struct {
	Raw      string
	Segments []Segment
}

func ParsePatterns(values []string) ([]PathPattern, error) {
	patterns := make([]PathPattern, 0, len(values))
	for _, value := range values {
		pattern, err := ParsePath(value)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func ParsePath(value string) (PathPattern, error) {
	if strings.TrimSpace(value) == "" {
		return PathPattern{}, fmt.Errorf("path cannot be empty")
	}

	pattern := PathPattern{Raw: value}
	input := strings.TrimSpace(value)

	for i := 0; i < len(input); {
		switch input[i] {
		case '.':
			i++
		case '[':
			segment, next, err := parseBracketSegment(input, i)
			if err != nil {
				return PathPattern{}, err
			}
			pattern.Segments = append(pattern.Segments, segment)
			i = next
		default:
			start := i
			for i < len(input) && input[i] != '.' && input[i] != '[' {
				i++
			}
			token := input[start:i]
			if token == "" {
				return PathPattern{}, fmt.Errorf("invalid empty segment in %q", value)
			}
			if token == "*" {
				pattern.Segments = append(pattern.Segments, Segment{Kind: SegmentFieldWildcard})
				continue
			}
			pattern.Segments = append(pattern.Segments, Segment{Kind: SegmentField, Key: token})
		}
	}

	return pattern, nil
}

func MatchAny(path string, patterns []PathPattern) bool {
	actual, err := ParsePath(path)
	if err != nil {
		return false
	}

	for _, pattern := range patterns {
		if matchesPath(pattern, actual) {
			return true
		}
	}

	return false
}

func RenderPath(segments []Segment) string {
	if len(segments) == 0 {
		return "<root>"
	}

	var builder strings.Builder
	for i, segment := range segments {
		switch segment.Kind {
		case SegmentField:
			if isBareIdentifier(segment.Key) {
				if i > 0 {
					builder.WriteByte('.')
				}
				builder.WriteString(segment.Key)
				continue
			}
			if i == 0 {
				builder.WriteString("['")
				builder.WriteString(strings.ReplaceAll(segment.Key, "'", "\\'"))
				builder.WriteString("']")
				continue
			}
			if segments[i-1].Kind == SegmentFieldWildcard {
				builder.WriteByte('.')
			}
			builder.WriteString("['")
			builder.WriteString(strings.ReplaceAll(segment.Key, "'", "\\'"))
			builder.WriteString("']")
		case SegmentFieldWildcard:
			if i > 0 {
				builder.WriteByte('.')
			}
			builder.WriteByte('*')
		case SegmentIndex:
			builder.WriteString(fmt.Sprintf("[%d]", segment.Index))
		case SegmentIndexWildcard:
			builder.WriteString("[*]")
		}
	}

	return builder.String()
}

func matchesPath(pattern, actual PathPattern) bool {
	if len(pattern.Segments) != len(actual.Segments) {
		return false
	}

	for i := range pattern.Segments {
		if !segmentsMatch(pattern.Segments[i], actual.Segments[i]) {
			return false
		}
	}

	return true
}

func hasDescendantMatch(pattern PathPattern, prefix []Segment) bool {
	if len(prefix) > len(pattern.Segments) {
		return false
	}

	for i := range prefix {
		if !segmentsMatch(pattern.Segments[i], prefix[i]) {
			return false
		}
	}

	return true
}

func segmentsMatch(pattern Segment, actual Segment) bool {
	switch pattern.Kind {
	case SegmentFieldWildcard:
		return actual.Kind == SegmentField
	case SegmentIndexWildcard:
		return actual.Kind == SegmentIndex
	case SegmentField:
		return actual.Kind == SegmentField && pattern.Key == actual.Key
	case SegmentIndex:
		return actual.Kind == SegmentIndex && pattern.Index == actual.Index
	default:
		return false
	}
}

func parseBracketSegment(input string, start int) (Segment, int, error) {
	if start+2 >= len(input) {
		return Segment{}, 0, fmt.Errorf("unterminated bracket segment in %q", input)
	}

	if input[start+1] == '*' {
		if input[start+2] != ']' {
			return Segment{}, 0, fmt.Errorf("invalid index wildcard in %q", input)
		}
		return Segment{Kind: SegmentIndexWildcard}, start + 3, nil
	}

	if input[start+1] == '\'' || input[start+1] == '"' {
		quote := input[start+1]
		end := start + 2
		for end < len(input) && input[end] != quote {
			end++
		}
		if end >= len(input) || end+1 >= len(input) || input[end+1] != ']' {
			return Segment{}, 0, fmt.Errorf("unterminated quoted key in %q", input)
		}
		return Segment{
			Kind: SegmentField,
			Key:  input[start+2 : end],
		}, end + 2, nil
	}

	end := start + 1
	for end < len(input) && input[end] != ']' {
		end++
	}
	if end >= len(input) {
		return Segment{}, 0, fmt.Errorf("unterminated index in %q", input)
	}

	index, err := strconv.Atoi(input[start+1 : end])
	if err != nil {
		return Segment{}, 0, fmt.Errorf("invalid index in %q: %w", input, err)
	}

	return Segment{Kind: SegmentIndex, Index: index}, end + 1, nil
}

func isBareIdentifier(value string) bool {
	if value == "" {
		return false
	}

	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '-' {
			continue
		}
		return false
	}

	return true
}
