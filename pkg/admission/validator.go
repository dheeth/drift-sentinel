package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"drift-sentinel/pkg/diff"
	"drift-sentinel/pkg/rules"
)

var implicitExcludePatterns = mustParsePatterns([]string{
	"status",
	"metadata.managedFields",
	"metadata.resourceVersion",
	"metadata.generation",
	"metadata.uid",
	"metadata.creationTimestamp",
	"metadata.selfLink",
})

type Decision struct {
	Allowed      bool
	StatusCode   int32
	Reason       string
	Result       string
	ChangedPaths []string
	DeniedPaths  []string
	RuleName     string
	Mode         string
}

type NamespaceModeResolver interface {
	ResolveMode(ctx context.Context, namespace string) (rules.Mode, bool, error)
}

type Validator struct {
	store                 *rules.Store
	namespaceModeResolver NamespaceModeResolver
}

func NewValidator(store *rules.Store, namespaceModeResolver NamespaceModeResolver) *Validator {
	if store == nil {
		store = rules.NewStore()
	}

	return &Validator{
		store:                 store,
		namespaceModeResolver: namespaceModeResolver,
	}
}

func (v *Validator) Validate(ctx context.Context, req AdmissionRequest) Decision {
	if req.Operation != "" && req.Operation != "UPDATE" {
		return Decision{
			Allowed: true,
			Reason:  "operation not enforced",
			Result:  "allowed",
		}
	}

	rule, ok := v.store.Match(rules.MatchInput{
		Namespace: req.Namespace,
		APIGroup:  req.Resource.Group,
		Kind:      req.Kind.Kind,
	})
	if !ok {
		return Decision{
			Allowed: true,
			Reason:  "no matching rule",
			Result:  "allowed",
			Mode:    string(rules.ModeOff),
		}
	}

	oldObj, err := decodeObject(req.OldObject)
	if err != nil {
		return errorDecision(rule, 400, fmt.Sprintf("invalid oldObject: %v", err))
	}
	newObj, err := decodeObject(req.Object)
	if err != nil {
		return errorDecision(rule, 400, fmt.Sprintf("invalid object: %v", err))
	}

	if hasTrueAnnotation(newObj, rule.Bypass) {
		return Decision{
			Allowed:  true,
			Reason:   "bypass annotation present",
			Result:   "allowed",
			RuleName: rule.Name,
			Mode:     string(rule.Mode),
		}
	}

	mode := rule.Mode
	if v.namespaceModeResolver != nil && req.Namespace != "" {
		resolvedMode, found, resolveErr := v.namespaceModeResolver.ResolveMode(ctx, req.Namespace)
		if resolveErr != nil {
			return errorDecision(rule, 500, fmt.Sprintf("resolve namespace annotation %q: %v", rules.NamespaceModeAnnotation, resolveErr))
		}
		if found {
			mode = resolvedMode
		}
	}

	scopedOld, scopedNew, err := prepareObjects(oldObj, newObj, *rule)
	if err != nil {
		return errorDecision(rule, 500, err.Error())
	}

	diffResult := diff.Compare(scopedOld, scopedNew)
	if diffResult.Identical {
		return Decision{
			Allowed:  true,
			Reason:   "no changes detected",
			Result:   "allowed",
			RuleName: rule.Name,
			Mode:     string(mode),
		}
	}

	mutablePatterns, err := diff.ParsePatterns(rule.Mutable)
	if err != nil {
		return errorDecision(rule, 500, fmt.Sprintf("parse mutable patterns: %v", err))
	}

	mutableChanged, immutableChanged := partitionChangedPaths(diffResult.ChangedPaths, mutablePatterns)
	if len(immutableChanged) == 0 {
		return Decision{
			Allowed:      true,
			Reason:       "only mutable fields changed",
			Result:       "allowed",
			ChangedPaths: diffResult.ChangedPaths,
			RuleName:     rule.Name,
			Mode:         string(mode),
		}
	}

	reason := "Drift Sentinel: immutable fields changed: " + strings.Join(immutableChanged, ", ")
	decision := Decision{
		ChangedPaths: diffResult.ChangedPaths,
		DeniedPaths:  immutableChanged,
		RuleName:     rule.Name,
		Mode:         string(mode),
	}

	switch mode {
	case rules.ModeEnforce:
		decision.Allowed = false
		decision.StatusCode = 403
		decision.Reason = reason
		decision.Result = "denied"
	case rules.ModeWarn:
		decision.Allowed = true
		decision.Reason = reason
		decision.Result = "allowed"
	case rules.ModeDryRun:
		decision.Allowed = true
		decision.Reason = "would deny: " + reason
		decision.Result = "allowed"
	case rules.ModeOff:
		decision.Allowed = true
		decision.Reason = reason
		decision.Result = "allowed"
	default:
		return errorDecision(rule, 500, fmt.Sprintf("unsupported mode %q", mode))
	}

	diffResult.MutableChanged = mutableChanged
	diffResult.ImmutableChanged = immutableChanged

	return decision
}

func prepareObjects(oldObj, newObj map[string]any, rule rules.Rule) (map[string]any, map[string]any, error) {
	scopedOld := diff.Strip(oldObj, implicitExcludePatterns)
	scopedNew := diff.Strip(newObj, implicitExcludePatterns)

	if len(rule.Include) > 0 {
		includePatterns, err := diff.ParsePatterns(rule.Include)
		if err != nil {
			return nil, nil, fmt.Errorf("parse include patterns: %w", err)
		}
		scopedOld = diff.Extract(scopedOld, includePatterns)
		scopedNew = diff.Extract(scopedNew, includePatterns)
	}

	if len(rule.Exclude) > 0 {
		excludePatterns, err := diff.ParsePatterns(rule.Exclude)
		if err != nil {
			return nil, nil, fmt.Errorf("parse exclude patterns: %w", err)
		}
		scopedOld = diff.Strip(scopedOld, excludePatterns)
		scopedNew = diff.Strip(scopedNew, excludePatterns)
	}

	return scopedOld, scopedNew, nil
}

func partitionChangedPaths(changedPaths []string, mutablePatterns []diff.PathPattern) ([]string, []string) {
	mutableChanged := make([]string, 0)
	immutableChanged := make([]string, 0)

	for _, changedPath := range changedPaths {
		if diff.MatchAny(changedPath, mutablePatterns) {
			mutableChanged = append(mutableChanged, changedPath)
			continue
		}
		immutableChanged = append(immutableChanged, changedPath)
	}

	return mutableChanged, immutableChanged
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if decoded == nil {
		return map[string]any{}, nil
	}

	return decoded, nil
}

func hasTrueAnnotation(obj map[string]any, key string) bool {
	metadata, ok := obj["metadata"].(map[string]any)
	if !ok {
		return false
	}

	annotations, ok := metadata["annotations"].(map[string]any)
	if !ok {
		return false
	}

	value, ok := annotations[key]
	if !ok {
		return false
	}

	switch typed := value.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	case bool:
		return typed
	default:
		return false
	}
}

func errorDecision(rule *rules.Rule, code int32, reason string) Decision {
	decision := Decision{
		Allowed:    false,
		StatusCode: code,
		Reason:     reason,
		Result:     "denied",
		Mode:       string(rules.ModeOff),
	}
	if rule != nil {
		decision.RuleName = rule.Name
		decision.Mode = string(rule.Mode)
	}
	return decision
}

func mustParsePatterns(values []string) []diff.PathPattern {
	patterns, err := diff.ParsePatterns(values)
	if err != nil {
		panic(err)
	}
	return patterns
}
