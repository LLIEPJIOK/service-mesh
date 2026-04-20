package uninstaller

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/LLIEPJIOK/service-mesh/installer/internal/adapters/kube"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/domain"
)

type Service struct {
	kubeClient *kube.Client
	logger     *log.Logger
}

func NewService(kubeClient *kube.Client, logger *log.Logger) *Service {
	return &Service{kubeClient: kubeClient, logger: logger}
}

func (s *Service) Uninstall(ctx context.Context, opts domain.UninstallOptions) error {
	namespace := strings.TrimSpace(opts.Namespace)
	if namespace == "" {
		namespace = domain.DefaultNamespace
	}

	var errs []error

	appendErr := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	appendErr(s.kubeClient.DeleteMutatingWebhookConfiguration(ctx, false))
	appendErr(s.kubeClient.DeleteWebhookWorkload(ctx, namespace, false))
	appendErr(s.kubeClient.DeleteCertManagerWorkload(ctx, namespace, false))
	appendErr(s.kubeClient.DeleteRootCASecret(ctx, namespace, false))
	appendErr(s.kubeClient.DeleteSidecarConfigMap(ctx, namespace, false))
	appendErr(s.kubeClient.DeleteRBACResources(ctx, namespace, false))

	if opts.DeleteNamespace {
		appendErr(s.kubeClient.DeleteNamespace(ctx, namespace, false))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	s.logger.Printf("mesh uninstall completed in namespace %q", namespace)
	return nil
}
