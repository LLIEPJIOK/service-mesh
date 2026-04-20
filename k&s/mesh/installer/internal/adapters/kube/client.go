package kube

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/LLIEPJIOK/service-mesh/installer/internal/config"
)

const (
	namespaceMeshSystem            = "mesh-system"
	rootCASecretName               = "mesh-root-ca"
	webhookTLSSecretName           = "mesh-webhook-tls"
	certManagerServiceAccountName  = "cert-manager"
	certManagerDeploymentName      = "mesh-cert-manager"
	certManagerServiceName         = "mesh-cert-manager"
	sidecarConfigMapName           = "mesh-sidecar-config"
	webhookServiceAccountName      = "mesh-webhook"
	webhookDeploymentName          = "mesh-webhook"
	webhookServiceName             = "mesh-webhook"
	webhookConfigurationName       = "mesh-sidecar-injector"
	certManagerClusterRoleName     = "cert-manager-tokenreviewer"
	certManagerClusterRoleBindName = "cert-manager-tokenreviewer-binding"
)

type Client struct {
	clientset *kubernetes.Clientset
	logger    *log.Logger
}

func NewClient(kubeconfigPath string, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "mesh-installer ", log.LstdFlags|log.LUTC)
	}

	restConfig, _, err := buildRESTConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	return &Client{clientset: clientset, logger: logger}, nil
}

func buildRESTConfig(kubeconfigPath string) (*rest.Config, string, error) {
	path := resolveKubeConfigPath(kubeconfigPath)
	if strings.TrimSpace(path) != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", path)
		if err != nil {
			return nil, "", fmt.Errorf("build config from kubeconfig %q: %w", path, err)
		}
		return cfg, path, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, "", fmt.Errorf("load in-cluster kubeconfig: %w", err)
	}

	return cfg, "", nil
}

func resolveKubeConfigPath(explicit string) string {
	if path := strings.TrimSpace(explicit); path != "" {
		return path
	}

	if path := strings.TrimSpace(os.Getenv("KUBECONFIG")); path != "" {
		return path
	}

	home := homedir.HomeDir()
	if home == "" {
		return ""
	}

	path := filepath.Join(home, ".kube", "config")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return ""
}

func (c *Client) EnsureNamespace(ctx context.Context, namespace string, dryRun bool) error {
	desired := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":    namespace,
				"app.kubernetes.io/part-of": "service-mesh",
			},
		},
	}

	namespaces := c.clientset.CoreV1().Namespaces()
	existing, err := namespaces.Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get namespace %q: %w", namespace, err)
		}
		_, err := namespaces.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create namespace %q: %w", namespace, err)
		}
		return nil
	}

	existing.Labels = mergeStringMaps(existing.Labels, desired.Labels)
	_, err = namespaces.Update(ctx, existing, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update namespace %q: %w", namespace, err)
	}

	return nil
}

func (c *Client) ApplyRootCASecret(ctx context.Context, namespace string, certPEM []byte, keyPEM []byte, dryRun bool) error {
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rootCASecretName,
			Namespace: namespace,
			Labels:    labels("mesh-root-ca"),
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}

	return c.upsertSecret(ctx, desired, dryRun)
}

func (c *Client) ApplyWebhookTLSSecret(ctx context.Context, namespace string, certPEM []byte, keyPEM []byte, dryRun bool) error {
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookTLSSecretName,
			Namespace: namespace,
			Labels:    labels("mesh-webhook"),
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}

	return c.upsertSecret(ctx, desired, dryRun)
}

