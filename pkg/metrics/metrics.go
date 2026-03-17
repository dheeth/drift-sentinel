package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var histogramBuckets = []float64{
	0.005,
	0.01,
	0.025,
	0.05,
	0.1,
	0.25,
	0.5,
	1,
	2.5,
	5,
}

type Registry struct {
	mu sync.RWMutex

	admissionRequests map[string]*counterSample
	violations        map[string]*counterSample
	configEvents      map[string]*counterSample
	rulesLoaded       int
	durationCount     uint64
	durationSum       float64
	durationBuckets   []uint64
}

type counterSample struct {
	labels map[string]string
	value  uint64
}

func NewRegistry() *Registry {
	return &Registry{
		admissionRequests: make(map[string]*counterSample),
		violations:        make(map[string]*counterSample),
		configEvents:      make(map[string]*counterSample),
		durationBuckets:   make([]uint64, len(histogramBuckets)),
	}
}

func (r *Registry) RecordAdmission(namespace, resource, result string, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCounter(r.admissionRequests, map[string]string{
		"namespace": namespaceOrUnknown(namespace),
		"resource":  namespaceOrUnknown(resource),
		"result":    namespaceOrUnknown(result),
	})

	seconds := duration.Seconds()
	r.durationCount++
	r.durationSum += seconds
	for i, bucket := range histogramBuckets {
		if seconds <= bucket {
			r.durationBuckets[i]++
		}
	}
}

func (r *Registry) RecordViolation(namespace, resource, field, mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCounter(r.violations, map[string]string{
		"namespace": namespaceOrUnknown(namespace),
		"resource":  namespaceOrUnknown(resource),
		"field":     namespaceOrUnknown(field),
		"mode":      namespaceOrUnknown(mode),
	})
}

func (r *Registry) RecordConfigEvent(eventType string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incrementCounter(r.configEvents, map[string]string{
		"event_type": namespaceOrUnknown(eventType),
	})
}

func (r *Registry) SetRulesLoaded(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rulesLoaded = count
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.render()))
	})
}

func (r *Registry) render() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var builder strings.Builder

	writeCounterMetric(&builder,
		"drift_sentinel_admission_requests_total",
		"Total admission requests processed.",
		r.admissionRequests,
	)
	writeCounterMetric(&builder,
		"drift_sentinel_violations_total",
		"Total immutable field violations detected.",
		r.violations,
	)

	builder.WriteString("# HELP drift_sentinel_admission_duration_seconds Admission request processing latency.\n")
	builder.WriteString("# TYPE drift_sentinel_admission_duration_seconds histogram\n")
	var cumulative uint64
	for i, bucket := range histogramBuckets {
		cumulative += r.durationBuckets[i]
		builder.WriteString(fmt.Sprintf(
			"drift_sentinel_admission_duration_seconds_bucket{le=%q} %d\n",
			strconv.FormatFloat(bucket, 'f', -1, 64),
			cumulative,
		))
	}
	builder.WriteString(fmt.Sprintf(
		"drift_sentinel_admission_duration_seconds_bucket{le=%q} %d\n",
		"+Inf",
		r.durationCount,
	))
	builder.WriteString(fmt.Sprintf("drift_sentinel_admission_duration_seconds_sum %g\n", r.durationSum))
	builder.WriteString(fmt.Sprintf("drift_sentinel_admission_duration_seconds_count %d\n", r.durationCount))

	builder.WriteString("# HELP drift_sentinel_rules_loaded_total Active rules loaded.\n")
	builder.WriteString("# TYPE drift_sentinel_rules_loaded_total gauge\n")
	builder.WriteString(fmt.Sprintf("drift_sentinel_rules_loaded_total %d\n", r.rulesLoaded))

	writeCounterMetric(&builder,
		"drift_sentinel_config_events_total",
		"ConfigMap watch events processed.",
		r.configEvents,
	)

	return builder.String()
}

func (r *Registry) incrementCounter(target map[string]*counterSample, labels map[string]string) {
	key := labelKey(labels)
	sample, ok := target[key]
	if !ok {
		sample = &counterSample{labels: cloneLabels(labels)}
		target[key] = sample
	}
	sample.value++
}

func writeCounterMetric(builder *strings.Builder, name, help string, samples map[string]*counterSample) {
	builder.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
	builder.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))

	keys := make([]string, 0, len(samples))
	for key := range samples {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		sample := samples[key]
		builder.WriteString(name)
		if len(sample.labels) > 0 {
			builder.WriteString("{")
			builder.WriteString(formatLabels(sample.labels))
			builder.WriteString("}")
		}
		builder.WriteString(fmt.Sprintf(" %d\n", sample.value))
	}
}

func labelKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}

	return strings.Join(parts, ",")
}

func formatLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, escapeLabelValue(labels[key])))
	}

	return strings.Join(parts, ",")
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\n", "\\n",
		"\"", "\\\"",
	)
	return replacer.Replace(value)
}

func cloneLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	return result
}

func namespaceOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}

	return value
}
