package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRemoteSessionProfileKeepsInstallationTrustAnchor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".sqlrs")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "defaultProfile: remote\nprofiles:\n  remote:\n    mode: remote\n    endpoint: https://api.taidon.dev/nsu\n    installationID: taidon-production\n    installationEndpoint: https://api.taidon.dev\n    auth:\n      mode: remoteSession\n      tokenEnv: SQLRS_TOKEN\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(LoadOptions{WorkingDir: root})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	profile := loaded.Config.Profiles["remote"]
	if profile.InstallationID != "taidon-production" || profile.InstallationEndpoint != "https://api.taidon.dev" || profile.Auth.Mode != "remoteSession" {
		t.Fatalf("profile = %+v", profile)
	}
}