func (c *Client) ApplySidecarConfigMap(ctx context.Context, cfg config.MeshConfig, namespace string, dryRun bool) error {
	sidecarYAML := fmt.Sprintf(
		"inboundPlainPort: %d\noutboundPort: %d\ninboundMTLSPort: %d\nmetricsPort: %d\nmonitoringEnabled: %t\nloadBalancerAlgorithm: %s\nretryPolicy:\n  attempts: %d\n  backoff:\n    type: %s\n    baseInterval: %s\ntimeout: %s\ncircuitBreakerPolicy:\n  failureThreshold: %d\n  recoveryTime: %s\nexcludeInboundPorts: %s\nexcludeOutboundIPs: %s\n",
		cfg.Spec.Sidecar.InboundPlainPort,
		cfg.Spec.Sidecar.OutboundPort,
		cfg.Spec.Sidecar.InboundMTLSPort,
		cfg.Spec.Sidecar.MetricsPort,
		cfg.Spec.Sidecar.MonitoringEnabled,
		cfg.Spec.Sidecar.LoadBalancerAlgorithm,
		cfg.Spec.Sidecar.RetryPolicy.Attempts,
		cfg.Spec.Sidecar.RetryPolicy.Backoff.Type,
		cfg.Spec.Sidecar.RetryPolicy.Backoff.BaseInterval,
		cfg.Spec.Sidecar.Timeout,
		cfg.Spec.Sidecar.CircuitBreakerPolicy.FailureThreshold,
		cfg.Spec.Sidecar.CircuitBreakerPolicy.RecoveryTime,
		cfg.Spec.Sidecar.ExcludeInboundPorts,
		cfg.Spec.Sidecar.ExcludeOutboundIPs,
	)

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sidecarConfigMapName,
			Namespace: namespace,
			Labels:    labels("mesh-sidecar-config"),
		},
		Data: map[string]string{
			"sidecar.yaml": sidecarYAML,
		},
	}

	return c.upsertConfigMap(ctx, desired, dryRun)
}

func (c *Client) ApplyCertManagerResources(ctx context.Context, cfg config.MeshConfig, namespace string, dryRun bool) error {
	if err := c.upsertServiceAccount(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerServiceAccountName,
			Namespace: namespace,
			Labels:    labels(certManagerDeploymentName),
		},
	}, dryRun); err != nil {
		return err
	}

	if err := c.upsertClusterRole(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: certManagerClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"authentication.k8s.io"},
			Resources: []string{"tokenreviews"},
			Verbs:     []string{"create"},
		}},
	}, dryRun); err != nil {
		return err
	}

	if err := c.upsertClusterRoleBinding(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: certManagerClusterRoleBindName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     certManagerClusterRoleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      certManagerServiceAccountName,
			Namespace: namespace,
		}},
	}, dryRun); err != nil {
		return err
	}

	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerDeploymentName,
			Namespace: namespace,
			Labels:    labels(certManagerDeploymentName),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": certManagerDeploymentName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels(certManagerDeploymentName)},
				Spec: corev1.PodSpec{
					ServiceAccountName: certManagerServiceAccountName,
					SecurityContext:    &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
					Containers: []corev1.Container{{
						Name:            "cert-manager",
						Image:           resolveImage(cfg.Spec.Images.CertManager, "mesh/cert-manager", cfg.Spec.Version),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports:           []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}},
						Env:             []corev1.EnvVar{{Name: "HTTP_ADDR", Value: ":8080"}, {Name: "ROOT_CA_CERT_FILE", Value: "/etc/mesh/ca/tls.crt"}, {Name: "ROOT_CA_KEY_FILE", Value: "/etc/mesh/ca/tls.key"}, {Name: "LEAF_TTL", Value: cfg.Spec.Certificates.Validity}},
						VolumeMounts:    []corev1.VolumeMount{{Name: "mesh-root-ca", MountPath: "/etc/mesh/ca", ReadOnly: true}},
						ReadinessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromString("http")}}, InitialDelaySeconds: 2, PeriodSeconds: 5, TimeoutSeconds: 2, FailureThreshold: 5},
						LivenessProbe:   &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromString("http")}}, InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 2, FailureThreshold: 3},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: boolPtr(false),
							ReadOnlyRootFilesystem:   boolPtr(true),
							RunAsNonRoot:             boolPtr(true),
							RunAsUser:                int64Ptr(1337),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
					}},
					Volumes: []corev1.Volume{{Name: "mesh-root-ca", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: rootCASecretName}}}},
				},
			},
		},
	}

	if err := c.upsertDeployment(ctx, desiredDeployment, dryRun); err != nil {
		return err
	}

	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: certManagerServiceName, Namespace: namespace, Labels: labels(certManagerDeploymentName)},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": certManagerDeploymentName},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP}},
		},
	}

	return c.upsertService(ctx, desiredService, dryRun)
}

