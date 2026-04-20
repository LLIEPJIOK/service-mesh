package injector

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/LLIEPJIOK/service-mesh/hook/internal/config"
)

const (
	annotationInject           = "sidecar.mesh.io/inject"
	annotationInjected         = "sidecar.mesh.io/injected"
	annotationVersion          = "sidecar.mesh.io/version"
	annotationPrometheusScrape = "prometheus.io/scrape"
	annotationPrometheusPort   = "prometheus.io/port"
	annotationPrometheusPath   = "prometheus.io/path"

	containerNameIptables = "iptables-init"
	containerNameSidecar  = "sidecar"
	volumeNameMeshCA      = "mesh-ca"
)

type Service struct {
	cfg    config.Config
	logger *log.Logger
}

type Decision struct {
	Patch      []byte
	Mutated    bool
	SkipReason string
}

type patchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func NewService(cfg config.Config, logger *log.Logger) *Service {
	return &Service{cfg: cfg, logger: logger}
}

func (s *Service) BuildPatch(request *admissionv1.AdmissionRequest, pod *corev1.Pod) (Decision, error) {
	if request == nil {
		return Decision{}, fmt.Errorf("admission request is nil")
	}

	if pod == nil {
		return Decision{}, fmt.Errorf("pod is nil")
	}

	namespace := strings.TrimSpace(request.Namespace)
	if namespace == "" {
		namespace = strings.TrimSpace(pod.Namespace)
	}

	if s.isIgnoredNamespace(namespace) {
		s.logger.Printf("skip pod in ignored namespace %q", namespace)
		return Decision{SkipReason: "ignored namespace"}, nil
	}

	if isOptOutPod(pod) {
		s.logger.Printf("skip pod %q/%q due to opt-out annotation", namespace, pod.Name)
		return Decision{SkipReason: "opt-out annotation"}, nil
	}

	serviceAccountName := strings.TrimSpace(pod.Spec.ServiceAccountName)
	if serviceAccountName == "" {
		serviceAccountName = deriveServiceAccountName(pod)
		s.logger.Printf(
			"derived service account for pod %q/%q operation=%s serviceAccount=%q",
			namespace,
			pod.Name,
			request.Operation,
			serviceAccountName,
		)
	} else {
		s.logger.Printf(
			"using explicit service account for pod %q/%q operation=%s serviceAccount=%q",
			namespace,
			pod.Name,
			request.Operation,
			serviceAccountName,
		)
	}

	operations := make([]patchOperation, 0, 12)

	if strings.TrimSpace(pod.Spec.ServiceAccountName) == "" {
		operations = append(operations, patchOperation{
			Op:    "add",
			Path:  "/spec/serviceAccountName",
			Value: serviceAccountName,
		})
	}

	annotations := map[string]string{
		annotationInjected: "true",
		annotationVersion:  s.cfg.MeshVersion,
	}
	if s.cfg.MonitoringEnabled {
		annotations[annotationPrometheusScrape] = "true"
		annotations[annotationPrometheusPort] = strconv.Itoa(s.cfg.MetricsPort)
		annotations[annotationPrometheusPath] = "/metrics"
	}

	operations = append(operations, buildAnnotationsPatchOps(pod.Annotations, annotations)...)

	if !hasVolumeByName(pod.Spec.Volumes, volumeNameMeshCA) {
		s.logger.Printf("adding mesh-ca secret volume for pod %q/%q secret=%q", namespace, pod.Name, "mesh-root-ca")
		meshCAVolume := corev1.Volume{
			Name: volumeNameMeshCA,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "mesh-root-ca",
					Items: []corev1.KeyToPath{{
						Key:  "tls.crt",
						Path: "ca.crt",
					}},
				},
			},
		}

		if len(pod.Spec.Volumes) == 0 {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/spec/volumes",
				Value: []corev1.Volume{meshCAVolume},
			})
		} else {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/spec/volumes/-",
				Value: meshCAVolume,
			})
		}
	} else {
		s.logger.Printf("mesh-ca volume already exists for pod %q/%q", namespace, pod.Name)
	}

	uid := s.cfg.SidecarUID
	uidString := strconv.FormatInt(uid, 10)
	inboundPorts := collectInboundPorts(pod.Spec.Containers)
	appTargetAddr := deriveAppTargetAddr(inboundPorts)
	if inboundPorts == "" {
		s.logger.Printf("no application container ports detected for pod %q/%q", namespace, pod.Name)
	}

	if !hasContainerByName(pod.Spec.InitContainers, containerNameIptables) {
		initContainer := s.buildIptablesContainer(inboundPorts, uidString)

		if len(pod.Spec.InitContainers) == 0 {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/spec/initContainers",
				Value: []corev1.Container{initContainer},
			})
		} else {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/spec/initContainers/0",
				Value: initContainer,
			})
		}
	}

	if !hasContainerByName(pod.Spec.Containers, containerNameSidecar) {
		sidecar := s.buildSidecarContainer(serviceAccountName, uid, appTargetAddr)

		if len(pod.Spec.Containers) == 0 {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/spec/containers",
				Value: []corev1.Container{sidecar},
			})
		} else {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/spec/containers/-",
				Value: sidecar,
			})
		}
	}

	if len(operations) == 0 {
		s.logger.Printf("skip pod %q/%q because it is already injected", namespace, pod.Name)
		return Decision{SkipReason: "already injected"}, nil
	}

	patch, err := json.Marshal(operations)
	if err != nil {
		return Decision{}, fmt.Errorf("marshal json patch: %w", err)
	}

	s.logger.Printf(
		"inject sidecar into pod %q/%q operation=%s serviceAccount=%q inboundPorts=%q appTarget=%q",
		namespace,
		pod.Name,
		request.Operation,
		serviceAccountName,
		inboundPorts,
		appTargetAddr,
	)

	return Decision{Patch: patch, Mutated: true}, nil
}

