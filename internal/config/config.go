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
	defaultAuthFileName    = "auth.json"
	defaultSessionFileName = "session.json"
	defaultTokenFileName   = "tokens.json"
	defaultLockFileName    = "tokens.lock"
)

type FileConfig struct {
	DefaultTenantID   string   `json:"defaultTenantId,omitempty"`
	DefaultTenantName string   `json:"defaultTenantName,omitempty"`
	OutputMode        string   `json:"outputMode,omitempty"`
	Scopes            []string `json:"scopes,omitempty"`
}

type AuthConfig struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

type Settings struct {
	ConfigDir         string
	ConfigFilePath    string
	AuthFilePath      string
	SessionFilePath   string
	TokenFallbackPath string
	TokenLockPath     string
	ClientID          string
	ClientSecret      string
	OutputJSON        bool
	Quiet             bool
	NoBrowser         bool
	TenantOverride    string
	DefaultTenantID   string
	DefaultTenantName string
	CallbackTimeout   time.Duration
	RefreshAfter      time.Duration
	Interactive       bool
	XeroScopes        []string
	OpenCommand       string
	Version           string
}

type Manager struct {
	viper      *viper.Viper
	configDir  string
	configFile string
	authFile   string
	loaded     FileConfig
	loadedAuth AuthConfig
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
		authFile:   filepath.Join(configDir, defaultAuthFileName),
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
	v.SetDefault("auth.callback_timeout", "2m")
	v.SetDefault("auth.refresh_after", "25m")
	v.SetDefault("auth.open_command", "")
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

	authCfg, err := m.readAuthConfig()
	if err != nil {
		return Settings{}, err
	}
	m.loadedAuth = authCfg

	callbackTimeout, err := time.ParseDuration(m.viper.GetString("auth.callback_timeout"))
	if err != nil {
		return Settings{}, clierrors.Wrap(clierrors.KindValidation, "invalid auth callback timeout", err)
	}

	refreshAfter, err := time.ParseDuration(m.viper.GetString("auth.refresh_after"))
	if err != nil {
		return Settings{}, clierrors.Wrap(clierrors.KindValidation, "invalid auth refresh threshold", err)
	}

	scopes := stringSliceValue(m.viper, "auth.scopes")
	if len(scopes) == 0 && len(fileCfg.Scopes) > 0 {
		scopes = append([]string(nil), fileCfg.Scopes...)
	}

	settings := Settings{
		ConfigDir:         m.configDir,
		ConfigFilePath:    m.configFile,
		AuthFilePath:      m.authFile,
		SessionFilePath:   filepath.Join(m.configDir, defaultSessionFileName),
		TokenFallbackPath: filepath.Join(m.configDir, defaultTokenFileName),
		TokenLockPath:     filepath.Join(m.configDir, defaultLockFileName),
		ClientID:          firstNonEmpty(m.viper.GetString("auth.client_id"), authCfg.ClientID),
		ClientSecret:      firstNonEmpty(m.viper.GetString("auth.client_secret"), authCfg.ClientSecret),
		OutputJSON:        m.viper.GetBool("output.json"),
		Quiet:             m.viper.GetBool("output.quiet"),
		NoBrowser:         m.viper.GetBool("auth.no_browser"),
		TenantOverride:    firstNonEmpty(m.viper.GetString("tenant"), fileCfg.DefaultTenantID),
		DefaultTenantID:   fileCfg.DefaultTenantID,
		DefaultTenantName: fileCfg.DefaultTenantName,
		CallbackTimeout:   callbackTimeout,
		RefreshAfter:      refreshAfter,
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

func (m *Manager) LoadedAuthConfig() AuthConfig {
	return m.loadedAuth
}

func (m *Manager) UpdateDefaultTenant(id, name string) error {
	cfg := m.loaded
	cfg.DefaultTenantID = id
	cfg.DefaultTenantName = name
	return m.save(cfg)
}

func (m *Manager) ClearDefaultTenant() error {
	cfg := m.loaded
	cfg.DefaultTenantID = ""
	cfg.DefaultTenantName = ""
	return m.save(cfg)
}

func (m *Manager) SetOutputMode(mode string) error {
	cfg := m.loaded
	cfg.OutputMode = mode
	return m.save(cfg)
}

func (m *Manager) PersistAuthCredentials(clientID, clientSecret string) error {
	authCfg := m.loadedAuth
	if strings.TrimSpace(clientID) != "" {
		authCfg.ClientID = clientID
	}
	if strings.TrimSpace(clientSecret) != "" {
		authCfg.ClientSecret = clientSecret
	}
	if authCfg == m.loadedAuth {
		return nil
	}
	return m.saveAuthConfig(authCfg)
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
	return cfg, nil
}

func (m *Manager) readAuthConfig() (AuthConfig, error) {
	data, err := os.ReadFile(m.authFile)
	if err != nil {
		if os.IsNotExist(err) {
			return AuthConfig{}, nil
		}
		return AuthConfig{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "read auth config file", err)
	}

	var cfg AuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AuthConfig{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "parse auth config file", err)
	}
	return cfg, nil
}

func (m *Manager) save(cfg FileConfig) error {
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

func (m *Manager) saveAuthConfig(cfg AuthConfig) error {
	if err := os.MkdirAll(filepath.Dir(m.authFile), 0o700); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "create auth config directory", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.KindInternal, "marshal auth config file", err)
	}
	data = append(data, '\n')

	tmp := m.authFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "write auth config file", err)
	}
	if err := os.Rename(tmp, m.authFile); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "replace auth config file", err)
	}
	m.loadedAuth = cfg
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
		return clierrors.New(clierrors.KindValidation, "missing Xero OAuth client ID; set XERO_AUTH_CLIENT_ID or use --client-id")
	}
	if len(settings.XeroScopes) == 0 {
		return clierrors.New(clierrors.KindValidation, "missing Xero OAuth scopes; set XERO_AUTH_SCOPES or add `scopes` to ~/.config/xero/config.json")
	}
	return nil
}

func DescribePaths(settings Settings) string {
	return fmt.Sprintf("config=%s auth=%s session=%s token-fallback=%s", settings.ConfigFilePath, settings.AuthFilePath, settings.SessionFilePath, settings.TokenFallbackPath)
}
