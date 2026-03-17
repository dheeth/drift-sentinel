package rules

import "testing"

func TestParseConfigMap(t *testing.T) {
	configMap := ConfigMap{
		Name:      "drift-sentinel-production",
		Namespace: "drift-system",
		Annotations: map[string]string{
			RuleAnnotationKey: "true",
		},
		Data: map[string]string{
			"spec": `
mode: enforce
priority: 200
namespaces:
  - "prod-*"
  - "staging"
selectors:
  - apiGroup: "apps"
    kind: "Deployment"
  - apiGroup: "apps"
    kind: "StatefulSet"
exclude:
  - "status"
  - "metadata.*"
include: []
mutable:
  - "spec.template.spec.containers[*].image"
`,
		},
	}

	rule, err := ParseConfigMap(configMap)
	if err != nil {
		t.Fatalf("ParseConfigMap returned error: %v", err)
	}

	if rule.Name != "drift-sentinel-production" {
		t.Fatalf("unexpected rule name: %q", rule.Name)
	}
	if rule.Namespace != "drift-system" {
		t.Fatalf("unexpected rule namespace: %q", rule.Namespace)
	}
	if rule.Priority != 200 {
		t.Fatalf("unexpected rule priority: %d", rule.Priority)
	}
	if rule.Mode != ModeEnforce {
		t.Fatalf("unexpected rule mode: %q", rule.Mode)
	}
	if len(rule.Namespaces) != 2 {
		t.Fatalf("unexpected namespace count: %d", len(rule.Namespaces))
	}
	if len(rule.Selectors) != 2 {
		t.Fatalf("unexpected selector count: %d", len(rule.Selectors))
	}
	if len(rule.Include) != 0 {
		t.Fatalf("unexpected include count: %d", len(rule.Include))
	}
	if rule.Bypass != DefaultBypassAnnotation {
		t.Fatalf("unexpected bypass annotation: %q", rule.Bypass)
	}
}

func TestParseConfigMapInlineLists(t *testing.T) {
	configMap := ConfigMap{
		Name:      "inline",
		Namespace: "drift-system",
		Annotations: map[string]string{
			RuleAnnotationKey: "true",
		},
		Data: map[string]string{
			"spec": `
mode: "warn"
priority: 50
namespaces: ["team-a-*", "team-b"]
selectors:
  - apiGroup: "apps"
    kind: "Deployment"
exclude: ["status"]
include: ["spec.replicas"]
mutable: ["spec.template.spec.containers[*].image"]
bypass: "custom.dev/bypass"
`,
		},
	}

	rule, err := ParseConfigMap(configMap)
	if err != nil {
		t.Fatalf("ParseConfigMap returned error: %v", err)
	}

	if rule.Mode != ModeWarn {
		t.Fatalf("unexpected mode: %q", rule.Mode)
	}
	if rule.Bypass != "custom.dev/bypass" {
		t.Fatalf("unexpected bypass: %q", rule.Bypass)
	}
	if got := len(rule.Namespaces); got != 2 {
		t.Fatalf("unexpected namespace count: %d", got)
	}
	if got := len(rule.Include); got != 1 {
		t.Fatalf("unexpected include count: %d", got)
	}
}

func TestParseConfigMapRejectsInvalidMode(t *testing.T) {
	configMap := ConfigMap{
		Name:      "invalid-mode",
		Namespace: "drift-system",
		Annotations: map[string]string{
			RuleAnnotationKey: "true",
		},
		Data: map[string]string{
			"spec": `
mode: block
namespaces: ["*"]
selectors:
  - apiGroup: "apps"
    kind: "Deployment"
`,
		},
	}

	if _, err := ParseConfigMap(configMap); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestParseConfigMapRejectsMissingSelectors(t *testing.T) {
	configMap := ConfigMap{
		Name:      "missing-selectors",
		Namespace: "drift-system",
		Annotations: map[string]string{
			RuleAnnotationKey: "true",
		},
		Data: map[string]string{
			"spec": `
mode: enforce
namespaces: ["*"]
`,
		},
	}

	if _, err := ParseConfigMap(configMap); err == nil {
		t.Fatal("expected missing selectors error")
	}
}