func (s *Service) isIgnoredNamespace(namespace string) bool {
	_, exists := s.cfg.IgnoreNamespaces[namespace]
	return exists
}

func isOptOutPod(pod *corev1.Pod) bool {
	if pod == nil || pod.Annotations == nil {
		return false
	}

	value, exists := pod.Annotations[annotationInject]
	if !exists {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(value), "false")
}

func deriveServiceAccountName(pod *corev1.Pod) string {
	if pod == nil {
		return "default"
	}

	if value := strings.TrimSpace(pod.Labels["app"]); value != "" {
		return value
	}

	if value := strings.TrimSpace(pod.Labels["app.kubernetes.io/name"]); value != "" {
		return value
	}

	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			if value := strings.TrimSpace(owner.Name); value != "" {
				return value
			}
		}
	}

	return "default"
}

func buildAnnotationsPatchOps(existing map[string]string, desired map[string]string) []patchOperation {
	if len(desired) == 0 {
		return nil
	}

	if existing == nil {
		copyMap := make(map[string]string, len(desired))
		for key, value := range desired {
			copyMap[key] = value
		}

		return []patchOperation{{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: copyMap,
		}}
	}

	keys := make([]string, 0, len(desired))
	for key := range desired {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	operations := make([]patchOperation, 0, len(keys))
	for _, key := range keys {
		desiredValue := desired[key]
		currentValue, exists := existing[key]
		if !exists {
			operations = append(operations, patchOperation{
				Op:    "add",
				Path:  "/metadata/annotations/" + escapeJSONPointer(key),
				Value: desiredValue,
			})
			continue
		}

		if currentValue != desiredValue {
			operations = append(operations, patchOperation{
				Op:    "replace",
				Path:  "/metadata/annotations/" + escapeJSONPointer(key),
				Value: desiredValue,
			})
		}
	}

	return operations
}

func hasContainerByName(containers []corev1.Container, name string) bool {
	for _, container := range containers {
		if container.Name == name {
			return true
		}
	}

	return false
}

func hasVolumeByName(volumes []corev1.Volume, name string) bool {
	for _, volume := range volumes {
		if volume.Name == name {
			return true
		}
	}

	return false
}

func collectInboundPorts(containers []corev1.Container) string {
	unique := make(map[int32]struct{})
	for _, container := range containers {
		if container.Name == containerNameSidecar {
			continue
		}

		for _, port := range container.Ports {
			if port.ContainerPort <= 0 {
				continue
			}
			unique[port.ContainerPort] = struct{}{}
		}
	}

	if len(unique) == 0 {
		return ""
	}

	ports := make([]int, 0, len(unique))
	for port := range unique {
		ports = append(ports, int(port))
	}
	sort.Ints(ports)

	values := make([]string, 0, len(ports))
	for _, port := range ports {
		values = append(values, strconv.Itoa(port))
	}

	return strings.Join(values, ",")
}

func escapeJSONPointer(value string) string {
	replaced := strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(replaced, "/", "~1")
}

func (s *Service) buildIptablesContainer(inboundPorts string, uid string) corev1.Container {
	return corev1.Container{
		Name:            containerNameIptables,
		Image:           s.cfg.IptablesImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}},
		},
		Env: []corev1.EnvVar{
			{Name: "INBOUND_PORTS", Value: inboundPorts},
			{Name: "INBOUND_PLAIN_PORT", Value: strconv.Itoa(s.cfg.InboundPlainPort)},
			{Name: "OUTBOUND_PORT", Value: strconv.Itoa(s.cfg.OutboundPort)},
			{Name: "EXCLUDE_INBOUND_PORTS", Value: s.cfg.ExcludeInbound},
			{Name: "EXCLUDE_OUTBOUND_IPS", Value: s.cfg.ExcludeOutbound},
			{Name: "UID", Value: uid},
		},
	}
}

