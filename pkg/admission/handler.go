package admission

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"drift-sentinel/pkg/metrics"
)

type Handler struct {
	logger         *slog.Logger
	decisionLogger *decisionLogger
	validator      *Validator
	metrics        *metrics.Registry
}

func NewHandler(logger *slog.Logger, validator *Validator, registry *metrics.Registry) *Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Handler{
		logger:         logger,
		decisionLogger: newDecisionLogger(os.Stdout),
		validator:      validator,
		metrics:        registry,
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

	if err := h.decisionLogger.Log(*review.Request, resource, decision, duration); err != nil {
		h.logger.Error("failed to write admission decision log", "uid", review.Request.UID, "error", err)
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
