package config

import "github.com/sqlrs/cli/internal/paths"

type Config struct {
	DefaultProfile string                   `yaml:"defaultProfile"`
	Client         ClientConfig             `yaml:"client"`
	Orchestrator   OrchestratorConfig       `yaml:"orchestrator"`
	Engine         EngineConfig             `yaml:"engine"`
	DBMS           DBMSConfig               `yaml:"dbms"`
	Profiles       map[string]ProfileConfig `yaml:"profiles"`
}

type ClientConfig struct {
	Timeout string `yaml:"timeout"`
	Retries int    `yaml:"retries"`
	Output  string `yaml:"output"`
}

type OrchestratorConfig struct {
	StartupTimeout string `yaml:"startupTimeout"`
	IdleTimeout    string `yaml:"idleTimeout"`
	RunDir         string `yaml:"runDir"`
	DaemonPath     string `yaml:"daemonPath"`
}

type DBMSConfig struct {
	Image string `yaml:"image"`
}

type EngineConfig struct {
	StorePath string          `yaml:"storePath"`
	WSL       EngineWSLConfig `yaml:"wsl"`
}

type EngineWSLConfig struct {
	Mode       string               `yaml:"mode"`
	Distro     string               `yaml:"distro"`
	StateDir   string               `yaml:"stateDir"`
	EnginePath string               `yaml:"enginePath"`
	Mount      EngineWSLMountConfig `yaml:"mount"`
}

type EngineWSLMountConfig struct {
	Device     string `yaml:"device"`
	FSType     string `yaml:"fstype"`
	DeviceUUID string `yaml:"deviceUUID"`
	Unit       string `yaml:"unit"`
}

type ProfileConfig struct {
	Mode                 string           `yaml:"mode"`
	Endpoint             string           `yaml:"endpoint"`
	InstallationID       string           `yaml:"installationID"`
	InstallationEndpoint string           `yaml:"installationEndpoint"`
	Autostart            bool             `yaml:"autostart"`
	Auth                 AuthConfig       `yaml:"auth"`
	SourceSync           SourceSyncConfig `yaml:"sourceSync"`
}

type AuthConfig struct {
	Mode         string `yaml:"mode"`
	TokenEnv     string `yaml:"tokenEnv"`
	Token        string `yaml:"token"`
	ClientID     string `yaml:"clientID"`
	ClientSecret string `yaml:"clientSecret"`
	Issuer       string `yaml:"issuer"`
}

type SourceSyncConfig struct {
	Mode      string `yaml:"mode"`
	MaxRounds int    `yaml:"maxRounds"`
}

type LoadOptions struct {
	WorkingDir string
	Dirs       *paths.Dirs
}

type LoadedConfig struct {
	Config            Config
	Paths             paths.Dirs
	ProjectConfigPath string
}
