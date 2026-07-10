package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	"github.com/inscoder/xero-cli/internal/commands"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/viper"
)

const writeIntegrationInvoiceID = "220ddca8-3144-4085-9a88-2d72c5133734"

func TestInvoiceCreateIntegrationRefreshesAndUsesTenant(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		assertWriteIntegrationHeaders(t, r)
		if r.Method != http.MethodPut || r.URL.Path != "/api.xro/2.0/Invoices" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"Type":"ACCREC"`)) || !bytes.Contains(body, []byte(`"Status":"DRAFT"`)) {
			t.Fatalf("unexpected create body: %s", body)
		}
		_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"220ddca8-3144-4085-9a88-2d72c5133734","Type":"ACCREC","InvoiceNumber":"INV-LIVE-TEST","Status":"DRAFT"}]}`)
	}))
	defer server.Close()

	harness := newWriteIntegrationHarness(t, server, bytes.NewBuffer(nil))
	inputPath := filepath.Join(harness.tempDir, "invoice-create.json")
	if err := os.WriteFile(inputPath, []byte(`{"contactId":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0","reference":"integration-create","lineItems":[{"description":"Test line"}]}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	cmd := commands.NewRootCommand(harness.deps)
	cmd.SetArgs([]string{"--config", harness.configPath, "invoices", "create", "--file", inputPath, "--idempotency-key", "integration-create", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute create: %v", err)
	}
	if requestCount != 1 || !strings.Contains(harness.stdout.String(), `"operation": "created"`) {
		t.Fatalf("unexpected request count/output: count=%d output=%s", requestCount, harness.stdout.String())
	}
	harness.assertRefreshPersisted(t)
}

func TestBillUpdateIntegrationPreflightsThenMutates(t *testing.T) {
	preflightCount := 0
	mutationCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertWriteIntegrationHeaders(t, r)
		if r.URL.Path != "/api.xro/2.0/Invoices/"+writeIntegrationInvoiceID {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			preflightCount++
			_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"220ddca8-3144-4085-9a88-2d72c5133734","Type":"ACCPAY","Reference":"old","LineItems":[{"LineItemID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0","Description":"Existing"}]}]}`)
		case http.MethodPost:
			mutationCount++
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte(`"Type":"ACCPAY"`)) || !bytes.Contains(body, []byte(`"Reference":"new"`)) || bytes.Contains(body, []byte(`"LineItems"`)) {
				t.Fatalf("unexpected update body: %s", body)
			}
			_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"220ddca8-3144-4085-9a88-2d72c5133734","Type":"ACCPAY","InvoiceNumber":"SUPPLIER-1","Reference":"new","Status":"DRAFT"}]}`)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	harness := newWriteIntegrationHarness(t, server, bytes.NewBuffer(nil))
	inputPath := filepath.Join(harness.tempDir, "bill-update.json")
	if err := os.WriteFile(inputPath, []byte(`{"reference":"new"}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	cmd := commands.NewRootCommand(harness.deps)
	cmd.SetArgs([]string{"--config", harness.configPath, "bills", "update", "--invoice-id", writeIntegrationInvoiceID, "--file", inputPath, "--idempotency-key", "integration-update", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute update: %v", err)
	}
	if preflightCount != 1 || mutationCount != 1 || !strings.Contains(harness.stdout.String(), `"resource": "bill"`) {
		t.Fatalf("unexpected counts/output: preflight=%d mutation=%d output=%s", preflightCount, mutationCount, harness.stdout.String())
	}
}

