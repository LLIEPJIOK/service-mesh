package domain

import "time"

type InstallOptions struct {
	ConfigFile string
	Namespace  string
	Wait       bool
	Timeout    time.Duration
	DryRun     bool
	Kubeconfig string
	CLIVersion string
}

type UninstallOptions struct {
	Namespace       string
	DeleteNamespace bool
	Kubeconfig      string
}

const (
	DefaultNamespace = "mesh-system"
)
