package diff

import "testing"

func TestCompareDetectsChangedPaths(t *testing.T) {
	oldObj := map[string]any{
		"spec": map[string]any{
			"replicas": 2,
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"image": "v1",
						},
					},
				},
			},
		},
	}
	newObj := map[string]any{
		"spec": map[string]any{
			"replicas": 3,
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"image": "v2",
						},
					},
				},
			},
		},
	}

	result := Compare(oldObj, newObj)
	if result.Identical {
		t.Fatal("expected objects to differ")
	}
	if len(result.ChangedPaths) != 2 {
		t.Fatalf("unexpected changed path count: %d", len(result.ChangedPaths))
	}
	if result.ChangedPaths[0] != "spec.replicas" {
		t.Fatalf("unexpected first changed path: %q", result.ChangedPaths[0])
	}
	if result.ChangedPaths[1] != "spec.template.spec.containers[0].image" {
		t.Fatalf("unexpected second changed path: %q", result.ChangedPaths[1])
	}
}

func TestCompareDetectsAddedAndTypeChangedFields(t *testing.T) {
	oldObj := map[string]any{
		"spec": map[string]any{
			"replicas": 2,
		},
	}
	newObj := map[string]any{
		"spec": map[string]any{
			"replicas": "2",
			"paused":   true,
		},
	}

	result := Compare(oldObj, newObj)
	if result.Identical {
		t.Fatal("expected objects to differ")
	}
	if len(result.ChangedPaths) != 2 {
		t.Fatalf("unexpected changed path count: %d", len(result.ChangedPaths))
	}
	if result.ChangedPaths[0] != "spec.paused" {
		t.Fatalf("unexpected first changed path: %q", result.ChangedPaths[0])
	}
	if result.ChangedPaths[1] != "spec.replicas" {
		t.Fatalf("unexpected second changed path: %q", result.ChangedPaths[1])
	}
}
