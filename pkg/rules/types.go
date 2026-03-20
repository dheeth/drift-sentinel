package rules

const (
	RuleAnnotationKey       = "drift-sentinel.k8s.io/rule"
	DefaultBypassAnnotation = "drift-sentinel.k8s.io/bypass"
	NamespaceModeAnnotation = "drift-sentinel.k8s.io/mode"
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
	Labels     []string
	Exclude    []string
	Include    []string
	Mutable    []string
	Users      []string
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
	Labels    map[string]string
}
