package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	"github.com/inscoder/xero-cli/internal/commands"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/viper"
)

const integrationContactID = "220ddca8-3144-4085-9a88-2d72c5133734"

type integrationContactTransport func(*http.Request) (*http.Response, error)

func (transport integrationContactTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func integrationContactResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestContactsIntegrationRefreshesSelectedTenantAndRunsLifecycle(t *testing.T) {
	requestCount := 0
	transport := integrationContactTransport(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if got := request.Header.Get("Authorization"); got != "Bearer fresh-token" {
			t.Fatalf("request %d used unexpected token %q", requestCount, got)
		}
		if got := request.Header.Get("Xero-tenant-id"); got != "tenant-selected" {
			t.Fatalf("request %d used unexpected tenant %q", requestCount, got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("request %d used unexpected Accept %q", requestCount, got)
		}

		switch requestCount {
		case 1:
			if request.Method != http.MethodGet || request.URL.Path != "/api.xro/2.0/Contacts" || request.URL.Query().Get("searchTerm") != "Integration" || request.URL.Query().Get("page") != "1" {
				t.Fatalf("unexpected list request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
			}
			return integrationContactResponse(`{"Contacts":[{"ContactID":"` + integrationContactID + `","Name":"Integration Contact","ContactStatus":"ACTIVE","Phones":[]}]}`), nil
		case 2:
			if request.Method != http.MethodPut || request.URL.Path != "/api.xro/2.0/Contacts" || request.URL.RawQuery != "summarizeErrors=true" || request.Header.Get("Idempotency-Key") != "integration-create-key" {
				t.Fatalf("unexpected create request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
			}
			body, _ := io.ReadAll(request.Body)
			want := `{"Contacts":[{"Name":"Integration Contact","EmailAddress":"contact@example.invalid"}]}`
			if string(body) != want {
				t.Fatalf("unexpected create body: got %s want %s", body, want)
			}
			return integrationContactResponse(`{"Contacts":[{"ContactID":"` + integrationContactID + `","Name":"Integration Contact","ContactStatus":"ACTIVE"}]}`), nil
		case 3:
			if request.Method != http.MethodPost || request.URL.Path != "/api.xro/2.0/Contacts/"+integrationContactID || request.URL.RawQuery != "" || request.Header.Get("Idempotency-Key") != "integration-update-key" {
				t.Fatalf("unexpected update request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
			}
			body, _ := io.ReadAll(request.Body)
			want := `{"Contacts":[{"ContactID":"` + integrationContactID + `","Name":"Integration Contact Updated"}]}`
			if string(body) != want {
				t.Fatalf("unexpected update body: got %s want %s", body, want)
			}
			return integrationContactResponse(`{"Contacts":[{"ContactID":"` + integrationContactID + `","Name":"Integration Contact Updated","ContactStatus":"ACTIVE"}]}`), nil
		case 4:
			if request.Method != http.MethodPost || request.URL.Path != "/api.xro/2.0/Contacts/"+integrationContactID || request.Header.Get("Idempotency-Key") != "integration-archive-key" {
				t.Fatalf("unexpected archive request: %s %s", request.Method, request.URL.Path)
			}
			body, _ := io.ReadAll(request.Body)
			want := `{"Contacts":[{"ContactID":"` + integrationContactID + `","ContactStatus":"ARCHIVED"}]}`
			if string(body) != want {
				t.Fatalf("unexpected archive body: got %s want %s", body, want)
			}
			return integrationContactResponse(`{"Contacts":[{"ContactID":"` + integrationContactID + `","Name":"Integration Contact Updated","ContactStatus":"ARCHIVED"}]}`), nil
		default:
			t.Fatalf("unexpected extra request %d: %s %s", requestCount, request.Method, request.URL.Path)
			return nil, nil
		}
	})

	tempDir := t.TempDir()
	configPath := tempDir + "/config.json"
	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)
	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Load(false, "test"); err != nil {
		t.Fatalf("load initial settings: %v", err)
	}
	if err := manager.AddProfile("demo", "client-id", true); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	v.Set("profile", "demo")
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	tokens := auth.NewTokenStore(settings)
	oldToken := auth.TokenSet{
		AccessToken: "stale-token", RefreshToken: "refresh-token",
		GeneratedAt: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		ExpiresAt:   time.Date(2026, 7, 11, 0, 30, 0, 0, time.UTC),
		AuthMode:    "browser_oauth", TenantID: "tenant-selected", TenantName: "Demo Company",
	}
	if err := tokens.Save(oldToken); err != nil {
		t.Fatalf("save old token: %v", err)
	}
	session := auth.NewSessionStore(settings.SessionFilePath)
	if err := session.Save(auth.SessionMetadata{
		Authenticated: true,
		AuthMode:      "browser_oauth",
		KnownTenants:  []auth.Tenant{{ID: "tenant-other", Name: "Other"}, {ID: "tenant-selected", Name: "Demo Company"}},
		GeneratedAt:   oldToken.GeneratedAt,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	httpClient := &http.Client{Transport: transport}
	deps := commands.Dependencies{
		Version: "test",
		IO:      commands.IOStreams{In: bytes.NewBuffer(nil), Out: stdout, ErrOut: stderr},
		NewViper: func() *viper.Viper {
			return viper.New()
		},
		NewTokenStore:   func(appconfig.Settings) auth.TokenStore { return auth.NewTokenStore(settings) },
		NewSessionStore: auth.NewSessionStore,
		NewInvoiceClient: func(appconfig.Settings) xeroapi.InvoiceClient {
			return xeroapi.NewClient(settings, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: httpClient})
		},
		NewContactClient: func(appconfig.Settings) xeroapi.ContactClient {
			return xeroapi.NewClient(settings, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: httpClient})
		},
		NewBrowserAuth: func(_ appconfig.Settings, store auth.TokenStore, _ *auth.TenantStore, _ io.Reader, _ io.Writer) commands.Authenticator {
			return fakeIntegrationAuth{store: store}
		},
		IsTerminal:     func(int) bool { return false },
		LookPath:       func(string) error { return nil },
		Now:            func() time.Time { return time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC) },
		ContextFactory: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		PostRefreshState: func(runtime *commands.Runtime, token auth.TokenSet, refreshed bool) error {
			if !refreshed {
				return nil
			}
			return runtime.Tenants.UpdateRefreshState(token, runtime.SessionMeta.KnownTenants, runtime.Tokens.StorageMode(), runtime.Tokens.FallbackPath())
		},
	}

	run := func(args ...string) string {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		cmd := commands.NewRootCommand(deps)
		cmd.SetArgs(append([]string{"--config", configPath, "-p", "demo"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("execute %v wrote stderr: %s", args, stderr.String())
		}
		return stdout.String()
	}

	if output := run("contacts", "list", "--search", "Integration", "--json"); !strings.Contains(output, `"contactId": "`+integrationContactID+`"`) {
		t.Fatalf("unexpected list output: %s", output)
	}
	if output := run("contacts", "create", "--name", "Integration Contact", "--email", "contact@example.invalid", "--idempotency-key", "integration-create-key", "--json"); !strings.Contains(output, `"operation": "created"`) {
		t.Fatalf("unexpected create output: %s", output)
	}
	if output := run("contacts", "update", "--contact-id", integrationContactID, "--name", "Integration Contact Updated", "--idempotency-key", "integration-update-key", "--json"); !strings.Contains(output, `"name": "Integration Contact Updated"`) {
		t.Fatalf("unexpected update output: %s", output)
	}
	if output := run("contacts", "update", "--contact-id", integrationContactID, "--status", "ARCHIVED", "--confirm-archive", "--idempotency-key", "integration-archive-key", "--json"); !strings.Contains(output, `"status": "ARCHIVED"`) {
		t.Fatalf("unexpected archive output: %s", output)
	}

	if requestCount != 4 {
		t.Fatalf("expected four lifecycle requests, got %d", requestCount)
	}
	refreshedToken, err := tokens.Load()
	if err != nil {
		t.Fatalf("load refreshed token: %v", err)
	}
	if refreshedToken.AccessToken != "fresh-token" || refreshedToken.TenantID != "tenant-selected" {
		t.Fatalf("unexpected refreshed token/tenant: %+v", refreshedToken)
	}
}
