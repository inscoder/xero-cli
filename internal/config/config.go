package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

const (
	defaultConfigFileName  = "config.json"
	defaultSessionFileName = "session.json"
	defaultTokenFileName   = "tokens.json"
	defaultTokenKeyName    = ".encryption-key"
	defaultTokenSaltName   = ".token-salt"
	defaultLockFileName    = "tokens.lock"
	inlineProfileName      = "_inline"
)

type FileConfig struct {
	DefaultProfile string                   `json:"defaultProfile,omitempty"`
	Profiles       map[string]ProfileConfig `json:"profiles,omitempty"`
	OutputMode     string                   `json:"outputMode,omitempty"`
	Scopes         []string                 `json:"scopes,omitempty"`
}

type ProfileConfig struct {
	ClientID string `json:"clientId"`
}

type Settings struct {
	ConfigDir         string
	ConfigFilePath    string
	SessionFilePath   string
	TokenFallbackPath string
	TokenKeyPath      string
	TokenSaltPath     string
	TokenLockPath     string
	ProfileName       string
	ClientID          string
	OutputJSON        bool
	Quiet             bool
	NoBrowser         bool
	CallbackTimeout   time.Duration
	RefreshBefore     time.Duration
	Interactive       bool
	XeroScopes        []string
	OpenCommand       string
	Version           string
}

type Manager struct {
	viper      *viper.Viper
	configDir  string
	configFile string
	loaded     FileConfig
}

func NewManager(v *viper.Viper) (*Manager, error) {
	configDir, err := defaultConfigDir()
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "resolve config directory", err)
	}

	configPath := v.GetString("config")
	if configPath == "" {
		configPath = filepath.Join(configDir, defaultConfigFileName)
	} else {
		configDir = filepath.Dir(configPath)
	}

	return &Manager{
		viper:      v,
		configDir:  configDir,
		configFile: configPath,
	}, nil
}

func defaultConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "xero"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "xero"), nil
}

func ConfigureViper(v *viper.Viper) {
	loadDotEnv()
	v.SetEnvPrefix("XERO")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()
	_ = v.BindEnv("auth.scopes", "XERO_SCOPES")
	_ = v.BindEnv("client_id", "XERO_CLIENT_ID")
	v.SetDefault("auth.callback_timeout", "2m")
	v.SetDefault("auth.refresh_before", "1m")
	v.SetDefault("auth.open_command", "")
	v.SetDefault("auth.scopes", strings.Join(DefaultScopes(), " "))
	v.SetDefault("output.json", false)
	v.SetDefault("output.quiet", false)
}

func loadDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}
	_ = gotenv.Load(".env")
}

func (m *Manager) Load(interactive bool, version string) (Settings, error) {
	if err := os.MkdirAll(m.configDir, 0o700); err != nil {
		return Settings{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "create config directory", err)
	}

	fileCfg, err := m.readFileConfig()
	if err != nil {
		return Settings{}, err
	}
	m.loaded = fileCfg

	callbackTimeout, err := time.ParseDuration(m.viper.GetString("auth.callback_timeout"))
	if err != nil {
		return Settings{}, clierrors.Wrap(clierrors.KindValidation, "invalid auth callback timeout", err)
	}

	refreshBefore, err := time.ParseDuration(m.viper.GetString("auth.refresh_before"))
	if err != nil {
		return Settings{}, clierrors.Wrap(clierrors.KindValidation, "invalid auth refresh buffer", err)
	}
	if refreshBefore < 0 {
		return Settings{}, clierrors.New(clierrors.KindValidation, "auth refresh buffer must not be negative")
	}

	scopes := stringSliceValue(m.viper, "auth.scopes")
	if len(scopes) == 0 && len(fileCfg.Scopes) > 0 {
		scopes = append([]string(nil), fileCfg.Scopes...)
	}
	scopes = NormalizeScopes(scopes)

	profileName, clientID, err := m.resolveProfile(fileCfg)
	if err != nil {
		return Settings{}, err
	}

	settings := Settings{
		ConfigDir:         m.configDir,
		ConfigFilePath:    m.configFile,
		SessionFilePath:   filepath.Join(m.configDir, defaultSessionFileName),
		TokenFallbackPath: filepath.Join(m.configDir, defaultTokenFileName),
		TokenKeyPath:      filepath.Join(m.configDir, defaultTokenKeyName),
		TokenSaltPath:     filepath.Join(m.configDir, defaultTokenSaltName),
		TokenLockPath:     filepath.Join(m.configDir, defaultLockFileName),
		ProfileName:       profileName,
		ClientID:          clientID,
		OutputJSON:        m.viper.GetBool("output.json"),
		Quiet:             m.viper.GetBool("output.quiet"),
		NoBrowser:         m.viper.GetBool("auth.no_browser"),
		CallbackTimeout:   callbackTimeout,
		RefreshBefore:     refreshBefore,
		Interactive:       interactive,
		XeroScopes:        scopes,
		OpenCommand:       m.viper.GetString("auth.open_command"),
		Version:           version,
	}

	if fileCfg.OutputMode == "json" && !settings.OutputJSON {
		settings.OutputJSON = true
	}
	if fileCfg.OutputMode == "quiet" && !settings.Quiet {
		settings.Quiet = true
		settings.OutputJSON = true
	}

	return settings, nil
}

func (m *Manager) LoadedConfig() FileConfig {
	return m.loaded
}

