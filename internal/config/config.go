package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	FixedModel                  = "gpt-5.4"
	defaultHTTPAddr             = "127.0.0.1:4999"
	defaultAppDirName           = "cva"
	defaultHiddenAppDirName     = ".cva"
	defaultDataDir              = "workspace"
	defaultProjectsDirName      = "projects"
	defaultProjectSlug          = "no_project"
	defaultMaxGenerationAttempt = 3
	defaultRuntimeProvider      = "codex"
	defaultCodexBin             = "codex"
	defaultClaudeBin            = "claude"
	defaultCodexApprovalPolicy  = "never"
	defaultCodexSandboxMode     = "workspace-write"
	defaultSchedulerInterval    = 30 * time.Second
)

type Config struct {
	HTTPAddr              string
	ConfigDir             string
	DataDir               string
	ProjectsDir           string
	DatabasePath          string
	ArtifactDir           string
	DefaultModel          string
	MaxGenerationAttempts int
	RuntimeProvider       string
	CodexBin              string
	ClaudeBin             string
	ClaudeModel           string
	CodexCwd              string
	CodexApprovalPolicy   string
	CodexSandboxMode      string
	CodexNetworkAccess    bool
	SchedulerInterval     time.Duration
}

type FileConfig struct {
	RuntimeProvider string `json:"runtime_provider,omitempty"`
}

func Load() (Config, error) {
	return loadFromSources(os.Getenv, os.Getwd, os.UserConfigDir, os.UserHomeDir, ReadFileConfig)
}

func LoadFromEnv(getenv func(string) string, getwd func() (string, error)) (Config, error) {
	return loadFromEnv(getenv, getwd, os.UserConfigDir, os.UserHomeDir)
}

func loadFromEnv(
	getenv func(string) string,
	getwd func() (string, error),
	userConfigDir func() (string, error),
	userHomeDir func() (string, error),
) (Config, error) {
	return loadFromSources(getenv, getwd, userConfigDir, userHomeDir, nil)
}

func loadFromSources(
	getenv func(string) string,
	getwd func() (string, error),
	userConfigDir func() (string, error),
	userHomeDir func() (string, error),
	readFileConfig func(string) (FileConfig, error),
) (Config, error) {
	baseDir, err := resolveBaseDir(getwd, userConfigDir, userHomeDir)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:              defaultHTTPAddr,
		DataDir:               defaultDataDir,
		DefaultModel:          FixedModel,
		MaxGenerationAttempts: defaultMaxGenerationAttempt,
		RuntimeProvider:       defaultRuntimeProvider,
		CodexBin:              defaultCodexBin,
		ClaudeBin:             defaultClaudeBin,
		CodexApprovalPolicy:   defaultCodexApprovalPolicy,
		CodexSandboxMode:      defaultCodexSandboxMode,
		CodexNetworkAccess:    true,
		SchedulerInterval:     defaultSchedulerInterval,
	}

	if readFileConfig != nil {
		fileConfig, err := readFileConfig(ConfigFilePath(baseDir))
		if err != nil {
			return Config{}, err
		}
		if fileConfig.RuntimeProvider != "" {
			cfg.RuntimeProvider = fileConfig.RuntimeProvider
		}
	}

	if value := getenv("ASSISTANT_HTTP_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	if value := getenv("ASSISTANT_DATA_DIR"); value != "" {
		cfg.DataDir = value
	}
	if value := getenv("ASSISTANT_PROJECTS_DIR"); value != "" {
		cfg.ProjectsDir = value
	}
	if value := getenv("ASSISTANT_DATABASE_PATH"); value != "" {
		cfg.DatabasePath = value
	}
	if value := getenv("ASSISTANT_ARTIFACT_DIR"); value != "" {
		cfg.ArtifactDir = value
	}
	if value := getenv("ASSISTANT_MAX_GENERATION_ATTEMPTS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse ASSISTANT_MAX_GENERATION_ATTEMPTS: %w", err)
		}
		cfg.MaxGenerationAttempts = parsed
	}
	if value := getenv("ASSISTANT_RUNTIME"); value != "" {
		cfg.RuntimeProvider = value
	}
	if value := getenv("ASSISTANT_CODEX_BIN"); value != "" {
		cfg.CodexBin = value
	}
	if value := getenv("ASSISTANT_CLAUDE_BIN"); value != "" {
		cfg.ClaudeBin = value
	}
	if value := getenv("ASSISTANT_CLAUDE_MODEL"); value != "" {
		cfg.ClaudeModel = value
	}
	if value := getenv("ASSISTANT_CODEX_CWD"); value != "" {
		cfg.CodexCwd = value
	}
	if value := getenv("ASSISTANT_CODEX_APPROVAL_POLICY"); value != "" {
		cfg.CodexApprovalPolicy = value
	}
	if value := getenv("ASSISTANT_CODEX_SANDBOX"); value != "" {
		cfg.CodexSandboxMode = value
	}
	if value := getenv("ASSISTANT_CODEX_NETWORK_ACCESS"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse ASSISTANT_CODEX_NETWORK_ACCESS: %w", err)
		}
		cfg.CodexNetworkAccess = parsed
	}
	if value := getenv("ASSISTANT_SCHEDULER_INTERVAL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse ASSISTANT_SCHEDULER_INTERVAL: %w", err)
		}
		cfg.SchedulerInterval = parsed
	}
	return cfg.Normalize(baseDir)
}

