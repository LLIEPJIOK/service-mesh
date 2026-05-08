package installer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/LLIEPJIOK/service-mesh/installer/internal/adapters/kube"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/config"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/domain"
)

type Service struct {
	kubeClient *kube.Client
	logger     *log.Logger
}

func NewService(kubeClient *kube.Client, logger *log.Logger) *Service {
	return &Service{kubeClient: kubeClient, logger: logger}
}

func BuildPlan(namespace string) []string {
	return []string{
		"1) create namespace " + namespace,
		"2) apply CRDs (MVP: no-op)",
		"3) create root CA secret mesh-root-ca",
		"4) install cert-manager (ServiceAccount, ClusterRole, ClusterRoleBinding, Deployment, Service)",
		"5) apply sidecar default ConfigMap mesh-sidecar-config",
		"6) install webhook (ServiceAccount, Deployment, Service, MutatingWebhookConfiguration)",
		"7) apply additional components (MVP: no-op)",
	}
}

func (s *Service) Install(ctx context.Context, cfg config.MeshConfig, opts domain.InstallOptions) error {
	namespace := cfg.EffectiveNamespace(opts.Namespace)
	if namespace == "" {
		namespace = domain.DefaultNamespace
	}

	if opts.DryRun {
		s.logger.Printf("dry-run enabled; rendering install plan only")
		for _, step := range BuildPlan(namespace) {
			s.logger.Println(step)
		}
		return nil
	}

	if s.kubeClient == nil {
		return fmt.Errorf("kubernetes client is required for non-dry-run install")
	}

	if versionMismatch(opts.CLIVersion, cfg.Spec.Version) {
		s.logger.Printf("warning: CLI version %q may be incompatible with mesh spec.version %q", opts.CLIVersion, cfg.Spec.Version)
	}

	if err := s.kubeClient.EnsureNamespace(ctx, namespace, false); err != nil {
		return err
	}

	s.logger.Printf("CRD phase: no resources defined in MVP, skipping")

	rootCert := []byte(strings.TrimSpace(cfg.Spec.Certificates.RootCA.Cert) + "\n")
	rootKey := []byte(strings.TrimSpace(cfg.Spec.Certificates.RootCA.Key) + "\n")
	if err := s.kubeClient.ApplyRootCASecret(ctx, namespace, rootCert, rootKey, false); err != nil {
		return err
	}

	validity, err := kube.ParseCertificateValidity(cfg.Spec.Certificates.Validity)
	if err != nil {
		return err
	}

	webhookCert, webhookKey, err := s.kubeClient.GenerateWebhookTLS(rootCert, rootKey, namespace, "mesh-webhook", minDuration(validity, 365*24*time.Hour))
	if err != nil {
		return err
	}

	if err := s.kubeClient.ApplyWebhookTLSSecret(ctx, namespace, webhookCert, webhookKey, false); err != nil {
		return err
	}

	if err := s.kubeClient.ApplyCertManagerResources(ctx, cfg, namespace, false); err != nil {
		return err
	}

	if err := s.kubeClient.ApplySidecarConfigMap(ctx, cfg, namespace, false); err != nil {
		return err
	}

	if err := s.kubeClient.ApplyWebhookResources(ctx, cfg, namespace, rootCert, false); err != nil {
		return err
	}

	if opts.Wait {
		s.logger.Printf("waiting for critical components to become Ready (timeout: %s)", opts.Timeout)
		if err := s.kubeClient.WaitDeploymentReady(ctx, namespace, "mesh-cert-manager", opts.Timeout); err != nil {
			return fmt.Errorf("cert-manager is not ready before timeout: %w", err)
		}

		if err := s.kubeClient.WaitDeploymentReady(ctx, namespace, "mesh-webhook", opts.Timeout); err != nil {
			return fmt.Errorf("mesh-webhook is not ready before timeout: %w", err)
		}
	}

	s.logger.Printf("mesh install completed in namespace %q", namespace)
	return nil
}

func versionMismatch(cliVersion string, specVersion string) bool {
	if strings.TrimSpace(cliVersion) == "" || strings.TrimSpace(cliVersion) == "dev" {
		return false
	}
	return strings.TrimSpace(cliVersion) != strings.TrimSpace(specVersion)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
