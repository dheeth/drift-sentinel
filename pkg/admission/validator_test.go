package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"drift-sentinel/pkg/metrics"
	"drift-sentinel/pkg/rules"
)

func TestValidatorAllowsWhenNoRuleMatches(t *testing.T) {
	validator := NewValidator(rules.NewStore(), nil)

	decision := validator.Validate(context.Background(), AdmissionRequest{
		Operation: "UPDATE",
		Namespace: "prod-apps",
		Resource: GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		Kind: GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		},
	})

	if !decision.Allowed {
		t.Fatal("expected request to be allowed")
	}
	if decision.Reason != "no matching rule" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestValidatorAllowsMutableImageChange(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Mutable: []string{"spec.template.spec.containers[*].image"},
			Bypass:  rules.DefaultBypassAnnotation,
		},
	})

	decision := NewValidator(store, nil).Validate(context.Background(), newDeploymentRequest(t, deploymentObject("v1", 2, nil), deploymentObject("v2", 2, nil)))
	if !decision.Allowed {
		t.Fatalf("expected mutable image change to be allowed: %s", decision.Reason)
	}
	if decision.Reason != "only mutable fields changed" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestValidatorDeniesImmutableReplicaChange(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Mutable: []string{"spec.template.spec.containers[*].image"},
			Bypass:  rules.DefaultBypassAnnotation,
		},
	})

	decision := NewValidator(store, nil).Validate(context.Background(), newDeploymentRequest(t, deploymentObject("v1", 2, nil), deploymentObject("v1", 3, nil)))
	if decision.Allowed {
		t.Fatal("expected replica change to be denied")
	}
	if decision.StatusCode != 403 {
		t.Fatalf("unexpected status code: %d", decision.StatusCode)
	}
	if len(decision.DeniedPaths) != 1 || decision.DeniedPaths[0] != "spec.replicas" {
		t.Fatalf("unexpected denied paths: %#v", decision.DeniedPaths)
	}
}

func TestValidatorSupportsBypassAnnotation(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Bypass: rules.DefaultBypassAnnotation,
		},
	})

	decision := NewValidator(store, nil).Validate(context.Background(), newDeploymentRequest(t, deploymentObject("v1", 2, nil), deploymentObject("v1", 3, map[string]string{
		rules.DefaultBypassAnnotation: "true",
	})))
	if !decision.Allowed {
		t.Fatalf("expected bypassed request to be allowed: %s", decision.Reason)
	}
	if decision.Reason != "bypass annotation present" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestValidatorAllowsStandaloneBypassRemoval(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Bypass: rules.DefaultBypassAnnotation,
		},
	})

	decision := NewValidator(store, nil).Validate(context.Background(), newDeploymentRequest(t,
		deploymentObject("v1", 2, map[string]string{
			rules.DefaultBypassAnnotation: "true",
		}),
		deploymentObject("v1", 2, nil),
	))
	if !decision.Allowed {
		t.Fatalf("expected standalone bypass removal to be allowed: %s", decision.Reason)
	}
	if decision.Reason != "bypass annotation removed" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestValidatorDeniesBypassRemovalWithOtherChanges(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Mutable: []string{"spec.template.spec.containers[*].image"},
			Bypass:  rules.DefaultBypassAnnotation,
		},
	})

	decision := NewValidator(store, nil).Validate(context.Background(), newDeploymentRequest(t,
		deploymentObject("v1", 2, map[string]string{
			rules.DefaultBypassAnnotation: "true",
		}),
		deploymentObject("v2", 2, nil),
	))
	if decision.Allowed {
		t.Fatal("expected bypass removal with other changes to be denied")
	}
	if decision.Reason != "bypass annotation removal must be the only change in the request" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestValidatorAppliesNamespaceModeOverride(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Bypass: rules.DefaultBypassAnnotation,
		},
	})

	resolver := staticNamespaceModeResolver{
		mode: rules.ModeWarn,
	}

	decision := NewValidator(store, resolver).Validate(context.Background(), newDeploymentRequest(t, deploymentObject("v1", 2, nil), deploymentObject("v1", 3, nil)))
	if !decision.Allowed {
		t.Fatalf("expected warn override to allow request: %s", decision.Reason)
	}
	if decision.Mode != string(rules.ModeWarn) {
		t.Fatalf("unexpected effective mode: %q", decision.Mode)
	}
	if len(decision.Warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", decision.Warnings)
	}
}

