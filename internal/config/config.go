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

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
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
	AutomationSafety      AutomationSafetyConfig
}

type FileConfig struct {
	RuntimeProvider  string                 `json:"runtime_provider,omitempty"`
	AutomationSafety AutomationSafetyConfig `json:"automation_safety,omitempty"`
}

type AutomationSafetyConfig struct {
	Defaults map[string]AutomationSafetyPolicyOverride  `json:"defaults,omitempty"`
	Projects map[string]AutomationSafetyProjectOverride `json:"projects,omitempty"`
}

type AutomationSafetyProjectOverride struct {
	ProfileOverride assistant.AutomationSafetyProfile `json:"profile_override,omitempty"`
	AutomationSafetyPolicyOverride
}

type AutomationSafetyPolicyOverride struct {
	Enforcement     *assistant.AutomationSafetyEnforcement  `json:"enforcement,omitempty"`
	ModePolicy      AutomationSafetyModePolicyOverride      `json:"mode_policy,omitempty"`
	RateLimits      AutomationSafetyRateLimitsOverride      `json:"rate_limits,omitempty"`
	PatternRules    AutomationSafetyPatternRulesOverride    `json:"pattern_rules,omitempty"`
	TextReusePolicy AutomationSafetyTextReusePolicyOverride `json:"text_reuse_policy,omitempty"`
	CooldownPolicy  AutomationSafetyCooldownPolicyOverride  `json:"cooldown_policy,omitempty"`
}

type AutomationSafetyModePolicyOverride struct {
	AllowedSessionModes      []string `json:"allowed_session_modes,omitempty"`
	AllowNoActionSuccess     *bool    `json:"allow_no_action_success,omitempty"`
	RequireNoActionEvidence  *bool    `json:"require_no_action_evidence,omitempty"`
	NoActionEvidenceRequired []string `json:"no_action_evidence_required,omitempty"`
}

type AutomationSafetyRateLimitsOverride struct {
	MaxAccountChangingActionsPerRun *int `json:"max_account_changing_actions_per_run,omitempty"`
	MaxRepliesPer24h                *int `json:"max_replies_per_24h,omitempty"`
	MinSpacingMinutes               *int `json:"min_spacing_minutes,omitempty"`
}

type AutomationSafetyPatternRulesOverride struct {
	DisallowDefaultActionTrios *bool `json:"disallow_default_action_trios,omitempty"`
	DisallowFixedShortFollowup *bool `json:"disallow_fixed_short_followups,omitempty"`
	RequireSourceDiversity     *bool `json:"require_source_diversity,omitempty"`
}

type AutomationSafetyTextReusePolicyOverride struct {
	RejectHighSimilarity      *bool `json:"reject_high_similarity,omitempty"`
	AvoidRepeatedSelfIntro    *bool `json:"avoid_repeated_self_intro,omitempty"`
	RequireTextVariantSupport *bool `json:"require_text_variant_support,omitempty"`
}

type AutomationSafetyCooldownPolicyOverride struct {
	ForceReadOnlyAfterDenseActivity      *bool `json:"force_read_only_after_dense_activity,omitempty"`
	PreferLongerCooldownAfterBlockedRuns *bool `json:"prefer_longer_cooldown_after_blocked_runs,omitempty"`
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
		cfg.AutomationSafety = fileConfig.AutomationSafety
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
		return FileConfig{}, fmt.Errorf("config file: runtime provider must be %q, %q, or %q", "codex", "claude", "zai")
	}
	if err := cfg.AutomationSafety.Validate(); err != nil {
		return FileConfig{}, fmt.Errorf("config file: automation safety %w", err)
	}
	return cfg, nil
}

