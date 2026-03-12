package config_test

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/inscoder/xero-cli/internal/config"
	"github.com/spf13/viper"
)

func TestLoadAppliesFlagEnvThenPersistedConfigPrecedence(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{\n  \"defaultTenantId\": \"tenant-file\",\n  \"defaultTenantName\": \"File Tenant\",\n  \"outputMode\": \"json\",\n  \"scopes\": [\"accounting.settings.read\"]\n}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("XERO_AUTH_CLIENT_ID", "client-from-env")
	t.Setenv("XERO_TENANT", "tenant-from-env")
	t.Setenv("XERO_AUTH_SCOPES", "openid profile email offline_access accounting.transactions accounting.contacts accounting.settings.read accounting.reports.read")

	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)
	v.Set("tenant", "tenant-from-flag")

	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if settings.TenantOverride != "tenant-from-flag" {
		t.Fatalf("expected flag tenant override, got %q", settings.TenantOverride)
	}
	if settings.ClientID != "client-from-env" {
		t.Fatalf("expected env client id, got %q", settings.ClientID)
	}
	if settings.DefaultTenantID != "tenant-file" {
		t.Fatalf("expected persisted default tenant, got %q", settings.DefaultTenantID)
	}
	if !settings.OutputJSON {
		t.Fatal("expected persisted json output mode to load")
	}
	if len(settings.XeroScopes) != 8 || settings.XeroScopes[0] != "openid" || settings.XeroScopes[7] != "accounting.reports.read" {
		t.Fatalf("expected env scopes to override config, got %#v", settings.XeroScopes)
	}
}

func TestLoadUsesConfigScopesWhenEnvMissing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{\n  \"scopes\": [\"openid\", \"profile\", \"email\"]\n}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)

	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if len(settings.XeroScopes) != 3 || settings.XeroScopes[0] != "openid" || settings.XeroScopes[2] != "email" {
		t.Fatalf("expected config scopes, got %#v", settings.XeroScopes)
	}
}

func TestLoadUsesPersistedAuthCredentialsWhenEnvMissing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	authPath := filepath.Join(tempDir, "auth.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(authPath, []byte("{\n  \"clientId\": \"client-from-auth\",\n  \"clientSecret\": \"secret-from-auth\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write auth config: %v", err)
	}

	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)

	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if settings.ClientID != "client-from-auth" {
		t.Fatalf("expected persisted client id, got %q", settings.ClientID)
	}
	if settings.ClientSecret != "secret-from-auth" {
		t.Fatalf("expected persisted client secret, got %q", settings.ClientSecret)
	}
	if settings.AuthFilePath != authPath {
		t.Fatalf("expected auth file path %q, got %q", authPath, settings.AuthFilePath)
	}
}

func TestPersistAuthCredentialsWritesSeparateSecretFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)

	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Load(false, "test"); err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if err := manager.PersistAuthCredentials("client-123", "secret-123"); err != nil {
		t.Fatalf("persist auth credentials: %v", err)
	}

	authPath := filepath.Join(tempDir, "auth.json")
	data, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	if got := string(data); got != "{\n  \"clientId\": \"client-123\",\n  \"clientSecret\": \"secret-123\"\n}\n" {
		t.Fatalf("unexpected auth file contents:\n%s", got)
	}
	if manager.LoadedAuthConfig().ClientSecret != "secret-123" {
		t.Fatalf("expected loaded auth config to update, got %+v", manager.LoadedAuthConfig())
	}
}

func TestConfigureViperLoadsDotEnvForDevelopment(t *testing.T) {
	tempDir := t.TempDir()
	originalClientID, hadClientID := os.LookupEnv("XERO_AUTH_CLIENT_ID")
	originalClientSecret, hadClientSecret := os.LookupEnv("XERO_AUTH_CLIENT_SECRET")
	originalScopes, hadScopes := os.LookupEnv("XERO_AUTH_SCOPES")
	defer func() {
		if hadClientID {
			_ = os.Setenv("XERO_AUTH_CLIENT_ID", originalClientID)
		} else {
			_ = os.Unsetenv("XERO_AUTH_CLIENT_ID")
		}
		if hadClientSecret {
			_ = os.Setenv("XERO_AUTH_CLIENT_SECRET", originalClientSecret)
		} else {
			_ = os.Unsetenv("XERO_AUTH_CLIENT_SECRET")
		}
		if hadScopes {
			_ = os.Setenv("XERO_AUTH_SCOPES", originalScopes)
		} else {
			_ = os.Unsetenv("XERO_AUTH_SCOPES")
		}
	}()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("XERO_AUTH_CLIENT_ID=client-from-dotenv\nXERO_AUTH_CLIENT_SECRET=secret-from-dotenv\nXERO_AUTH_SCOPES=openid profile email\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)

	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if settings.ClientID != "client-from-dotenv" {
		t.Fatalf("expected .env client id, got %q", settings.ClientID)
	}
	if settings.ClientSecret != "secret-from-dotenv" {
		t.Fatalf("expected .env client secret, got %q", settings.ClientSecret)
	}
	if len(settings.XeroScopes) != 3 || settings.XeroScopes[2] != "email" {
		t.Fatalf("expected .env scopes, got %#v", settings.XeroScopes)
	}
}

func TestValidateLoginConfigRequiresScopes(t *testing.T) {
	err := appconfig.ValidateLoginConfig(appconfig.Settings{ClientID: "client-123"})
	if err == nil {
		t.Fatal("expected missing scopes error")
	}
	if got := err.Error(); got != "missing Xero OAuth scopes; set XERO_AUTH_SCOPES or add `scopes` to ~/.config/xero/config.json" {
		t.Fatalf("unexpected error: %s", got)
	}
}
