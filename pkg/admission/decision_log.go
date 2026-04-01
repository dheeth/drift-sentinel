package admission

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type decisionLogger struct {
	mu  sync.Mutex
	out io.Writer
}

type decisionLogEntry struct {
	Level         string   `json:"level"`
	Timestamp     string   `json:"ts"`
	Message       string   `json:"msg"`
	UID           string   `json:"uid"`
	Operation     string   `json:"operation"`
	Namespace     string   `json:"namespace"`
	Resource      string   `json:"resource"`
	Name          string   `json:"name"`
	User          string   `json:"user"`
	Rule          string   `json:"rule,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	Result        string   `json:"result"`
	Reason        string   `json:"reason,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
	LatencyMS     int64    `json:"latency_ms"`
}

func newDecisionLogger(out io.Writer) *decisionLogger {
	if out == nil {
		out = io.Discard
	}

	return &decisionLogger{out: out}
}

func (l *decisionLogger) Log(request AdmissionRequest, resource string, decision Decision, duration time.Duration) error {
	entry := decisionLogEntry{
		Level:         decisionLogLevel(decision),
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Message:       "admission decision",
		UID:           request.UID,
		Operation:     request.Operation,
		Namespace:     request.Namespace,
		Resource:      resource,
		Name:          request.Name,
		User:          request.UserInfo.Username,
		Rule:          decision.RuleName,
		Mode:          decision.Mode,
		Result:        decision.Result,
		Reason:        decision.Reason,
		ChangedFields: append([]string(nil), decision.ChangedPaths...),
		LatencyMS:     duration.Milliseconds(),
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.out.Write(payload); err != nil {
		return err
	}
	if _, err := l.out.Write([]byte("\n")); err != nil {
		return err
	}

	return nil
}

func decisionLogLevel(decision Decision) string {
	if !decision.Allowed {
		return "error"
	}
	if len(decision.Warnings) > 0 {
		return "warn"
	}
	return "info"
}
