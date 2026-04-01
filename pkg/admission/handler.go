package admission

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"drift-sentinel/pkg/metrics"
)

type Handler struct {
	logger    *slog.Logger
	validator *Validator
	metrics   *metrics.Registry
}

func NewHandler(logger *slog.Logger, validator *Validator, registry *metrics.Registry) *Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Handler{
		logger:    logger,
		validator: validator,
		metrics:   registry,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	review, err := DecodeReview(body)
	if err != nil {
		http.Error(w, "invalid admission review", http.StatusBadRequest)
		return
	}

	if review.Request == nil {
		h.writeAdmissionResponse(w, NewResponse("", false, 400, "missing admission request", nil))
		return
	}

	decision := h.validator.Validate(r.Context(), *review.Request)
	resource := formatResource(review.Request.Resource)
	duration := time.Since(start)
	h.metrics.RecordAdmission(review.Request.Namespace, resource, decision.Result, duration)

	if !decision.Allowed {
		for _, path := range decision.DeniedPaths {
			h.metrics.RecordViolation(review.Request.Namespace, resource, path, decision.Mode)
		}
	}

	logAttrs := []slog.Attr{
		slog.String("uid", review.Request.UID),
		slog.String("operation", review.Request.Operation),
		slog.String("namespace", review.Request.Namespace),
		slog.String("resource", resource),
		slog.String("name", review.Request.Name),
		slog.String("user", review.Request.UserInfo.Username),
		slog.String("rule", decision.RuleName),
		slog.String("mode", decision.Mode),
		slog.String("result", decision.Result),
		slog.String("reason", decision.Reason),
		slog.Any("changed_fields", decision.ChangedPaths),
		slog.Int64("latency_ms", duration.Milliseconds()),
	}
	switch {
	case !decision.Allowed:
		h.logger.LogAttrs(r.Context(), slog.LevelError, "admission decision", logAttrs...)
	case len(decision.Warnings) > 0:
		h.logger.LogAttrs(r.Context(), slog.LevelWarn, "admission decision", logAttrs...)
	default:
		h.logger.LogAttrs(r.Context(), slog.LevelInfo, "admission decision", logAttrs...)
	}

	code := int32(0)
	if !decision.Allowed {
		code = decision.StatusCode
		if code == 0 {
			code = 403
		}
	}

	responseMessage := ""
	if !decision.Allowed {
		responseMessage = decision.Reason
	}

	h.writeAdmissionResponse(w, NewResponse(review.Request.UID, decision.Allowed, code, responseMessage, decision.Warnings))
}

func (h *Handler) writeAdmissionResponse(w http.ResponseWriter, review AdmissionReview) {
	payload, err := EncodeReview(review)
	if err != nil {
		http.Error(w, "failed to encode admission response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func formatResource(resource GroupVersionResource) string {
	if resource.Group == "" {
		return resource.Version + "/" + resource.Resource
	}

	return resource.Group + "/" + resource.Version + "/" + resource.Resource
}
