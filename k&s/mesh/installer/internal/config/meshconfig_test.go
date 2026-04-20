package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFileValidatesRootCA(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mesh.yaml")

	content := `apiVersion: mesh.io/v1alpha1
kind: MeshConfig
spec:
  namespace: mesh-system
  sidecar:
    inboundPlainPort: 15006
    outboundPort: 15002
    inboundMTLSPort: 15001
`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := LoadFromFile(path); err == nil {
		t.Fatalf("expected validation error for missing rootCA")
	}
}

func TestLoadFromFileSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mesh.yaml")

	content := `apiVersion: mesh.io/v1alpha1
kind: MeshConfig
spec:
  namespace: mesh-system
  version: v0.1.0
  certificates:
    rootCA:
      cert: |-
        -----BEGIN CERTIFICATE-----
        test
        -----END CERTIFICATE-----
      key: |-
        -----BEGIN RSA PRIVATE KEY-----
        test
        -----END RSA PRIVATE KEY-----
  sidecar:
    inboundPlainPort: 15006
    outboundPort: 15002
    inboundMTLSPort: 15001
`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if cfg.Spec.Namespace != "mesh-system" {
		t.Fatalf("unexpected namespace: %s", cfg.Spec.Namespace)
	}
}