func (m *Manager) SetOutputMode(mode string) error {
	cfg := m.loaded
	cfg.OutputMode = mode
	return m.save(cfg)
}

func (m *Manager) readFileConfig() (FileConfig, error) {
	data, err := os.ReadFile(m.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return FileConfig{}, nil
		}
		return FileConfig{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "read config file", err)
	}

	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "parse config file", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	return cfg, nil
}

func (m *Manager) save(cfg FileConfig) error {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	if err := os.MkdirAll(filepath.Dir(m.configFile), 0o700); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "create config file directory", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.KindInternal, "marshal config file", err)
	}
	data = append(data, '\n')

	tmp := m.configFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "write config file", err)
	}
	if err := os.Rename(tmp, m.configFile); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "replace config file", err)
	}
	m.loaded = cfg
	return nil
}

func (s Settings) OutputMode() string {
	if s.Quiet {
		return "quiet"
	}
	if s.OutputJSON {
		return "json"
	}
	return "human"
}

func (m *Manager) AddProfile(name, clientID string, force bool) error {
	name = strings.TrimSpace(name)
	clientID = strings.TrimSpace(clientID)
	if name == "" {
		return clierrors.New(clierrors.KindValidation, "profile name must not be empty")
	}
	if clientID == "" {
		return clierrors.New(clierrors.KindValidation, "client ID must not be empty")
	}
	cfg := m.loaded
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	if _, exists := cfg.Profiles[name]; exists && !force {
		return clierrors.New(clierrors.KindValidation, fmt.Sprintf("profile %q already exists; use --force to overwrite", name))
	}
	cfg.Profiles[name] = ProfileConfig{ClientID: clientID}
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = name
	}
	return m.save(cfg)
}

func (m *Manager) RemoveProfile(name string) error {
	name = strings.TrimSpace(name)
	cfg := m.loaded
	if cfg.Profiles == nil || cfg.Profiles[name].ClientID == "" {
		return clierrors.New(clierrors.KindValidation, fmt.Sprintf("profile %q not found", name))
	}
	delete(cfg.Profiles, name)
	if cfg.DefaultProfile == name {
		cfg.DefaultProfile = ""
		for candidate := range cfg.Profiles {
			cfg.DefaultProfile = candidate
			break
		}
	}
	return m.save(cfg)
}

func (m *Manager) SetDefaultProfile(name string) error {
	name = strings.TrimSpace(name)
	cfg := m.loaded
	if cfg.Profiles == nil || cfg.Profiles[name].ClientID == "" {
		return clierrors.New(clierrors.KindValidation, fmt.Sprintf("profile %q not found; run `xero profile add %s` first", name, name))
	}
	cfg.DefaultProfile = name
	return m.save(cfg)
}

func (m *Manager) resolveProfile(cfg FileConfig) (string, string, error) {
	inlineClientID := strings.TrimSpace(m.viper.GetString("client_id"))
	if inlineClientID != "" {
		profileName := strings.TrimSpace(m.viper.GetString("profile"))
		if profileName == "" {
			profileName = inlineProfileName
		}
		return profileName, inlineClientID, nil
	}

	profileName := strings.TrimSpace(m.viper.GetString("profile"))
	if profileName == "" {
		profileName = strings.TrimSpace(cfg.DefaultProfile)
	}
	if profileName == "" {
		return "", "", nil
	}
	profile := cfg.Profiles[profileName]
	if strings.TrimSpace(profile.ClientID) == "" {
		return "", "", clierrors.New(clierrors.KindValidation, fmt.Sprintf("profile %q not found; run `xero profile list` to see available profiles", profileName))
	}
	return profileName, strings.TrimSpace(profile.ClientID), nil
}

func DefaultScopes() []string {
	return []string{
		"openid",
		"profile",
		"email",
		"offline_access",
		"accounting.contacts",
		"accounting.settings",
		"accounting.invoices",
		"accounting.payments",
		"accounting.banktransactions",
		"accounting.manualjournals",
		"accounting.reports.aged.read",
		"accounting.reports.balancesheet.read",
		"accounting.reports.profitandloss.read",
		"accounting.reports.trialbalance.read",
		"accounting.budgets.read",
		"accounting.attachments",
	}
}

func NormalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return DefaultScopes()
	}
	required := []string{"openid", "profile", "email", "offline_access"}
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(required)+len(scopes))
	for _, scope := range append(required, scopes...) {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	return normalized
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var listDelimiter = regexp.MustCompile(`\s*,\s*|\s+`)

func stringSliceValue(v *viper.Viper, key string) []string {
	values := v.GetStringSlice(key)
	if len(values) > 0 {
		return append([]string(nil), values...)
	}
	raw := strings.TrimSpace(v.GetString(key))
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var parsed []string
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return parsed
		}
	}
	parts := listDelimiter.Split(raw, -1)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func ValidateLoginConfig(settings Settings) error {
	if strings.TrimSpace(settings.ClientID) == "" {
		return clierrors.New(clierrors.KindValidation, "no profile configured; run `xero profile add <name>` or pass --client-id")
	}
	if len(settings.XeroScopes) == 0 {
		return clierrors.New(clierrors.KindValidation, "missing Xero OAuth scopes; set XERO_SCOPES or pass --scope")
	}
	return nil
}

func DescribePaths(settings Settings) string {
	return fmt.Sprintf("config=%s session=%s tokens=%s token-key=%s", settings.ConfigFilePath, settings.SessionFilePath, settings.TokenFallbackPath, settings.TokenKeyPath)
}