func (c *Client) ApplyWebhookResources(ctx context.Context, cfg config.MeshConfig, namespace string, caBundle []byte, dryRun bool) error {
	if err := c.upsertServiceAccount(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: webhookServiceAccountName, Namespace: namespace, Labels: labels(webhookDeploymentName)},
	}, dryRun); err != nil {
		return err
	}

	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: webhookDeploymentName, Namespace: namespace, Labels: labels(webhookDeploymentName)},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": webhookDeploymentName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels(webhookDeploymentName)},
				Spec: corev1.PodSpec{
					ServiceAccountName: webhookServiceAccountName,
					SecurityContext:    &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
					Containers: []corev1.Container{{
						Name:            "webhook",
						Image:           resolveImage("", "mesh/hook", cfg.Spec.Version),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports:           []corev1.ContainerPort{{Name: "https", ContainerPort: 8443}},
						Env: []corev1.EnvVar{
							{Name: "HTTP_ADDR", Value: ":8443"},
							{Name: "TLS_CERT_FILE", Value: "/tls/tls.crt"},
							{Name: "TLS_KEY_FILE", Value: "/tls/tls.key"},
							{Name: "MESH_VERSION", Value: cfg.Spec.Version},
							{Name: "SIDECAR_IMAGE", Value: resolveImage(cfg.Spec.Images.Sidecar, "mesh/sidecar", cfg.Spec.Version)},
							{Name: "IPTABLES_IMAGE", Value: resolveImage(cfg.Spec.Images.IptablesInit, "mesh/iptables-init", cfg.Spec.Version)},
							{Name: "MONITORING_ENABLED", Value: boolToString(cfg.Spec.Sidecar.MonitoringEnabled)},
							{Name: "INBOUND_PLAIN_PORT", Value: fmt.Sprintf("%d", cfg.Spec.Sidecar.InboundPlainPort)},
							{Name: "OUTBOUND_PORT", Value: fmt.Sprintf("%d", cfg.Spec.Sidecar.OutboundPort)},
							{Name: "INBOUND_MTLS_PORT", Value: fmt.Sprintf("%d", cfg.Spec.Sidecar.InboundMTLSPort)},
							{Name: "METRICS_PORT", Value: fmt.Sprintf("%d", cfg.Spec.Sidecar.MetricsPort)},
							{Name: "EXCLUDE_INBOUND_PORTS", Value: cfg.Spec.Sidecar.ExcludeInboundPorts},
							{Name: "EXCLUDE_OUTBOUND_IPS", Value: cfg.Spec.Sidecar.ExcludeOutboundIPs},
							{Name: "SIDECAR_UID", Value: "1337"},
							{Name: "LOAD_BALANCER_ALGORITHM", Value: cfg.Spec.Sidecar.LoadBalancerAlgorithm},
							{Name: "RETRY_ATTEMPTS", Value: fmt.Sprintf("%d", cfg.Spec.Sidecar.RetryPolicy.Attempts)},
							{Name: "TIMEOUT", Value: cfg.Spec.Sidecar.Timeout},
							{Name: "CIRCUIT_BREAKER_FAILURE_THRESHOLD", Value: fmt.Sprintf("%d", cfg.Spec.Sidecar.CircuitBreakerPolicy.FailureThreshold)},
							{Name: "CIRCUIT_BREAKER_RECOVERY_TIME", Value: cfg.Spec.Sidecar.CircuitBreakerPolicy.RecoveryTime},
						},
						VolumeMounts:    []corev1.VolumeMount{{Name: "webhook-tls", MountPath: "/tls", ReadOnly: true}},
						ReadinessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Scheme: corev1.URISchemeHTTPS, Path: "/healthz", Port: intstr.FromString("https")}}, InitialDelaySeconds: 2, PeriodSeconds: 5, TimeoutSeconds: 2, FailureThreshold: 5},
						LivenessProbe:   &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Scheme: corev1.URISchemeHTTPS, Path: "/healthz", Port: intstr.FromString("https")}}, InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 2, FailureThreshold: 3},
						SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: boolPtr(false), ReadOnlyRootFilesystem: boolPtr(true), RunAsNonRoot: boolPtr(true), RunAsUser: int64Ptr(1337), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
					}},
					Volumes: []corev1.Volume{{Name: "webhook-tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: webhookTLSSecretName}}}},
				},
			},
		},
	}

	if err := c.upsertDeployment(ctx, desiredDeployment, dryRun); err != nil {
		return err
	}

	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: webhookServiceName, Namespace: namespace, Labels: labels(webhookDeploymentName)},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app.kubernetes.io/name": webhookDeploymentName}, Ports: []corev1.ServicePort{{Name: "https", Port: 443, TargetPort: intstr.FromString("https"), Protocol: corev1.ProtocolTCP}}},
	}

	if err := c.upsertService(ctx, desiredService, dryRun); err != nil {
		return err
	}

	failurePolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	namespaceSelector := &metav1.LabelSelector{MatchLabels: cfg.Spec.Injection.NamespaceSelector.MatchLabels}
	path := "/mutate"
	port := int32(443)
	timeoutSeconds := int32(10)
	reinvocation := admissionregistrationv1.NeverReinvocationPolicy
	desiredWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: webhookConfigurationName, Labels: labels(webhookDeploymentName)},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			Name:                    "sidecar-injector.mesh.io",
			AdmissionReviewVersions: []string{"v1"},
			SideEffects:             &sideEffects,
			FailurePolicy:           &failurePolicy,
			TimeoutSeconds:          &timeoutSeconds,
			ReinvocationPolicy:      &reinvocation,
			NamespaceSelector:       namespaceSelector,
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service:  &admissionregistrationv1.ServiceReference{Name: webhookServiceName, Namespace: namespace, Path: &path, Port: &port},
				CABundle: caBundle,
			},
			Rules: []admissionregistrationv1.RuleWithOperations{{
				Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
				Rule:       admissionregistrationv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			}},
		}},
	}

	return c.upsertMutatingWebhookConfiguration(ctx, desiredWebhook, dryRun)
}

