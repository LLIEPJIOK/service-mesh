package injector

import (
	"io"
	"log"
	"strings"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LLIEPJIOK/service-mesh/hook/internal/config"
)

func TestBuildPatchSkipsIgnoredNamespace(t *testing.T) {
	svc := newTestService()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "kube-system"}}
	request := &admissionv1.AdmissionRequest{Namespace: "kube-system"}

	decision, err := svc.BuildPatch(request, pod)
	if err != nil {
		t.Fatalf("BuildPatch() error = %v", err)
	}

	if decision.Mutated {
		t.Fatalf("BuildPatch() expected no mutation")
	}

	if decision.SkipReason == "" {
		t.Fatalf("BuildPatch() expected skip reason")
	}
}

func TestBuildPatchSkipsOptOutPod(t *testing.T) {
	svc := newTestService()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "demo",
			Namespace:   "default",
			Annotations: map[string]string{annotationInject: "false"},
		},
	}
	request := &admissionv1.AdmissionRequest{Namespace: "default"}

	decision, err := svc.BuildPatch(request, pod)
	if err != nil {
		t.Fatalf("BuildPatch() error = %v", err)
	}

	if decision.Mutated {
		t.Fatalf("BuildPatch() expected no mutation")
	}

	if decision.SkipReason == "" {
		t.Fatalf("BuildPatch() expected skip reason")
	}
}

func TestBuildPatchInjectsExpectedResources(t *testing.T) {
	svc := newTestService()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-app",
			Namespace: "default",
			Labels: map[string]string{
				"app": "demo-app",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "demo:v1",
				Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
			}},
		},
	}

	request := &admissionv1.AdmissionRequest{Namespace: "default"}

	decision, err := svc.BuildPatch(request, pod)
	if err != nil {
		t.Fatalf("BuildPatch() error = %v", err)
	}

	if !decision.Mutated {
		t.Fatalf("BuildPatch() expected mutation")
	}

	patch := string(decision.Patch)
	mustContain := []string{
		"/spec/serviceAccountName",
		"demo-app",
		"iptables-init",
		"sidecar",
		"mesh-ca",
		"prometheus.io/scrape",
	}

	for _, fragment := range mustContain {
		if !strings.Contains(patch, fragment) {
			t.Fatalf("patch does not contain %q: %s", fragment, patch)
		}
	}
}

func TestBuildPatchIsIdempotent(t *testing.T) {
	svc := newTestService()

	uid := int64(1337)
	runAsNonRoot := true

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-app",
			Namespace: "default",
			Annotations: map[string]string{
				annotationInjected:         "true",
				annotationVersion:          "v0.1.0",
				annotationPrometheusScrape: "true",
				annotationPrometheusPort:   "9090",
				annotationPrometheusPath:   "/metrics",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "demo-app",
			InitContainers: []corev1.Container{{
				Name: containerNameIptables,
			}},
			Containers: []corev1.Container{
				{Name: "app", Image: "demo:v1"},
				{
					Name: containerNameSidecar,
					SecurityContext: &corev1.SecurityContext{
						RunAsUser:    &uid,
						RunAsNonRoot: &runAsNonRoot,
					},
				},
			},
			Volumes: []corev1.Volume{{Name: volumeNameMeshCA}},
		},
	}

	request := &admissionv1.AdmissionRequest{Namespace: "default"}

	decision, err := svc.BuildPatch(request, pod)
	if err != nil {
		t.Fatalf("BuildPatch() error = %v", err)
	}

	if decision.Mutated {
		t.Fatalf("BuildPatch() expected no mutation")
	}

	if decision.SkipReason != "already injected" {
		t.Fatalf("BuildPatch() unexpected skip reason: %q", decision.SkipReason)
	}
}

func TestDeriveServiceAccountName(t *testing.T) {
	controller := true

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "app label has highest priority",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"app":                    "from-app",
				"app.kubernetes.io/name": "from-k8s-name",
			}}},
			want: "from-app",
		},
		{
			name: "kubernetes app name fallback",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"app.kubernetes.io/name": "from-k8s-name",
			}}},
			want: "from-k8s-name",
		},
		{
			name: "owner reference fallback",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				Name:       "from-owner",
				Controller: &controller,
			}}}},
			want: "from-owner",
		},
		{
			name: "default fallback",
			pod:  &corev1.Pod{},
			want: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveServiceAccountName(tt.pod)
			if got != tt.want {
				t.Fatalf("deriveServiceAccountName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func newTestService() *Service {
	cfg := config.Config{
		IgnoreNamespaces: map[string]struct{}{
			"kube-system": {},
			"mesh-system": {},
		},
		IptablesImage:                  "mesh/iptables-init:latest",
		SidecarImage:                   "mesh/sidecar:latest",
		MeshVersion:                    "v0.1.0",
		MonitoringEnabled:              true,
		MetricsPort:                    9090,
		InboundPlainPort:               15006,
		OutboundPort:                   15002,
		InboundMTLSPort:                15001,
		ExcludeInbound:                 "9090",
		ExcludeOutbound:                "169.254.169.254/32",
		SidecarUID:                     1337,
		LoadBalancerAlgorithm:          "roundRobin",
		RetryAttempts:                  3,
		ConnectTimeout:                 5 * time.Second,
		CircuitBreakerFailureThreshold: 5,
		CircuitBreakerRecoveryTime:     30 * time.Second,
	}

	return NewService(cfg, log.New(io.Discard, "", 0))
}
