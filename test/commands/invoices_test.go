package commands_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cesar/xero-cli/internal/auth"
	"github.com/cesar/xero-cli/internal/commands"
	appconfig "github.com/cesar/xero-cli/internal/config"
	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/viper"
)

type fakeAuth struct {
	loginResult auth.LoginResult
	ensure      func(context.Context, auth.TokenSet, bool) (auth.TokenSet, bool, error)
}

func (f fakeAuth) Login(ctx context.Context) (auth.LoginResult, error) { return f.loginResult, nil }

func (f fakeAuth) EnsureFreshToken(ctx context.Context, token auth.TokenSet, interactive bool) (auth.TokenSet, bool, error) {
	if f.ensure != nil {
		return f.ensure(ctx, token, interactive)
	}
	return token, false, nil
}

type fakeStore struct {
	token auth.TokenSet
	err   error
}

func (s *fakeStore) Load() (auth.TokenSet, error)   { return s.token, s.err }
func (s *fakeStore) Save(token auth.TokenSet) error { s.token = token; return nil }
func (s *fakeStore) Clear() error                   { s.token = auth.TokenSet{}; return nil }
func (s *fakeStore) StorageMode() string            { return "file:test" }
func (s *fakeStore) FallbackPath() string           { return "test" }

type fakeLister struct {
	request  xeroapi.ListInvoicesRequest
	invoices []xeroapi.Invoice
	err      error
}

func (f *fakeLister) ListInvoices(ctx context.Context, token auth.TokenSet, request xeroapi.ListInvoicesRequest) ([]xeroapi.Invoice, error) {
	f.request = request
	return f.invoices, f.err
}

func TestInvoicesCommandEmitsStableJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "INV-0001", ContactName: "Acme Ltd", Contact: xeroapi.InvoiceContact{Name: "Acme Ltd"}, Status: "AUTHORISED", CurrencyCode: "USD", Currency: "USD", LineItems: []xeroapi.InvoiceLineItem{}, Payments: []xeroapi.InvoicePayment{}, CreditNotes: []xeroapi.InvoiceAllocation{}, Prepayments: []xeroapi.InvoiceAllocation{}, Overpayments: []xeroapi.InvoiceAllocation{}}}}
	deps, stdout, stderr := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices: %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"invoiceNumber\": \"INV-0001\"") || !strings.Contains(stdout.String(), "\"summary\": \"1 invoice\"") || !strings.Contains(stdout.String(), "\"contact\": {") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if lister.request.TenantID != "tenant-1" {
		t.Fatalf("expected default tenant to be used, got %q", lister.request.TenantID)
	}
}

func TestInvoicesCommandPassesAdvancedFiltersToClient(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "INV-0001", LineItems: []xeroapi.InvoiceLineItem{}, Payments: []xeroapi.InvoicePayment{}, CreditNotes: []xeroapi.InvoiceAllocation{}, Prepayments: []xeroapi.InvoiceAllocation{}, Overpayments: []xeroapi.InvoiceAllocation{}}}}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734,88192a99-cbc5-4a66-bf1a-2f9fea2d36d0", "--status", "authorised,paid", "--where", `Type=="ACCPAY" AND AmountDue>=5000`, "--order", "Date asc", "--page", "2", "--page-size", "50", "--contact", "Acme", "--since", "2026-03-01", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices with filters: %v", err)
	}

	expectedIDs := []string{"220ddca8-3144-4085-9a88-2d72c5133734", "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"}
	if !reflect.DeepEqual(lister.request.InvoiceIDs, expectedIDs) {
		t.Fatalf("unexpected invoice IDs: %#v", lister.request.InvoiceIDs)
	}
	expectedStatuses := []string{"AUTHORISED", "PAID"}
	if !reflect.DeepEqual(lister.request.Statuses, expectedStatuses) {
		t.Fatalf("unexpected statuses: %#v", lister.request.Statuses)
	}
	if lister.request.Where != `Type=="ACCPAY" AND AmountDue>=5000` {
		t.Fatalf("unexpected where: %q", lister.request.Where)
	}
	if lister.request.Order != "Date ASC" {
		t.Fatalf("unexpected order: %q", lister.request.Order)
	}
	if lister.request.Page != 2 || lister.request.PageSize != 50 {
		t.Fatalf("unexpected paging: page=%d pageSize=%d", lister.request.Page, lister.request.PageSize)
	}
	if lister.request.Contact != "Acme" || lister.request.Since != "2026-03-01" {
		t.Fatalf("unexpected passthrough fields: %+v", lister.request)
	}
}

func TestInvoicesCommandRejectsPageSizeWithoutPage(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--page-size", "100", "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if lister.request.PageSize != 0 {
		t.Fatalf("expected client not to be called, got request %+v", lister.request)
	}
}

func TestInvoicesCommandRejectsUnknownStatus(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--status", "banana", "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestInvoicesCommandFailsWithTypedAuthErrorWhenSessionMissing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{err: auth.ErrTokenNotFound}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--no-browser", "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindAuthRequired {
		t.Fatalf("expected auth required error, got %v", err)
	}
}

func testDependencies(configPath string, store auth.TokenStore, lister xeroapi.InvoiceLister, interactive bool) (commands.Dependencies, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return commands.Dependencies{
		Version: "test",
		IO:      commands.IOStreams{In: bytes.NewBuffer(nil), Out: stdout, ErrOut: stderr},
		NewViper: func() *viper.Viper {
			return viper.New()
		},
		NewTokenStore:   func(appconfig.Settings) auth.TokenStore { return store },
		NewSessionStore: auth.NewSessionStore,
		NewInvoiceClient: func(appconfig.Settings) xeroapi.InvoiceLister {
			return lister
		},
		NewBrowserAuth: func(appconfig.Settings, auth.TokenStore, *auth.TenantStore, io.Reader, io.Writer) commands.Authenticator {
			return fakeAuth{}
		},
		IsTerminal:       func(int) bool { return interactive },
		LookPath:         func(string) error { return nil },
		Now:              func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) },
		ContextFactory:   func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		PostRefreshState: func(*commands.Runtime, auth.TokenSet, bool) error { return nil },
	}, stdout, stderr
}

func prepareConfig(t *testing.T, configPath string) {
	t.Helper()
	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", configPath)
	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Load(false, "test"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := manager.UpdateDefaultTenant("tenant-1", "Acme"); err != nil {
		t.Fatalf("update default tenant: %v", err)
	}
}

func prepareSession(t *testing.T, sessionPath string) {
	t.Helper()
	store := auth.NewSessionStore(sessionPath)
	if err := store.Save(auth.SessionMetadata{Authenticated: true, AuthMode: "browser_oauth", KnownTenants: []auth.Tenant{{ID: "tenant-1", Name: "Acme"}}}); err != nil {
		t.Fatalf("save session: %v", err)
	}
}
