package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cesar/xero-cli/internal/auth"
	"github.com/cesar/xero-cli/internal/commands"
	appconfig "github.com/cesar/xero-cli/internal/config"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/viper"
)

func TestInvoicesIntegrationRefreshesThenCallsXeroAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.xro/2.0/Invoices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fresh-token" {
			t.Fatalf("expected refreshed token, got %q", got)
		}
		if got := r.Header.Get("Xero-tenant-id"); got != "tenant-1" {
			t.Fatalf("expected tenant header, got %q", got)
		}
		_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"1","InvoiceNumber":"INV-1000","Contact":{"Name":"Acme"},"Status":"AUTHORISED","Total":12.34,"AmountDue":4.56,"CurrencyCode":"USD","DueDate":"2026-03-10T00:00:00","UpdatedDateUTC":"2026-03-09T12:30:00Z"}]}`)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
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
	if err := manager.UpdateDefaultTenant("tenant-1", "Acme"); err != nil {
		t.Fatalf("update tenant: %v", err)
	}

	tokens := auth.NewTokenStore(settings)
	oldToken := auth.TokenSet{AccessToken: "stale-token", RefreshToken: "refresh-token", GeneratedAt: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 3, 10, 11, 30, 0, 0, time.UTC), AuthMode: "browser_oauth"}
	if err := tokens.Save(oldToken); err != nil {
		t.Fatalf("save old token: %v", err)
	}
	session := auth.NewSessionStore(settings.SessionFilePath)
	if err := session.Save(auth.SessionMetadata{Authenticated: true, AuthMode: "browser_oauth", KnownTenants: []auth.Tenant{{ID: "tenant-1", Name: "Acme"}}, GeneratedAt: oldToken.GeneratedAt}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := commands.Dependencies{
		Version: "test",
		IO:      commands.IOStreams{In: bytes.NewBuffer(nil), Out: stdout, ErrOut: stderr},
		NewViper: func() *viper.Viper {
			return viper.New()
		},
		NewTokenStore:   func(appconfig.Settings) auth.TokenStore { return auth.NewTokenStore(settings) },
		NewSessionStore: auth.NewSessionStore,
		NewInvoiceClient: func(settings appconfig.Settings) xeroapi.InvoiceLister {
			return xeroapi.NewClient(settings, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
		},
		NewBrowserAuth: func(settings appconfig.Settings, store auth.TokenStore, tenants *auth.TenantStore, in io.Reader, errOut io.Writer) commands.Authenticator {
			return fakeIntegrationAuth{store: store}
		},
		IsTerminal:     func(int) bool { return false },
		LookPath:       func(string) error { return nil },
		Now:            func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) },
		ContextFactory: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		PostRefreshState: func(rt *commands.Runtime, token auth.TokenSet, refreshed bool) error {
			if !refreshed {
				return nil
			}
			return rt.Tenants.UpdateRefreshState(token, rt.SessionMeta.KnownTenants, rt.Tokens.StorageMode(), rt.Tokens.FallbackPath())
		},
	}

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("INV-1000")) {
		t.Fatalf("expected invoice output, got %s", stdout.String())
	}
	refreshedToken, err := tokens.Load()
	if err != nil {
		t.Fatalf("load refreshed token: %v", err)
	}
	if refreshedToken.AccessToken != "fresh-token" {
		t.Fatalf("expected refreshed token to persist, got %q", refreshedToken.AccessToken)
	}
	updatedSession, err := session.Load()
	if err != nil {
		t.Fatalf("load updated session: %v", err)
	}
	if updatedSession.LastRefreshAt.IsZero() {
		t.Fatal("expected refresh metadata to be recorded")
	}
}

type fakeIntegrationAuth struct {
	store auth.TokenStore
}

func (f fakeIntegrationAuth) Login(ctx context.Context) (auth.LoginResult, error) {
	return auth.LoginResult{}, fmt.Errorf("unexpected login call")
}

func (f fakeIntegrationAuth) EnsureFreshToken(ctx context.Context, token auth.TokenSet, interactive bool) (auth.TokenSet, bool, error) {
	refreshed := token
	refreshed.AccessToken = "fresh-token"
	refreshed.GeneratedAt = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	if err := f.store.Save(refreshed); err != nil {
		return auth.TokenSet{}, false, err
	}
	return refreshed, true, nil
}