func TestValidatorIgnoresImplicitSystemFields(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeEnforce,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Bypass: rules.DefaultBypassAnnotation,
		},
	})

	oldObj := deploymentObject("v1", 2, nil)
	newObj := deploymentObject("v1", 2, nil)
	oldMetadata := oldObj["metadata"].(map[string]any)
	newMetadata := newObj["metadata"].(map[string]any)
	oldMetadata["resourceVersion"] = "1"
	newMetadata["resourceVersion"] = "2"

	decision := NewValidator(store, nil).Validate(context.Background(), newDeploymentRequest(t, oldObj, newObj))
	if !decision.Allowed {
		t.Fatalf("expected resourceVersion drift to be ignored: %s", decision.Reason)
	}
	if decision.Reason != "no changes detected" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestHandlerWritesAdmissionWarnings(t *testing.T) {
	store := rules.NewStore()
	store.Replace([]rules.Rule{
		{
			Name:       "prod",
			Namespace:  "drift-system",
			Priority:   100,
			Mode:       rules.ModeWarn,
			Namespaces: []string{"prod-*"},
			Selectors: []rules.ResourceSelector{
				{APIGroup: "apps", Kind: "Deployment"},
			},
			Bypass: rules.DefaultBypassAnnotation,
		},
	})

	handler := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewValidator(store, nil),
		metrics.NewRegistry(),
	)

	admissionRequest := newDeploymentRequest(t, deploymentObject("v1", 2, nil), deploymentObject("v1", 3, nil))
	reviewBytes, err := EncodeReview(AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request:    &admissionRequest,
	})
	if err != nil {
		t.Fatalf("encode review: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(reviewBytes))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}

	responseReview, err := DecodeReview(recorder.Body.Bytes())
	if err != nil {
		t.Fatalf("decode response review: %v", err)
	}
	if responseReview.Response == nil {
		t.Fatal("expected response")
	}
	if len(responseReview.Response.Warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", responseReview.Response.Warnings)
	}
}

type staticNamespaceModeResolver struct {
	mode rules.Mode
}

func (s staticNamespaceModeResolver) ResolveMode(_ context.Context, _ string) (rules.Mode, bool, error) {
	return s.mode, true, nil
}

func newDeploymentRequest(t *testing.T, oldObj, newObj map[string]any) AdmissionRequest {
	t.Helper()

	oldPayload, err := json.Marshal(oldObj)
	if err != nil {
		t.Fatalf("marshal old object: %v", err)
	}
	newPayload, err := json.Marshal(newObj)
	if err != nil {
		t.Fatalf("marshal new object: %v", err)
	}

	return AdmissionRequest{
		UID:       "1234",
		Operation: "UPDATE",
		Namespace: "prod-apps",
		Name:      "api-service",
		Resource: GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		Kind: GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		},
		OldObject: oldPayload,
		Object:    newPayload,
	}
}

func deploymentObject(image string, replicas int, annotations map[string]string) map[string]any {
	rawAnnotations := make(map[string]any, len(annotations))
	for key, value := range annotations {
		rawAnnotations[key] = value
	}

	return map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":        "api-service",
			"namespace":   "prod-apps",
			"annotations": rawAnnotations,
		},
		"spec": map[string]any{
			"replicas": replicas,
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name":  "api",
							"image": image,
						},
					},
				},
			},
		},
	}
}