func (c *Client) WaitDeploymentReady(ctx context.Context, namespace string, deploymentName string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(waitCtx, 2*time.Second, true, func(pollCtx context.Context) (bool, error) {
		deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(pollCtx, deploymentName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		if deployment.Generation > deployment.Status.ObservedGeneration {
			return false, nil
		}

		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	})
}

func (c *Client) DeleteMutatingWebhookConfiguration(ctx context.Context, dryRun bool) error {
	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(ctx, webhookConfigurationName, deleteOptions(dryRun))
	}, "delete mutating webhook configuration")
}

func (c *Client) DeleteWebhookWorkload(ctx context.Context, namespace string, dryRun bool) error {
	if err := c.deleteIgnoreNotFound(func() error {
		return c.clientset.AppsV1().Deployments(namespace).Delete(ctx, webhookDeploymentName, deleteOptions(dryRun))
	}, "delete webhook deployment"); err != nil {
		return err
	}

	if err := c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().Services(namespace).Delete(ctx, webhookServiceName, deleteOptions(dryRun))
	}, "delete webhook service"); err != nil {
		return err
	}

	if err := c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().ServiceAccounts(namespace).Delete(ctx, webhookServiceAccountName, deleteOptions(dryRun))
	}, "delete webhook service account"); err != nil {
		return err
	}

	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().Secrets(namespace).Delete(ctx, webhookTLSSecretName, deleteOptions(dryRun))
	}, "delete webhook tls secret")
}

func (c *Client) DeleteCertManagerWorkload(ctx context.Context, namespace string, dryRun bool) error {
	if err := c.deleteIgnoreNotFound(func() error {
		return c.clientset.AppsV1().Deployments(namespace).Delete(ctx, certManagerDeploymentName, deleteOptions(dryRun))
	}, "delete cert-manager deployment"); err != nil {
		return err
	}

	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().Services(namespace).Delete(ctx, certManagerServiceName, deleteOptions(dryRun))
	}, "delete cert-manager service")
}

