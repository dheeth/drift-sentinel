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

	logArgs := []any{
		"uid", review.Request.UID,
		"operation", review.Request.Operation,
		"namespace", review.Request.Namespace,
		"resource", resource,
		"name", review.Request.Name,
		"user", review.Request.UserInfo.Username,
		"rule", decision.RuleName,
		"mode", decision.Mode,
		"result", decision.Result,
		"reason", decision.Reason,
		"changed_fields", decision.ChangedPaths,
		"latency_ms", duration.Milliseconds(),
	}
	switch {
	case !decision.Allowed:
		h.logger.Error("admission decision", logArgs...)
	case len(decision.Warnings) > 0:
		h.logger.Warn("admission decision", logArgs...)
	default:
		h.logger.Info("admission decision", logArgs...)
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
