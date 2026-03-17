package diff

import "testing"

func TestParsePathAndRender(t *testing.T) {
	pattern, err := ParsePath("spec.template.spec.containers[*].image")
	if err != nil {
		t.Fatalf("ParsePath returned error: %v", err)
	}

	if got := RenderPath(pattern.Segments); got != "spec.template.spec.containers[*].image" {
		t.Fatalf("unexpected rendered path: %q", got)
	}
}

func TestMatchAnySupportsWildcardsAndQuotedKeys(t *testing.T) {
	patterns, err := ParsePatterns([]string{
		"spec.template.spec.containers[*].image",
		"metadata.annotations['kubectl.kubernetes.io/last-applied-configuration']",
	})
	if err != nil {
		t.Fatalf("ParsePatterns returned error: %v", err)
	}

	if !MatchAny("spec.template.spec.containers[0].image", patterns) {
		t.Fatal("expected container image path to match")
	}
	if !MatchAny("metadata.annotations['kubectl.kubernetes.io/last-applied-configuration']", patterns) {
		t.Fatal("expected annotation path to match")
	}
	if MatchAny("spec.template.spec.containers[0].env", patterns) {
		t.Fatal("did not expect env path to match")
	}
}