func (c *Client) DeleteRootCASecret(ctx context.Context, namespace string, dryRun bool) error {
	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().Secrets(namespace).Delete(ctx, rootCASecretName, deleteOptions(dryRun))
	}, "delete root CA secret")
}

func (c *Client) DeleteSidecarConfigMap(ctx context.Context, namespace string, dryRun bool) error {
	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, sidecarConfigMapName, deleteOptions(dryRun))
	}, "delete sidecar config map")
}

func (c *Client) DeleteRBACResources(ctx context.Context, namespace string, dryRun bool) error {
	if err := c.deleteIgnoreNotFound(func() error {
		return c.clientset.RbacV1().ClusterRoleBindings().Delete(ctx, certManagerClusterRoleBindName, deleteOptions(dryRun))
	}, "delete cluster role binding"); err != nil {
		return err
	}

	if err := c.deleteIgnoreNotFound(func() error {
		return c.clientset.RbacV1().ClusterRoles().Delete(ctx, certManagerClusterRoleName, deleteOptions(dryRun))
	}, "delete cluster role"); err != nil {
		return err
	}

	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().ServiceAccounts(namespace).Delete(ctx, certManagerServiceAccountName, deleteOptions(dryRun))
	}, "delete cert-manager service account")
}

func (c *Client) DeleteNamespace(ctx context.Context, namespace string, dryRun bool) error {
	return c.deleteIgnoreNotFound(func() error {
		return c.clientset.CoreV1().Namespaces().Delete(ctx, namespace, deleteOptions(dryRun))
	}, "delete namespace")
}

func (c *Client) GenerateWebhookTLS(rootCertPEM []byte, rootKeyPEM []byte, namespace string, serviceName string, validity time.Duration) ([]byte, []byte, error) {
	caCert, err := parseCertificate(rootCertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse root CA certificate: %w", err)
	}

	caKey, err := parsePrivateKey(rootKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parse root CA private key: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate webhook private key: %w", err)
	}

	notBefore := time.Now().Add(-1 * time.Minute)
	notAfter := time.Now().Add(validity)
	if caCert.NotAfter.Before(notAfter) {
		notAfter = caCert.NotAfter
	}
	if !notAfter.After(notBefore) {
		return nil, nil, fmt.Errorf("webhook certificate validity is invalid")
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s.%s.svc", serviceName, namespace),
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		DNSNames: []string{
			serviceName,
			fmt.Sprintf("%s.%s", serviceName, namespace),
			fmt.Sprintf("%s.%s.svc", serviceName, namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace),
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create webhook certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return certPEM, keyPEM, nil
}

func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("failed to decode PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert, nil
}

func parsePrivateKey(keyPEM []byte) (interface{}, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("failed to decode PEM private key")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		switch parsed := key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey:
			return parsed, nil
		default:
			return nil, errors.New("unsupported PKCS8 private key type")
		}
	}

	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	return nil, errors.New("unsupported private key format")
}

func (c *Client) upsertSecret(ctx context.Context, desired *corev1.Secret, dryRun bool) error {
	secrets := c.clientset.CoreV1().Secrets(desired.Namespace)
	existing, err := secrets.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get secret %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		_, err := secrets.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create secret %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	_, err = secrets.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update secret %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func (c *Client) upsertConfigMap(ctx context.Context, desired *corev1.ConfigMap, dryRun bool) error {
	maps := c.clientset.CoreV1().ConfigMaps(desired.Namespace)
	existing, err := maps.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get configmap %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		_, err := maps.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create configmap %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	_, err = maps.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update configmap %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func (c *Client) upsertServiceAccount(ctx context.Context, desired *corev1.ServiceAccount, dryRun bool) error {
	accounts := c.clientset.CoreV1().ServiceAccounts(desired.Namespace)
	existing, err := accounts.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get serviceaccount %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		_, err := accounts.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create serviceaccount %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	desired.Secrets = existing.Secrets
	_, err = accounts.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update serviceaccount %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func (c *Client) upsertClusterRole(ctx context.Context, desired *rbacv1.ClusterRole, dryRun bool) error {
	roles := c.clientset.RbacV1().ClusterRoles()
	existing, err := roles.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get clusterrole %s: %w", desired.Name, err)
		}
		_, err := roles.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create clusterrole %s: %w", desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	_, err = roles.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update clusterrole %s: %w", desired.Name, err)
	}
	return nil
}

