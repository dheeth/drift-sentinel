package admission

import (
	"encoding/json"
	"fmt"
)

const (
	admissionAPIVersion = "admission.k8s.io/v1"
	admissionKind       = "AdmissionReview"
)

type AdmissionReview struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Request    *AdmissionRequest  `json:"request,omitempty"`
	Response   *AdmissionResponse `json:"response,omitempty"`
}

type AdmissionRequest struct {
	UID       string               `json:"uid"`
	Operation string               `json:"operation"`
	Namespace string               `json:"namespace"`
	Name      string               `json:"name"`
	Resource  GroupVersionResource `json:"resource"`
	Kind      GroupVersionKind     `json:"kind,omitempty"`
	UserInfo  UserInfo             `json:"userInfo,omitempty"`
	Object    json.RawMessage      `json:"object,omitempty"`
	OldObject json.RawMessage      `json:"oldObject,omitempty"`
}

type AdmissionResponse struct {
	UID      string   `json:"uid"`
	Allowed  bool     `json:"allowed"`
	Status   *Status  `json:"status,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type GroupVersionResource struct {
	Group    string `json:"group"`
	Version  string `json:"version"`
	Resource string `json:"resource"`
}

type GroupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

type UserInfo struct {
	Username string `json:"username"`
}

type Status struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
}

func DecodeReview(data []byte) (AdmissionReview, error) {
	var review AdmissionReview
	if err := json.Unmarshal(data, &review); err != nil {
		return AdmissionReview{}, fmt.Errorf("decode admission review: %w", err)
	}

	return review, nil
}

func NewResponse(uid string, allowed bool, code int32, message string, warnings []string) AdmissionReview {
	review := AdmissionReview{
		APIVersion: admissionAPIVersion,
		Kind:       admissionKind,
		Response: &AdmissionResponse{
			UID:      uid,
			Allowed:  allowed,
			Warnings: append([]string(nil), warnings...),
		},
	}

	if message != "" || code != 0 {
		review.Response.Status = &Status{
			Code:    code,
			Message: message,
		}
	}

	return review
}

func EncodeReview(review AdmissionReview) ([]byte, error) {
	if review.APIVersion == "" {
		review.APIVersion = admissionAPIVersion
	}
	if review.Kind == "" {
		review.Kind = admissionKind
	}

	payload, err := json.Marshal(review)
	if err != nil {
		return nil, fmt.Errorf("encode admission review: %w", err)
	}

	return payload, nil
}