func TestInvoiceAttachmentIntegrationPreflightsThenStreams(t *testing.T) {
	preflightCount := 0
	uploadCount := 0
	content := []byte("%PDF-1.7\nintegration attachment\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertWriteIntegrationHeaders(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api.xro/2.0/Invoices/"+writeIntegrationInvoiceID:
			preflightCount++
			_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"220ddca8-3144-4085-9a88-2d72c5133734","Type":"ACCREC","Attachments":[]}]}`)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/Attachments/integration.pdf"):
			uploadCount++
			body, _ := io.ReadAll(r.Body)
			if !bytes.Equal(body, content) || r.ContentLength != int64(len(content)) || r.URL.Query().Get("IncludeOnline") != "true" {
				t.Fatalf("unexpected upload: body=%q length=%d query=%s", body, r.ContentLength, r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"Attachments":[{"AttachmentID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0","FileName":"integration.pdf","MimeType":"application/pdf","ContentLength":32,"IncludeOnline":true}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	harness := newWriteIntegrationHarness(t, server, bytes.NewBuffer(nil))
	filePath := filepath.Join(harness.tempDir, "integration.pdf")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	cmd := commands.NewRootCommand(harness.deps)
	cmd.SetArgs([]string{"--config", harness.configPath, "invoices", "attachments", "upload", "--invoice-id", writeIntegrationInvoiceID, "--file", filePath, "--include-online", "--idempotency-key", "integration-attachment", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute attachment upload: %v", err)
	}
	if preflightCount != 1 || uploadCount != 1 || !strings.Contains(harness.stdout.String(), `"operation": "uploaded"`) {
		t.Fatalf("unexpected counts/output: preflight=%d upload=%d output=%s", preflightCount, uploadCount, harness.stdout.String())
	}
}

type writeIntegrationHarness struct {
	tempDir    string
	configPath string
	deps       commands.Dependencies
	stdout     *bytes.Buffer
	tokens     auth.TokenStore
}

func newWriteIntegrationHarness(t *testing.T, server *httptest.Server, in io.Reader) writeIntegrationHarness {
	t.Helper()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)
	v.Set("client_id", "client-id")
	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	tokens := auth.NewTokenStore(settings)
	oldToken := auth.TokenSet{
		AccessToken: "stale-token", RefreshToken: "refresh-token", AuthMode: "browser_oauth",
		GeneratedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 7, 10, 9, 30, 0, 0, time.UTC),
		TenantID: "tenant-1", TenantName: "Acme",
	}
	if err := tokens.Save(oldToken); err != nil {
		t.Fatalf("save token: %v", err)
	}
	session := auth.NewSessionStore(settings.SessionFilePath)
	if err := session.Save(auth.SessionMetadata{Authenticated: true, AuthMode: "browser_oauth", KnownTenants: []auth.Tenant{{ID: "tenant-1", Name: "Acme"}}, GeneratedAt: oldToken.GeneratedAt}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	stdout := &bytes.Buffer{}
	deps := commands.Dependencies{
		Version:         "test",
		IO:              commands.IOStreams{In: in, Out: stdout, ErrOut: &bytes.Buffer{}},
		NewViper:        viper.New,
		NewTokenStore:   func(appconfig.Settings) auth.TokenStore { return tokens },
		NewSessionStore: auth.NewSessionStore,
		NewInvoiceClient: func(appconfig.Settings) xeroapi.InvoiceClient {
			return xeroapi.NewClient(settings, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
		},
		NewBrowserAuth: func(_ appconfig.Settings, store auth.TokenStore, _ *auth.TenantStore, _ io.Reader, _ io.Writer) commands.Authenticator {
			return fakeIntegrationAuth{store: store}
		},
		IsTerminal:     func(int) bool { return false },
		LookPath:       func(string) error { return nil },
		Now:            func() time.Time { return time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC) },
		ContextFactory: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		PostRefreshState: func(rt *commands.Runtime, token auth.TokenSet, refreshed bool) error {
			if !refreshed {
				return nil
			}
			return rt.Tenants.UpdateRefreshState(token, rt.SessionMeta.KnownTenants, rt.Tokens.StorageMode(), rt.Tokens.FallbackPath())
		},
	}
	return writeIntegrationHarness{tempDir: tempDir, configPath: configPath, deps: deps, stdout: stdout, tokens: tokens}
}

func (h writeIntegrationHarness) assertRefreshPersisted(t *testing.T) {
	t.Helper()
	token, err := h.tokens.Load()
	if err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token.AccessToken != "fresh-token" {
		t.Fatalf("expected fresh token, got %q", token.AccessToken)
	}
}

func assertWriteIntegrationHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer fresh-token" || r.Header.Get("Xero-tenant-id") != "tenant-1" {
		t.Fatalf("unexpected auth headers: %+v", r.Header)
	}
}