func (c *Client) upsertClusterRoleBinding(ctx context.Context, desired *rbacv1.ClusterRoleBinding, dryRun bool) error {
	bindings := c.clientset.RbacV1().ClusterRoleBindings()
	existing, err := bindings.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get clusterrolebinding %s: %w", desired.Name, err)
		}
		_, err := bindings.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create clusterrolebinding %s: %w", desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	_, err = bindings.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update clusterrolebinding %s: %w", desired.Name, err)
	}
	return nil
}

func (c *Client) upsertDeployment(ctx context.Context, desired *appsv1.Deployment, dryRun bool) error {
	deployments := c.clientset.AppsV1().Deployments(desired.Namespace)
	existing, err := deployments.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get deployment %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		_, err := deployments.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create deployment %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	_, err = deployments.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update deployment %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func (c *Client) upsertService(ctx context.Context, desired *corev1.Service, dryRun bool) error {
	services := c.clientset.CoreV1().Services(desired.Namespace)
	existing, err := services.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get service %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		_, err := services.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create service %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	desired.Spec.ClusterIPs = existing.Spec.ClusterIPs
	desired.Spec.IPFamilies = existing.Spec.IPFamilies
	desired.Spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
	_, err = services.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update service %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func (c *Client) upsertMutatingWebhookConfiguration(ctx context.Context, desired *admissionregistrationv1.MutatingWebhookConfiguration, dryRun bool) error {
	webhooks := c.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations()
	existing, err := webhooks.Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get mutating webhook configuration %s: %w", desired.Name, err)
		}
		_, err := webhooks.Create(ctx, desired, createOptions(dryRun))
		if err != nil {
			return fmt.Errorf("create mutating webhook configuration %s: %w", desired.Name, err)
		}
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	_, err = webhooks.Update(ctx, desired, updateOptions(dryRun))
	if err != nil {
		return fmt.Errorf("update mutating webhook configuration %s: %w", desired.Name, err)
	}
	return nil
}

func (c *Client) deleteIgnoreNotFound(call func() error, action string) error {
	err := call()
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func labels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      name,
		"app.kubernetes.io/component": "control-plane",
		"app.kubernetes.io/part-of":   "service-mesh",
	}
}

func resolveImage(override string, base string, version string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}

	version = strings.TrimSpace(version)
	if version == "" {
		return base + ":latest"
	}

	return base + ":" + version
}

func mergeStringMaps(current map[string]string, desired map[string]string) map[string]string {
	result := make(map[string]string, len(current)+len(desired))
	for key, value := range current {
		result[key] = value
	}
	for key, value := range desired {
		result[key] = value
	}
	return result
}

func createOptions(dryRun bool) metav1.CreateOptions {
	if !dryRun {
		return metav1.CreateOptions{}
	}
	return metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}
}

func updateOptions(dryRun bool) metav1.UpdateOptions {
	if !dryRun {
		return metav1.UpdateOptions{}
	}
	return metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}}
}

func deleteOptions(dryRun bool) metav1.DeleteOptions {
	if !dryRun {
		return metav1.DeleteOptions{}
	}
	return metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}}
}

func boolPtr(value bool) *bool    { return &value }
func int32Ptr(value int32) *int32 { return &value }
func int64Ptr(value int64) *int64 { return &value }

func boolToString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func ParseCertificateValidity(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse certificate validity: %w", err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("certificate validity must be positive")
	}
	return duration, nil
}

func generateECDSAKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}
