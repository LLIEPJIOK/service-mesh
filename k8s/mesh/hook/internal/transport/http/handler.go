package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/LLIEPJIOK/service-mesh/hook/internal/app/injector"
)

type Handler struct {
	service         *injector.Service
	maxRequestBytes int64
	logger          *log.Logger
}

func NewHandler(service *injector.Service, maxRequestBytes int64, logger *log.Logger) http.Handler {
	h := &Handler{
		service:         service,
		maxRequestBytes: maxRequestBytes,
		logger:          logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("POST /mutate", h.handleMutate)

	return mux
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) handleMutate(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			h.logger.Printf("close request body: %v", closeErr)
		}
	}()

	reader := http.MaxBytesReader(w, r.Body, h.maxRequestBytes)
	body, err := io.ReadAll(reader)
	if err != nil {
		h.writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("read request body: %w", err))
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		h.writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("decode admission review: %w", err))
		return
	}

	if review.Request == nil {
		h.writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("admission review request is required"))
		return
	}

	response := &admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	if review.Request.Kind.Kind != "Pod" {
		h.logger.Printf("skip non-pod kind %q", review.Request.Kind.Kind)
		h.writeAdmissionResponse(w, review.Request.UID, response)
		return
	}

	var pod corev1.Pod
	if err := json.Unmarshal(review.Request.Object.Raw, &pod); err != nil {
		response.Allowed = false
		response.Result = &metav1.Status{
			Status:  metav1.StatusFailure,
			Reason:  metav1.StatusReasonBadRequest,
			Code:    http.StatusBadRequest,
			Message: "failed to decode pod object",
		}
		h.writeAdmissionResponse(w, review.Request.UID, response)
		return
	}

	h.logger.Printf(
		"admission request uid=%s operation=%s namespace=%q pod=%q",
		review.Request.UID,
		review.Request.Operation,
		review.Request.Namespace,
		pod.Name,
	)

	decision, err := h.service.BuildPatch(review.Request, &pod)
	if err != nil {
		h.logger.Printf("build patch: %v", err)
		response.Allowed = false
		response.Result = &metav1.Status{
			Status:  metav1.StatusFailure,
			Reason:  metav1.StatusReasonInternalError,
			Code:    http.StatusInternalServerError,
			Message: "failed to build pod mutation patch",
		}
		h.writeAdmissionResponse(w, review.Request.UID, response)
		return
	}

	if decision.Mutated {
		patchType := admissionv1.PatchTypeJSONPatch
		response.PatchType = &patchType
		response.Patch = decision.Patch
		h.logger.Printf("admission mutation applied uid=%s namespace=%q pod=%q", review.Request.UID, review.Request.Namespace, pod.Name)
	} else {
		h.logger.Printf("pod %q/%q not mutated: %s", review.Request.Namespace, pod.Name, decision.SkipReason)
	}

	h.writeAdmissionResponse(w, review.Request.UID, response)
}

func (h *Handler) writeAdmissionResponse(
	w http.ResponseWriter,
	uid types.UID,
	response *admissionv1.AdmissionResponse,
) {
	result := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: response,
	}

	if result.Response != nil {
		result.Response.UID = uid
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Printf("encode admission response: %v", err)
	}
}

func (h *Handler) writeHTTPError(w http.ResponseWriter, status int, err error) {
	h.logger.Printf("request error: %v", err)
	http.Error(w, err.Error(), status)
}
