package discovery

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func NewClientset(kubeConfigPath string) (kubernetes.Interface, error) {
	config, err := loadRESTConfig(kubeConfigPath)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes clientset: %w", err)
	}

	return clientset, nil
}

func loadRESTConfig(kubeConfigPath string) (*rest.Config, error) {
	if kubeConfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig from %s: %w", kubeConfigPath, err)
		}

		return config, nil
	}

	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	if home := homedir.HomeDir(); home != "" {
		path := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(path); err == nil {
			config, buildErr := clientcmd.BuildConfigFromFlags("", path)
			if buildErr != nil {
				return nil, fmt.Errorf("load kubeconfig from %s: %w", path, buildErr)
			}

			return config, nil
		}
	}

	return nil, fmt.Errorf("kubernetes configuration not found")
}