func (s *Service) buildSidecarContainer(serviceAccountName string, uid int64, appTargetAddr string) corev1.Container {
	runAsNonRoot := true

	return corev1.Container{
		Name:            containerNameSidecar,
		Image:           s.cfg.SidecarImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot: &runAsNonRoot,
			RunAsUser:    &uid,
		},
		Ports: []corev1.ContainerPort{
			{Name: "mesh-mtls", ContainerPort: int32(s.cfg.InboundMTLSPort)},
			{Name: "mesh-outbound", ContainerPort: int32(s.cfg.OutboundPort)},
			{Name: "mesh-inbound", ContainerPort: int32(s.cfg.InboundPlainPort)},
			{Name: "mesh-metrics", ContainerPort: int32(s.cfg.MetricsPort), Protocol: corev1.ProtocolTCP},
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
				},
			},
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
				},
			},
			{Name: "SERVICE_ACCOUNT", Value: serviceAccountName},
			{Name: "APP_TARGET_ADDR", Value: appTargetAddr},
			{Name: "INBOUND_PLAIN_PORT", Value: strconv.Itoa(s.cfg.InboundPlainPort)},
			{Name: "OUTBOUND_PORT", Value: strconv.Itoa(s.cfg.OutboundPort)},
			{Name: "INBOUND_MTLS_PORT", Value: strconv.Itoa(s.cfg.InboundMTLSPort)},
			{Name: "METRICS_PORT", Value: strconv.Itoa(s.cfg.MetricsPort)},
			{Name: "CERT_FILE", Value: "/etc/mesh/certs/tls.crt"},
			{Name: "KEY_FILE", Value: "/etc/mesh/certs/tls.key"},
			{Name: "CA_FILE", Value: "/etc/mesh/ca/ca.crt"},
			{Name: "LOAD_BALANCER_ALGORITHM", Value: s.cfg.LoadBalancerAlgorithm},
			{Name: "RETRY_ATTEMPTS", Value: strconv.Itoa(s.cfg.RetryAttempts)},
			{Name: "TIMEOUT", Value: s.cfg.ConnectTimeout.String()},
			{Name: "CIRCUIT_BREAKER_FAILURE_THRESHOLD", Value: strconv.Itoa(s.cfg.CircuitBreakerFailureThreshold)},
			{Name: "CIRCUIT_BREAKER_RECOVERY_TIME", Value: s.cfg.CircuitBreakerRecoveryTime.String()},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeNameMeshCA, MountPath: "/etc/mesh/ca", ReadOnly: true},
		},
	}
}

func deriveAppTargetAddr(inboundPorts string) string {
	inboundPorts = strings.TrimSpace(inboundPorts)
	if inboundPorts == "" {
		return "127.0.0.1:8080"
	}

	first := inboundPorts
	if idx := strings.Index(inboundPorts, ","); idx >= 0 {
		first = inboundPorts[:idx]
	}

	first = strings.TrimSpace(first)
	if first == "" {
		return "127.0.0.1:8080"
	}

	return "127.0.0.1:" + first
}
