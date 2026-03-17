package rules

const (
	RuleAnnotationKey       = "drift-sentinel.devtron.io/rule"
	DefaultBypassAnnotation = "drift-sentinel.devtron.io/bypass"
	NamespaceModeAnnotation = "drift-sentinel.devtron.io/mode"
)

type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeWarn    Mode = "warn"
	ModeDryRun  Mode = "dry-run"
	ModeOff     Mode = "off"
)

type Rule struct {
	Name       string
	Namespace  string
	Priority   int
	Mode       Mode
	Namespaces []string
	Selectors  []ResourceSelector
	Exclude    []string
	Include    []string
	Mutable    []string
	Bypass     string
}

type ResourceSelector struct {
	APIGroup string
	Kind     string
}

type ConfigMap struct {
	Name        string
	Namespace   string
	Annotations map[string]string
	Data        map[string]string
}

type MatchInput struct {
	Namespace string
	APIGroup  string
	Kind      string
}