func WriteRuntimeProvider(configDir, provider string) error {
	provider = strings.TrimSpace(provider)
	if !ValidRuntimeProvider(provider) {
		return fmt.Errorf("runtime provider must be %q, %q, or %q", "codex", "claude", "zai")
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
		return fmt.Errorf("runtime provider must be %q, %q, or %q", "codex", "claude", "zai")
	}
	if err := cfg.AutomationSafety.Validate(); err != nil {
		return fmt.Errorf("automation safety %w", err)
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
	case "codex", "claude", "zai":
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
		return fmt.Errorf("config: runtime provider must be %q, %q, or %q", "codex", "claude", "zai")
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
	}

	if err := c.AutomationSafety.Validate(); err != nil {
		return fmt.Errorf("config: automation safety %w", err)
	}
	return nil
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

func (c Config) ResolveAutomationSafetyPolicy(
	engineDefault *assistant.AutomationSafetyPolicy,
	inferredProfile assistant.AutomationSafetyProfile,
	projectSlug string,
) *assistant.AutomationSafetyPolicy {
	projectSlug = strings.TrimSpace(projectSlug)
	projectOverride, hasProjectOverride := c.AutomationSafety.Projects[projectSlug]
	if !hasProjectOverride {
		projectOverride = AutomationSafetyProjectOverride{}
	}

	profile := assistant.AutomationSafetyProfileNone
	if engineDefault != nil && engineDefault.Profile != "" {
		profile = engineDefault.Profile
	}
	if inferredProfile != "" {
		profile = inferredProfile
	}
	if projectOverride.ProfileOverride != "" {
		profile = projectOverride.ProfileOverride
	}
	if profile == "" {
		profile = assistant.AutomationSafetyProfileNone
	}

	defaultOverride, hasDefaultOverride := c.AutomationSafety.Defaults[string(profile)]
	hasExplicitProject := hasProjectOverride && (projectOverride.ProfileOverride != "" || projectOverride.AutomationSafetyPolicyOverride.hasAnyFieldSet())
	hasInferredProfile := inferredProfile != "" && inferredProfile != assistant.AutomationSafetyProfileNone
	if engineDefault == nil && !hasDefaultOverride && !hasExplicitProject && !hasInferredProfile {
		return nil
	}

	policy := cloneAutomationSafetyPolicy(engineDefault)
	if policy == nil {
		policy = &assistant.AutomationSafetyPolicy{}
	}
	policy.Profile = profile

	if hasDefaultOverride {
		applyAutomationSafetyPolicyOverride(policy, defaultOverride)
	}
	if hasProjectOverride {
		applyAutomationSafetyPolicyOverride(policy, projectOverride.AutomationSafetyPolicyOverride)
	}

	if policy.Profile == "" {
		policy.Profile = assistant.AutomationSafetyProfileNone
	}
	if policy.Enforcement == "" {
		policy.Enforcement = defaultEnforcementForProfile(policy.Profile)
	}

	return policy
}

func (c AutomationSafetyConfig) Validate() error {
	for profileKey, override := range c.Defaults {
		profile, err := parseAutomationSafetyProfile(strings.TrimSpace(profileKey))
		if err != nil {
			return fmt.Errorf("defaults[%q]: %w", profileKey, err)
		}
		if err := override.validate(&profile); err != nil {
			return fmt.Errorf("defaults[%q]: %w", profileKey, err)
		}
	}

	for projectKey, override := range c.Projects {
		if strings.TrimSpace(projectKey) == "" {
			return errors.New("projects: key must be non-empty")
		}
		var profileHint *assistant.AutomationSafetyProfile
		if override.ProfileOverride != "" {
			profile := override.ProfileOverride
			if _, err := parseAutomationSafetyProfile(string(profile)); err != nil {
				return fmt.Errorf("projects[%q].profile_override: %w", projectKey, err)
			}
			profileHint = &profile
		}
		if err := override.AutomationSafetyPolicyOverride.validate(profileHint); err != nil {
			return fmt.Errorf("projects[%q]: %w", projectKey, err)
		}
	}

	return nil
}

func (o AutomationSafetyPolicyOverride) validate(profileHint *assistant.AutomationSafetyProfile) error {
	if o.Enforcement != nil {
		enforcement := *o.Enforcement
		switch enforcement {
		case assistant.AutomationSafetyEnforcementAdvisory,
			assistant.AutomationSafetyEnforcementEvaluatorEnforced,
			assistant.AutomationSafetyEnforcementEngineBlocking:
		default:
			return errors.New("enforcement is invalid")
		}
		if enforcement == assistant.AutomationSafetyEnforcementEngineBlocking &&
			profileHint != nil && *profileHint != assistant.AutomationSafetyProfileBrowserHighRiskEngagement {
			return errors.New("engine_blocking is only valid for browser_high_risk_engagement")
		}
	}

	if o.RateLimits.MaxAccountChangingActionsPerRun != nil && *o.RateLimits.MaxAccountChangingActionsPerRun < 0 {
		return errors.New("max_account_changing_actions_per_run cannot be negative")
	}
	if o.RateLimits.MaxRepliesPer24h != nil && *o.RateLimits.MaxRepliesPer24h < 0 {
		return errors.New("max_replies_per_24h cannot be negative")
	}
	if o.RateLimits.MinSpacingMinutes != nil && *o.RateLimits.MinSpacingMinutes < 0 {
		return errors.New("min_spacing_minutes cannot be negative")
	}

	for _, mode := range o.ModePolicy.AllowedSessionModes {
		switch mode {
		case "read_only", "single_action", "reply_only":
		default:
			return fmt.Errorf("allowed session mode %q is invalid", mode)
		}
	}
	for _, requirement := range o.ModePolicy.NoActionEvidenceRequired {
		if strings.TrimSpace(requirement) == "" {
			return errors.New("no_action_evidence_required must not contain empty values")
		}
	}

	return nil
}

func (o AutomationSafetyPolicyOverride) hasAnyFieldSet() bool {
	return o.Enforcement != nil ||
		o.ModePolicy.AllowNoActionSuccess != nil ||
		o.ModePolicy.RequireNoActionEvidence != nil ||
		o.ModePolicy.AllowedSessionModes != nil ||
		o.ModePolicy.NoActionEvidenceRequired != nil ||
		o.RateLimits.MaxAccountChangingActionsPerRun != nil ||
		o.RateLimits.MaxRepliesPer24h != nil ||
		o.RateLimits.MinSpacingMinutes != nil ||
		o.PatternRules.DisallowDefaultActionTrios != nil ||
		o.PatternRules.DisallowFixedShortFollowup != nil ||
		o.PatternRules.RequireSourceDiversity != nil ||
		o.TextReusePolicy.RejectHighSimilarity != nil ||
		o.TextReusePolicy.AvoidRepeatedSelfIntro != nil ||
		o.TextReusePolicy.RequireTextVariantSupport != nil ||
		o.CooldownPolicy.ForceReadOnlyAfterDenseActivity != nil ||
		o.CooldownPolicy.PreferLongerCooldownAfterBlockedRuns != nil
}

func parseAutomationSafetyProfile(raw string) (assistant.AutomationSafetyProfile, error) {
	profile := assistant.AutomationSafetyProfile(strings.TrimSpace(raw))
	switch profile {
	case assistant.AutomationSafetyProfileNone,
		assistant.AutomationSafetyProfileBrowserReadOnly,
		assistant.AutomationSafetyProfileBrowserMutating,
		assistant.AutomationSafetyProfileBrowserHighRiskEngagement:
		return profile, nil
	default:
		return "", errors.New("profile is invalid")
	}
}

func cloneAutomationSafetyPolicy(source *assistant.AutomationSafetyPolicy) *assistant.AutomationSafetyPolicy {
	if source == nil {
		return nil
	}
	clone := *source
	clone.ModePolicy.AllowedSessionModes = append([]string(nil), source.ModePolicy.AllowedSessionModes...)
	clone.ModePolicy.NoActionEvidenceRequired = append([]string(nil), source.ModePolicy.NoActionEvidenceRequired...)
	return &clone
}

func applyAutomationSafetyPolicyOverride(target *assistant.AutomationSafetyPolicy, override AutomationSafetyPolicyOverride) {
	if target == nil {
		return
	}
	if override.Enforcement != nil {
		target.Enforcement = *override.Enforcement
	}

	if override.ModePolicy.AllowedSessionModes != nil {
		target.ModePolicy.AllowedSessionModes = cleanStringList(override.ModePolicy.AllowedSessionModes)
	}
	if override.ModePolicy.AllowNoActionSuccess != nil {
		target.ModePolicy.AllowNoActionSuccess = *override.ModePolicy.AllowNoActionSuccess
	}
	if override.ModePolicy.RequireNoActionEvidence != nil {
		target.ModePolicy.RequireNoActionEvidence = *override.ModePolicy.RequireNoActionEvidence
	}
	if override.ModePolicy.NoActionEvidenceRequired != nil {
		target.ModePolicy.NoActionEvidenceRequired = cleanStringList(override.ModePolicy.NoActionEvidenceRequired)
	}

	if override.RateLimits.MaxAccountChangingActionsPerRun != nil {
		target.RateLimits.MaxAccountChangingActionsPerRun = *override.RateLimits.MaxAccountChangingActionsPerRun
	}
	if override.RateLimits.MaxRepliesPer24h != nil {
		target.RateLimits.MaxRepliesPer24h = *override.RateLimits.MaxRepliesPer24h
	}
	if override.RateLimits.MinSpacingMinutes != nil {
		target.RateLimits.MinSpacingMinutes = *override.RateLimits.MinSpacingMinutes
	}

	if override.PatternRules.DisallowDefaultActionTrios != nil {
		target.PatternRules.DisallowDefaultActionTrios = *override.PatternRules.DisallowDefaultActionTrios
	}
	if override.PatternRules.DisallowFixedShortFollowup != nil {
		target.PatternRules.DisallowFixedShortFollowup = *override.PatternRules.DisallowFixedShortFollowup
	}
	if override.PatternRules.RequireSourceDiversity != nil {
		target.PatternRules.RequireSourceDiversity = *override.PatternRules.RequireSourceDiversity
	}

	if override.TextReusePolicy.RejectHighSimilarity != nil {
		target.TextReuse.RejectHighSimilarity = *override.TextReusePolicy.RejectHighSimilarity
	}
	if override.TextReusePolicy.AvoidRepeatedSelfIntro != nil {
		target.TextReuse.AvoidRepeatedSelfIntro = *override.TextReusePolicy.AvoidRepeatedSelfIntro
	}
	if override.TextReusePolicy.RequireTextVariantSupport != nil {
		target.TextReuse.RequireTextVariantSupport = *override.TextReusePolicy.RequireTextVariantSupport
	}

	if override.CooldownPolicy.ForceReadOnlyAfterDenseActivity != nil {
		target.CooldownPolicy.ForceReadOnlyAfterDenseActivity = *override.CooldownPolicy.ForceReadOnlyAfterDenseActivity
	}
	if override.CooldownPolicy.PreferLongerCooldownAfterBlockedRuns != nil {
		target.CooldownPolicy.PreferLongerCooldownAfterBlockedRuns = *override.CooldownPolicy.PreferLongerCooldownAfterBlockedRuns
	}
}

func cleanStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned
}

func defaultEnforcementForProfile(profile assistant.AutomationSafetyProfile) assistant.AutomationSafetyEnforcement {
	switch profile {
	case assistant.AutomationSafetyProfileBrowserMutating:
		return assistant.AutomationSafetyEnforcementEvaluatorEnforced
	case assistant.AutomationSafetyProfileBrowserHighRiskEngagement:
		return assistant.AutomationSafetyEnforcementEngineBlocking
	case assistant.AutomationSafetyProfileBrowserReadOnly, assistant.AutomationSafetyProfileNone:
		fallthrough
	default:
		return assistant.AutomationSafetyEnforcementAdvisory
	}
}