func ConfigFilePath(configDir string) string {
	return filepath.Join(configDir, "config.json")
}

func ReadFileConfig(path string) (FileConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return FileConfig{}, nil
	}
	if err != nil {
		return FileConfig{}, fmt.Errorf("read config file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return FileConfig{}, nil
	}

	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("parse config file: %w", err)
	}
	if cfg.RuntimeProvider != "" && !ValidRuntimeProvider(cfg.RuntimeProvider) {
		return FileConfig{}, fmt.Errorf("config file: runtime provider must be %q or %q", "codex", "claude")
	}
	return cfg, nil
}

func WriteRuntimeProvider(configDir, provider string) error {
	provider = strings.TrimSpace(provider)
	if !ValidRuntimeProvider(provider) {
		return fmt.Errorf("runtime provider must be %q or %q", "codex", "claude")
	}

	path := ConfigFilePath(configDir)
	cfg, err := ReadFileConfig(path)
	if err != nil {
		return err
	}
	cfg.RuntimeProvider = provider
	return WriteFileConfig(path, cfg)
}

func WriteFileConfig(path string, cfg FileConfig) error {
	if cfg.RuntimeProvider != "" && !ValidRuntimeProvider(cfg.RuntimeProvider) {
		return fmt.Errorf("runtime provider must be %q or %q", "codex", "claude")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config file: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func ValidRuntimeProvider(provider string) bool {
	switch provider {
	case "codex", "claude":
		return true
	default:
		return false
	}
}

func resolveBaseDir(
	getwd func() (string, error),
	userConfigDir func() (string, error),
	userHomeDir func() (string, error),
) (string, error) {
	if userConfigDir != nil {
		if dir, err := userConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
			return filepath.Clean(filepath.Join(dir, defaultAppDirName)), nil
		}
	}
	if userHomeDir != nil {
		if dir, err := userHomeDir(); err == nil && strings.TrimSpace(dir) != "" {
			return filepath.Clean(filepath.Join(dir, defaultHiddenAppDirName)), nil
		}
	}
	if getwd == nil {
		return "", errors.New("resolve working directory: getwd is required")
	}
	dir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("resolve working directory: empty path")
	}
	return filepath.Clean(dir), nil
}

