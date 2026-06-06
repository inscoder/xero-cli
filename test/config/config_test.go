package config_test

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/inscoder/xero-cli/internal/config"
	"github.com/spf13/viper"
)

func TestLoadResolvesInlineClientIDBeforeProfile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{\n  \"defaultProfile\": \"file\",\n  \"profiles\": {\n    \"file\": {\"clientId\": \"client-from-profile\"}\n  },\n  \"outputMode\": \"json\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("XERO_CLIENT_ID", "client-from-env")
	t.Setenv("XERO_SCOPES", "accounting.invoices.read accounting.contacts.read")

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

	if settings.ClientID != "client-from-env" {
		t.Fatalf("expected env client id, got %q", settings.ClientID)
	}
	if settings.ProfileName != "_inline" {
		t.Fatalf("expected inline profile, got %q", settings.ProfileName)
	}
	if !settings.OutputJSON {
		t.Fatal("expected persisted json output mode to load")
	}
	if len(settings.XeroScopes) != 6 || settings.XeroScopes[0] != "openid" || settings.XeroScopes[4] != "accounting.invoices.read" {
		t.Fatalf("expected required scopes to be prepended, got %#v", settings.XeroScopes)
	}
}

func TestLoadUsesDefaultProfile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{\n  \"defaultProfile\": \"acme\",\n  \"profiles\": {\n    \"acme\": {\"clientId\": \"client-acme\"}\n  }\n}\n"), 0o600); err != nil {
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

	if settings.ProfileName != "acme" || settings.ClientID != "client-acme" {
		t.Fatalf("unexpected profile resolution: profile=%q client=%q", settings.ProfileName, settings.ClientID)
	}
}

func TestProfileManagementPersistsProfiles(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

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
	if err := manager.AddProfile("acme", "client-acme", false); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	if err := manager.AddProfile("other", "client-other", false); err != nil {
		t.Fatalf("add second profile: %v", err)
	}
	if err := manager.SetDefaultProfile("other"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if err := manager.RemoveProfile("acme"); err != nil {
		t.Fatalf("remove profile: %v", err)
	}

	loaded := manager.LoadedConfig()
	if loaded.DefaultProfile != "other" || loaded.Profiles["other"].ClientID != "client-other" {
		t.Fatalf("unexpected config: %+v", loaded)
	}
	if _, exists := loaded.Profiles["acme"]; exists {
		t.Fatalf("expected acme profile to be removed: %+v", loaded.Profiles)
	}
}

func TestConfigureViperLoadsDotEnvForDevelopment(t *testing.T) {
	tempDir := t.TempDir()
	originalClientID, hadClientID := os.LookupEnv("XERO_CLIENT_ID")
	originalScopes, hadScopes := os.LookupEnv("XERO_SCOPES")
	defer func() {
		if hadClientID {
			_ = os.Setenv("XERO_CLIENT_ID", originalClientID)
		} else {
			_ = os.Unsetenv("XERO_CLIENT_ID")
		}
		if hadScopes {
			_ = os.Setenv("XERO_SCOPES", originalScopes)
		} else {
			_ = os.Unsetenv("XERO_SCOPES")
		}
	}()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("XERO_CLIENT_ID=client-from-dotenv\nXERO_SCOPES=accounting.invoices.read\n"), 0o600); err != nil {
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
	if len(settings.XeroScopes) != 5 || settings.XeroScopes[4] != "accounting.invoices.read" {
		t.Fatalf("expected .env scopes, got %#v", settings.XeroScopes)
	}
}

func TestValidateLoginConfigRequiresClientID(t *testing.T) {
	err := appconfig.ValidateLoginConfig(appconfig.Settings{})
	if err == nil {
		t.Fatal("expected missing client ID error")
	}
	if got := err.Error(); got != "no profile configured; run `xero profile add <name>` or pass --client-id" {
		t.Fatalf("unexpected error: %s", got)
	}
}
