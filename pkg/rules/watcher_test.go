package rules

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"drift-sentinel/pkg/metrics"
)

func TestControllerLoadsAnnotatedRulesAtStartup(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rule-a",
				Namespace: "team-a",
				Annotations: map[string]string{
					RuleAnnotationKey: "true",
				},
			},
			Data: map[string]string{
				"spec": `
mode: enforce
priority: 100
namespaces: ["prod-*"]
selectors:
  - apiGroup: "apps"
    kind: "Deployment"
mutable:
  - "spec.template.spec.containers[*].image"
`,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ignored",
				Namespace: "team-a",
			},
			Data: map[string]string{
				"spec": `mode: enforce`,
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prod-apps",
				Annotations: map[string]string{
					NamespaceModeAnnotation: string(ModeWarn),
				},
			},
		},
	)

	store := NewStore()
	controller := NewController(client, store, nil, metrics.NewRegistry(), 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := controller.Start(ctx, 5*time.Second); err != nil {
		t.Fatalf("controller start failed: %v", err)
	}

	rulesSnapshot := store.Snapshot()
	if len(rulesSnapshot) != 1 {
		t.Fatalf("unexpected rule count: %d", len(rulesSnapshot))
	}
	if rulesSnapshot[0].Name != "rule-a" {
		t.Fatalf("unexpected rule name: %q", rulesSnapshot[0].Name)
	}

	mode, found, err := controller.NamespaceModeResolver().ResolveMode(ctx, "prod-apps")
	if err != nil {
		t.Fatalf("namespace mode resolve failed: %v", err)
	}
	if !found {
		t.Fatal("expected namespace mode override to be found")
	}
	if mode != ModeWarn {
		t.Fatalf("unexpected namespace mode: %q", mode)
	}
}

func TestControllerReloadsRulesOnConfigMapCreate(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := NewStore()
	controller := NewController(client, store, nil, metrics.NewRegistry(), 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := controller.Start(ctx, 5*time.Second); err != nil {
		t.Fatalf("controller start failed: %v", err)
	}

	if _, err := client.CoreV1().ConfigMaps("team-a").Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-b",
			Namespace: "team-a",
			Annotations: map[string]string{
				RuleAnnotationKey: "true",
			},
		},
		Data: map[string]string{
			"spec": `
mode: enforce
priority: 50
namespaces: ["staging"]
selectors:
  - apiGroup: "apps"
    kind: "Deployment"
`,
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("configmap create failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(store.Snapshot()) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for rule reload, snapshot=%d", len(store.Snapshot()))
		}
		time.Sleep(25 * time.Millisecond)
	}
}