func (c Config) Normalize(baseDir string) (Config, error) {
	c.DefaultModel = FixedModel

	if c.ConfigDir == "" {
		c.ConfigDir = baseDir
	}
	if c.DataDir == "" {
		c.DataDir = defaultDataDir
	}
	if c.ProjectsDir == "" {
		c.ProjectsDir = filepath.Join(c.DataDir, defaultProjectsDirName)
	}
	if c.DatabasePath == "" {
		c.DatabasePath = filepath.Join(c.DataDir, "assistant.db")
	}
	if c.ArtifactDir == "" {
		c.ArtifactDir = filepath.Join(c.DataDir, "artifacts")
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = defaultHTTPAddr
	}
	if c.MaxGenerationAttempts == 0 {
		c.MaxGenerationAttempts = defaultMaxGenerationAttempt
	}
	if c.RuntimeProvider == "" {
		c.RuntimeProvider = defaultRuntimeProvider
	}
	if c.CodexBin == "" {
		c.CodexBin = defaultCodexBin
	}
	if c.ClaudeBin == "" {
		c.ClaudeBin = defaultClaudeBin
	}
	if c.CodexCwd == "" {
		c.CodexCwd = baseDir
	}
	if c.CodexApprovalPolicy == "" {
		c.CodexApprovalPolicy = defaultCodexApprovalPolicy
	}
	if c.CodexSandboxMode == "" {
		c.CodexSandboxMode = defaultCodexSandboxMode
	}
	if c.SchedulerInterval == 0 {
		c.SchedulerInterval = defaultSchedulerInterval
	}

	var err error
	if c.ConfigDir, err = resolvePath(baseDir, c.ConfigDir); err != nil {
		return Config{}, fmt.Errorf("resolve config dir: %w", err)
	}
	if c.DataDir, err = resolvePath(baseDir, c.DataDir); err != nil {
		return Config{}, fmt.Errorf("resolve data dir: %w", err)
	}
	if c.ProjectsDir, err = resolvePath(baseDir, c.ProjectsDir); err != nil {
		return Config{}, fmt.Errorf("resolve projects dir: %w", err)
	}
	if c.DatabasePath, err = resolvePath(baseDir, c.DatabasePath); err != nil {
		return Config{}, fmt.Errorf("resolve database path: %w", err)
	}
	if c.ArtifactDir, err = resolvePath(baseDir, c.ArtifactDir); err != nil {
		return Config{}, fmt.Errorf("resolve artifact dir: %w", err)
	}
	if c.CodexCwd, err = resolvePath(baseDir, c.CodexCwd); err != nil {
		return Config{}, fmt.Errorf("resolve codex cwd: %w", err)
	}

	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	runtimeProvider := c.RuntimeProvider
	if runtimeProvider == "" {
		runtimeProvider = defaultRuntimeProvider
	}
	switch {
	case c.HTTPAddr == "":
		return errors.New("config: HTTP address is required")
	case c.DataDir == "":
		return errors.New("config: data dir is required")
	case c.DatabasePath == "":
		return errors.New("config: database path is required")
	case c.ArtifactDir == "":
		return errors.New("config: artifact dir is required")
	case c.DefaultModel != FixedModel:
		return fmt.Errorf("config: default model must remain %q", FixedModel)
	case c.MaxGenerationAttempts <= 0:
		return errors.New("config: max generation attempts must be positive")
	case !ValidRuntimeProvider(runtimeProvider):
		return fmt.Errorf("config: runtime provider must be %q or %q", "codex", "claude")
	case runtimeProvider == "codex" && c.CodexBin == "":
		return errors.New("config: codex bin is required")
	case runtimeProvider == "claude" && c.ClaudeBin == "":
		return errors.New("config: claude bin is required")
	case c.CodexCwd == "":
		return errors.New("config: codex cwd is required")
	case c.CodexApprovalPolicy == "":
		return errors.New("config: codex approval policy is required")
	case c.CodexSandboxMode == "":
		return errors.New("config: codex sandbox mode is required")
	case c.SchedulerInterval < 0:
		return errors.New("config: scheduler interval must not be negative")
	default:
		return nil
	}
}

func (c Config) EffectiveProjectsDir() string {
	if strings.TrimSpace(c.ProjectsDir) != "" {
		return filepath.Clean(c.ProjectsDir)
	}
	if strings.TrimSpace(c.DataDir) != "" {
		return filepath.Clean(filepath.Join(c.DataDir, defaultProjectsDirName))
	}
	return defaultProjectsDirName
}

func (c Config) ProjectArtifactDir(projectSlug string) string {
	slug := strings.TrimSpace(projectSlug)
	if slug == "" {
		slug = defaultProjectSlug
	}
	return filepath.Clean(filepath.Join(c.EffectiveProjectsDir(), slug, "artifacts"))
}

func resolvePath(baseDir, value string) (string, error) {
	if value == "" {
		return "", errors.New("path is empty")
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Clean(filepath.Join(baseDir, value)), nil
}
