package diff

import "testing"

func TestExtractKeepsOnlyIncludedPaths(t *testing.T) {
	patterns, err := ParsePatterns([]string{
		"spec.replicas",
		"spec.template.spec.containers[*].image",
	})
	if err != nil {
		t.Fatalf("ParsePatterns returned error: %v", err)
	}

	input := map[string]any{
		"metadata": map[string]any{
			"name": "api",
		},
		"spec": map[string]any{
			"replicas": 3,
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

	output := Extract(input, patterns)
	if _, ok := output["metadata"]; ok {
		t.Fatal("did not expect metadata to be included")
	}

	spec := output["spec"].(map[string]any)
	if spec["replicas"] != 3 {
		t.Fatalf("unexpected replicas value: %#v", spec["replicas"])
	}

	container := spec["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	if _, ok := container["name"]; ok {
		t.Fatal("did not expect container name to be included")
	}
	if container["image"] != "v1" {
		t.Fatalf("unexpected container image: %#v", container["image"])
	}
}
