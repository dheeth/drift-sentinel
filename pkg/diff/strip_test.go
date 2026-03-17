package diff

import "testing"

func TestStripRemovesMatchingPaths(t *testing.T) {
	patterns, err := ParsePatterns([]string{
		"status",
		"metadata.resourceVersion",
		"spec.template.spec.containers[*].image",
	})
	if err != nil {
		t.Fatalf("ParsePatterns returned error: %v", err)
	}

	input := map[string]any{
		"status": map[string]any{"readyReplicas": 1},
		"metadata": map[string]any{
			"name":            "api",
			"resourceVersion": "123",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name":  "api",
							"image": "v1",
						},
					},
				},
			},
		},
	}

	output := Strip(input, patterns)
	if _, ok := output["status"]; ok {
		t.Fatal("expected status to be removed")
	}

	metadata := output["metadata"].(map[string]any)
	if _, ok := metadata["resourceVersion"]; ok {
		t.Fatal("expected metadata.resourceVersion to be removed")
	}

	container := output["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	if _, ok := container["image"]; ok {
		t.Fatal("expected container image to be removed")
	}
	if container["name"] != "api" {
		t.Fatalf("unexpected container name: %#v", container["name"])
	}
}
