package config_test

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/cesar/xero-cli/internal/config"
	"github.com/spf13/viper"
)

func TestLoadAppliesFlagEnvThenPersistedConfigPrecedence(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{\n  \"defaultTenantId\": \"tenant-file\",\n  \"defaultTenantName\": \"File Tenant\",\n  \"outputMode\": \"json\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("XERO_AUTH_CLIENT_ID", "client-from-env")
	t.Setenv("XERO_TENANT", "tenant-from-env")

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
}

func TestConfigureViperLoadsDotEnvForDevelopment(t *testing.T) {
	tempDir := t.TempDir()
	originalClientID, hadClientID := os.LookupEnv("XERO_AUTH_CLIENT_ID")
	originalClientSecret, hadClientSecret := os.LookupEnv("XERO_AUTH_CLIENT_SECRET")
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
	}()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("XERO_AUTH_CLIENT_ID=client-from-dotenv\nXERO_AUTH_CLIENT_SECRET=secret-from-dotenv\n"), 0o600); err != nil {
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
}
