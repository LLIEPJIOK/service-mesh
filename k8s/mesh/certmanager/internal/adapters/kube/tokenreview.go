package kube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/LLIEPJIOK/service-mesh/certmanager/internal/domain"
)

const serviceAccountUsernamePrefix = "system:serviceaccount:"

type TokenReviewer struct {
	client kubernetes.Interface
}

func NewTokenReviewer(kubeConfigPath string) (*TokenReviewer, error) {
	config, err := buildKubeConfig(kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("build kube config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kube client: %w", err)
	}

	return &TokenReviewer{client: client}, nil
}

func (t *TokenReviewer) ValidateToken(ctx context.Context, token string) (domain.Identity, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		slog.Warn("tokenreview rejected empty token")
		return domain.Identity{}, domain.ErrUnauthorized
	}

	review := &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{Token: token},
	}

	response, err := t.client.AuthenticationV1().
		TokenReviews().
		Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			slog.Warn("tokenreview forbidden", slog.Any("error", err))
			return domain.Identity{}, fmt.Errorf("%w: %v", domain.ErrForbidden, err)
		}

		if k8serrors.IsUnauthorized(err) {
			slog.Warn("tokenreview unauthorized", slog.Any("error", err))
			return domain.Identity{}, fmt.Errorf("%w: %v", domain.ErrUnauthorized, err)
		}

		slog.Warn("tokenreview call failed", slog.Any("error", err))
		return domain.Identity{}, fmt.Errorf("tokenreview call failed: %w", err)
	}

	if !response.Status.Authenticated {
		if strings.TrimSpace(response.Status.Error) != "" {
			slog.Warn("tokenreview authentication failed", slog.String("reason", response.Status.Error))
			return domain.Identity{}, fmt.Errorf(
				"%w: %s",
				domain.ErrUnauthorized,
				response.Status.Error,
			)
		}

		slog.Warn("tokenreview authentication failed without explicit reason")
		return domain.Identity{}, domain.ErrUnauthorized
	}

	identity, parseErr := parseServiceAccountUsername(response.Status.User.Username)
	if parseErr != nil {
		slog.Warn("tokenreview identity parse failed", slog.String("username", response.Status.User.Username), slog.Any("error", parseErr))
		return domain.Identity{}, fmt.Errorf("%w: %v", domain.ErrForbidden, parseErr)
	}

	slog.Info(
		"tokenreview authenticated service account",
		slog.String("namespace", identity.Namespace),
		slog.String("service_account", identity.ServiceAccount),
	)

	return identity, nil
}

func parseServiceAccountUsername(username string) (domain.Identity, error) {
	username = strings.TrimSpace(username)
	if !strings.HasPrefix(username, serviceAccountUsernamePrefix) {
		return domain.Identity{}, fmt.Errorf("unsupported username format %q", username)
	}

	parts := strings.Split(username, ":")
	if len(parts) != 4 {
		return domain.Identity{}, fmt.Errorf("invalid service account username format %q", username)
	}

	namespace := strings.TrimSpace(parts[2])
	serviceAccount := strings.TrimSpace(parts[3])
	if namespace == "" || serviceAccount == "" {
		return domain.Identity{}, errors.New("namespace or service account is empty")
	}

	return domain.Identity{
		Namespace:      namespace,
		ServiceAccount: serviceAccount,
	}, nil
}

func buildKubeConfig(kubeConfigPath string) (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	path := strings.TrimSpace(kubeConfigPath)
	if path == "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf("resolve home directory for kubeconfig: %w", homeErr)
		}

		path = filepath.Join(homeDir, ".kube", "config")
	}

	config, buildErr := clientcmd.BuildConfigFromFlags("", path)
	if buildErr != nil {
		return nil, fmt.Errorf("build kubeconfig from %s: %w", path, buildErr)
	}

	return config, nil
}
