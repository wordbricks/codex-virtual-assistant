package config

import (
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
	defaultCodexBin             = "codex"
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
	CodexBin              string
	CodexCwd              string
	CodexApprovalPolicy   string
	CodexSandboxMode      string
	CodexNetworkAccess    bool
	SchedulerInterval     time.Duration
}

func Load() (Config, error) {
	return loadFromEnv(os.Getenv, os.Getwd, os.UserConfigDir, os.UserHomeDir)
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
	baseDir, err := resolveBaseDir(getwd, userConfigDir, userHomeDir)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:              defaultHTTPAddr,
		DataDir:               defaultDataDir,
		DefaultModel:          FixedModel,
		MaxGenerationAttempts: defaultMaxGenerationAttempt,
		CodexBin:              defaultCodexBin,
		CodexApprovalPolicy:   defaultCodexApprovalPolicy,
		CodexSandboxMode:      defaultCodexSandboxMode,
		CodexNetworkAccess:    true,
		SchedulerInterval:     defaultSchedulerInterval,
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
	if value := getenv("ASSISTANT_CODEX_BIN"); value != "" {
		cfg.CodexBin = value
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
	if c.CodexBin == "" {
		c.CodexBin = defaultCodexBin
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
	case c.CodexBin == "":
		return errors.New("config: codex bin is required")
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
