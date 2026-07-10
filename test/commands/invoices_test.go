package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	"github.com/inscoder/xero-cli/internal/commands"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/viper"
)

type fakeAuth struct {
	loginResult auth.LoginResult
	loginErr    error
	ensure      func(context.Context, auth.TokenSet, bool) (auth.TokenSet, bool, error)
}

func (f fakeAuth) Login(ctx context.Context) (auth.LoginResult, error) {
	return f.loginResult, f.loginErr
}

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

func (s *fakeStore) Load() (auth.TokenSet, error) {
	if s.token.TenantID == "" {
		s.token.TenantID = "tenant-1"
		s.token.TenantName = "Acme"
	}
	return s.token, s.err
}
func (s *fakeStore) Save(token auth.TokenSet) error { s.token = token; return nil }
func (s *fakeStore) Clear() error                   { s.token = auth.TokenSet{}; return nil }
func (s *fakeStore) StorageMode() string            { return "file:test" }
func (s *fakeStore) FallbackPath() string           { return "test" }

type fakeLister struct {
	request        xeroapi.ListInvoicesRequest
	getRequest     xeroapi.GetInvoiceRequest
	onlineRequest  xeroapi.GetOnlineInvoiceRequest
	pdfRequest     xeroapi.GetInvoicePDFRequest
	approveReq     xeroapi.ApproveInvoiceRequest
	createReq      xeroapi.CreateInvoiceRequest
	updateReq      xeroapi.UpdateInvoiceRequest
	uploadReq      xeroapi.UploadInvoiceAttachmentRequest
	invoices       []xeroapi.Invoice
	onlineInvoice  xeroapi.OnlineInvoiceResult
	pdfResult      xeroapi.InvoicePDFResult
	approveResult  xeroapi.InvoiceApprovalResult
	mutationResult xeroapi.InvoiceMutationResult
	uploadResult   xeroapi.InvoiceAttachmentMutationResult
	uploadBody     []byte
	uploadCalls    int
	pdfContent     []byte
	err            error
}

func (f *fakeLister) ListInvoices(ctx context.Context, token auth.TokenSet, request xeroapi.ListInvoicesRequest) ([]xeroapi.Invoice, error) {
	f.request = request
	return f.invoices, f.err
}

func (f *fakeLister) GetInvoice(ctx context.Context, token auth.TokenSet, request xeroapi.GetInvoiceRequest) (xeroapi.Invoice, error) {
	f.getRequest = request
	if f.err != nil {
		return xeroapi.Invoice{}, f.err
	}
	if len(f.invoices) == 0 {
		return xeroapi.Invoice{}, nil
	}
	return f.invoices[0], nil
}

func (f *fakeLister) CreateInvoice(ctx context.Context, token auth.TokenSet, request xeroapi.CreateInvoiceRequest) (xeroapi.InvoiceMutationResult, error) {
	f.createReq = request
	result := f.mutationResult
	if result.Operation == "" {
		result.Operation = "created"
		result.Resource = request.Resource
		result.TenantID = request.TenantID
		result.Type = request.Invoice.Type
		result.IdempotencyKey = request.IdempotencyKey
	}
	return result, f.err
}

func (f *fakeLister) UpdateInvoice(ctx context.Context, token auth.TokenSet, request xeroapi.UpdateInvoiceRequest) (xeroapi.InvoiceMutationResult, error) {
	f.updateReq = request
	result := f.mutationResult
	if result.Operation == "" {
		result.Operation = "updated"
		result.Resource = request.Resource
		result.InvoiceID = request.InvoiceID
		result.TenantID = request.TenantID
		result.Type = request.Invoice.Type
		result.IdempotencyKey = request.IdempotencyKey
	}
	return result, f.err
}

func (f *fakeLister) UploadInvoiceAttachment(ctx context.Context, token auth.TokenSet, request xeroapi.UploadInvoiceAttachmentRequest) (xeroapi.InvoiceAttachmentMutationResult, error) {
	f.uploadReq = request
	f.uploadCalls++
	if f.err != nil {
		return xeroapi.InvoiceAttachmentMutationResult{}, f.err
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return xeroapi.InvoiceAttachmentMutationResult{}, err
	}
	f.uploadBody = body
	result := f.uploadResult
	if result.Operation == "" {
		result.Operation = "uploaded"
		if request.Replace {
			result.Operation = "replaced"
		}
		result.Resource = request.Resource
		result.InvoiceID = request.InvoiceID
		result.TenantID = request.TenantID
		result.Type = request.Type
		result.FileName = request.FileName
		result.ContentType = request.ContentType
		result.Bytes = request.ContentLength
		result.Overwritten = request.Replace
		result.IdempotencyKey = request.IdempotencyKey
		if request.Resource == "invoice" {
			includeOnline := request.IncludeOnline
			result.IncludeOnline = &includeOnline
		}
	}
	return result, nil
}

func (f *fakeLister) GetOnlineInvoice(ctx context.Context, token auth.TokenSet, request xeroapi.GetOnlineInvoiceRequest) (xeroapi.OnlineInvoiceResult, error) {
	f.onlineRequest = request
	return f.onlineInvoice, f.err
}

func (f *fakeLister) GetInvoicePDF(ctx context.Context, token auth.TokenSet, request xeroapi.GetInvoicePDFRequest, writer io.Writer) (xeroapi.InvoicePDFResult, error) {
	f.pdfRequest = request
	if f.err != nil {
		return xeroapi.InvoicePDFResult{}, f.err
	}
	if _, err := writer.Write(f.pdfContent); err != nil {
		return xeroapi.InvoicePDFResult{}, err
	}
	result := f.pdfResult
	result.InvoiceID = request.InvoiceID
	if result.ContentType == "" {
		result.ContentType = "application/pdf"
	}
	if result.Bytes == 0 {
		result.Bytes = int64(len(f.pdfContent))
	}
	return result, nil
}

func (f *fakeLister) ApproveInvoice(ctx context.Context, token auth.TokenSet, request xeroapi.ApproveInvoiceRequest) (xeroapi.InvoiceApprovalResult, error) {
	f.approveReq = request
	return f.approveResult, f.err
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
	if lister.request.Type != "ACCREC" {
		t.Fatalf("expected invoice type ACCREC, got %q", lister.request.Type)
	}
	if lister.request.Page != 1 || lister.request.PageSize != 0 {
		t.Fatalf("expected default first page without explicit page size, got page=%d pageSize=%d", lister.request.Page, lister.request.PageSize)
	}
}

func TestLoginUsesInlineClientID(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("XERO_SCOPES", "accounting.invoices.read")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := commands.Dependencies{
		Version: "test",
		IO:      commands.IOStreams{In: bytes.NewBuffer(nil), Out: stdout, ErrOut: stderr},
		NewViper: func() *viper.Viper {
			return viper.New()
		},
		NewTokenStore:   func(appconfig.Settings) auth.TokenStore { return &fakeStore{} },
		NewSessionStore: auth.NewSessionStore,
		NewInvoiceClient: func(appconfig.Settings) xeroapi.InvoiceClient {
			return &fakeLister{}
		},
		NewBrowserAuth: func(appconfig.Settings, auth.TokenStore, *auth.TenantStore, io.Reader, io.Writer) commands.Authenticator {
			return fakeAuth{loginResult: auth.LoginResult{
				Token:   auth.TokenSet{AuthMode: "browser_oauth", GeneratedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 3, 10, 12, 30, 0, 0, time.UTC)},
				Tenants: []auth.Tenant{{ID: "tenant-1", Name: "Acme", Type: "ORGANISATION"}},
				Default: auth.Tenant{ID: "tenant-1", Name: "Acme", Type: "ORGANISATION"},
			}}
		},
		IsTerminal:       func(int) bool { return false },
		LookPath:         func(string) error { return nil },
		Now:              func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) },
		ContextFactory:   func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		PostRefreshState: func(*commands.Runtime, auth.TokenSet, bool) error { return nil },
	}

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "--client-id", "client-from-flag", "login", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute login: %v", err)
	}
	if !strings.Contains(stdout.String(), `"summary": "Logged in to 1 tenant(s)"`) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
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
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734,88192a99-cbc5-4a66-bf1a-2f9fea2d36d0", "--status", "authorised,paid", "--where", `AmountDue>=5000`, "--order", "Date asc", "--page", "2", "--page-size", "50", "--since", "2026-03-01", "--json"})
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
	if lister.request.Type != "ACCREC" {
		t.Fatalf("unexpected type: %q", lister.request.Type)
	}
	if lister.request.Where != `AmountDue>=5000` {
		t.Fatalf("unexpected where: %q", lister.request.Where)
	}
	if lister.request.Order != "Date ASC" {
		t.Fatalf("unexpected order: %q", lister.request.Order)
	}
	if lister.request.Page != 2 || lister.request.PageSize != 50 {
		t.Fatalf("unexpected paging: page=%d pageSize=%d", lister.request.Page, lister.request.PageSize)
	}
	if lister.request.Since != "2026-03-01" {
		t.Fatalf("unexpected passthrough fields: %+v", lister.request)
	}
}

func TestBillsCommandPassesPurchaseBillTypeAndFiltersToClient(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "BILL-0001", Type: "ACCPAY", LineItems: []xeroapi.InvoiceLineItem{}, Payments: []xeroapi.InvoicePayment{}, CreditNotes: []xeroapi.InvoiceAllocation{}, Prepayments: []xeroapi.InvoiceAllocation{}, Overpayments: []xeroapi.InvoiceAllocation{}}}}
	deps, stdout, stderr := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "bills", "--status", "AUTHORISED", "--where", `AmountDue>=5000`, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute bills with filters: %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if lister.request.Type != "ACCPAY" {
		t.Fatalf("expected bill type ACCPAY, got %q", lister.request.Type)
	}
	if lister.request.Where != `AmountDue>=5000` {
		t.Fatalf("unexpected where: %q", lister.request.Where)
	}
	if lister.request.Page != 1 || lister.request.PageSize != 0 {
		t.Fatalf("unexpected paging: page=%d pageSize=%d", lister.request.Page, lister.request.PageSize)
	}
	if !strings.Contains(stdout.String(), `"summary": "1 bill"`) || !strings.Contains(stdout.String(), `"cmd": "xero bills --json"`) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestInvoicesCommandUsesDefaultPageWithPageSize(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "INV-0001", LineItems: []xeroapi.InvoiceLineItem{}, Payments: []xeroapi.InvoicePayment{}, CreditNotes: []xeroapi.InvoiceAllocation{}, Prepayments: []xeroapi.InvoiceAllocation{}, Overpayments: []xeroapi.InvoiceAllocation{}}}}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--page-size", "100", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices with default page and page size: %v", err)
	}
	if lister.request.Page != 1 || lister.request.PageSize != 100 {
		t.Fatalf("unexpected paging: page=%d pageSize=%d", lister.request.Page, lister.request.PageSize)
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

func TestInvoicesCommandRejectsTypeWhere(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--where", `Type=="ACCPAY"`, "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if lister.request.Type != "" {
		t.Fatalf("expected client not to be called, got request %+v", lister.request)
	}
}

func TestBillsCommandDoesNotExposeInvoiceActions(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "bills", "pdf"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "pdf"`) && !strings.Contains(err.Error(), "accepts 0 arg(s)") {
		t.Fatalf("expected bills action rejection, got %v", err)
	}
	if lister.request.Type != "" || lister.pdfRequest.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got list=%+v pdf=%+v", lister.request, lister.pdfRequest)
	}
}

func TestInvoicesCommandRejectsRemovedContactFlag(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "--contact", "Acme", "--json"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --contact") {
		t.Fatalf("expected unknown flag error for removed --contact, got %v", err)
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

func TestInvoicesOnlineURLCommandEmitsStableJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{onlineInvoice: xeroapi.OnlineInvoiceResult{InvoiceID: "220ddca8-3144-4085-9a88-2d72c5133734", OnlineInvoiceURL: "https://in.xero.com/abc", Available: true}}
	deps, stdout, stderr := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "online-url", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices online-url: %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"onlineInvoiceUrl": "https://in.xero.com/abc"`) || !strings.Contains(stdout.String(), `"available": true`) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if lister.onlineRequest.TenantID != "tenant-1" {
		t.Fatalf("expected default tenant to be used, got %q", lister.onlineRequest.TenantID)
	}
	if lister.onlineRequest.InvoiceID != "220ddca8-3144-4085-9a88-2d72c5133734" {
		t.Fatalf("expected invoice ID to be normalized, got %q", lister.onlineRequest.InvoiceID)
	}
}

func TestInvoicesOnlineURLCommandPrintsMissingMessage(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{onlineInvoice: xeroapi.OnlineInvoiceResult{InvoiceID: "220ddca8-3144-4085-9a88-2d72c5133734", Available: false}}
	deps, stdout, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "online-url", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices online-url: %v", err)
	}
	if got := stdout.String(); got != "No online invoice URL available for invoice 220ddca8-3144-4085-9a88-2d72c5133734\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestInvoicesOnlineURLCommandRejectsInvalidInvoiceID(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "online-url", "--invoice-id", "not-a-uuid", "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if lister.onlineRequest.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got request %+v", lister.onlineRequest)
	}
}

func TestInvoicesOnlineURLCommandRequiresInvoiceIDFlag(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "online-url", "--json"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "invoice-id" not set`) {
		t.Fatalf("expected required-flag error, got %v", err)
	}
	if lister.onlineRequest.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got request %+v", lister.onlineRequest)
	}
}

func TestInvoicesPDFCommandEmitsStableJSONForFileOutput(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{pdfContent: []byte("%PDF-1.7\nhello\n")}
	deps, stdout, stderr := testDependencies(configPath, store, lister, false)

	outputPath := filepath.Join(tempDir, "invoice.pdf")
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", outputPath, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices pdf: %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"savedTo": `+"\""+outputPath+"\"") || !strings.Contains(stdout.String(), `"contentType": "application/pdf"`) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if lister.pdfRequest.TenantID != "tenant-1" {
		t.Fatalf("expected default tenant to be used, got %q", lister.pdfRequest.TenantID)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(content) != "%PDF-1.7\nhello\n" {
		t.Fatalf("unexpected output content: %q", string(content))
	}
	if _, err := os.Stat(outputPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected temp file cleanup, got err=%v", err)
	}
}

func TestInvoicesPDFCommandPrintsSavedMessage(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{pdfContent: []byte("%PDF")}
	deps, stdout, _ := testDependencies(configPath, store, lister, false)

	outputPath := filepath.Join(tempDir, "invoice.pdf")
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", outputPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices pdf: %v", err)
	}
	if got := stdout.String(); got != "Saved invoice PDF to "+outputPath+" (4 bytes)\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestInvoicesPDFCommandStreamsToStdout(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{pdfContent: []byte("%PDF-raw")}
	deps, stdout, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices pdf: %v", err)
	}
	if got := stdout.String(); got != "%PDF-raw" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestInvoicesPDFCommandRejectsJSONWithStdoutOutput(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "--json", "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", "-"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if lister.pdfRequest.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got request %+v", lister.pdfRequest)
	}
}

func TestInvoicesPDFCommandRejectsInteractiveStdoutOutput(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, true)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", "-"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if lister.pdfRequest.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got request %+v", lister.pdfRequest)
	}
}

func TestInvoicesPDFCommandQuotesBreadcrumbOutputPath(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{pdfContent: []byte("%PDF")}
	deps, stdout, _ := testDependencies(configPath, store, lister, false)

	outputPath := filepath.Join(tempDir, "my invoice.pdf")
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", outputPath, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices pdf: %v", err)
	}

	var envelope output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if len(envelope.Breadcrumbs) != 1 {
		t.Fatalf("expected one breadcrumb, got %+v", envelope.Breadcrumbs)
	}
	expected := `xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output ` + `"` + outputPath + `" --json`
	if envelope.Breadcrumbs[0].Cmd != expected {
		t.Fatalf("unexpected breadcrumb: %q", envelope.Breadcrumbs[0].Cmd)
	}
}

func TestInvoicesPDFCommandClassifiesOutputPathErrorsAsInternal(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	blocker := filepath.Join(tempDir, "blocked")
	if err := os.WriteFile(blocker, []byte("nope"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{pdfContent: []byte("%PDF")}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "pdf", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--output", filepath.Join(blocker, "invoice.pdf")})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindInternal {
		t.Fatalf("expected internal error, got %v", err)
	}
	if lister.pdfRequest.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got %+v", lister.pdfRequest)
	}
}

func TestInvoicesApproveCommandEmitsStableJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{approveResult: xeroapi.InvoiceApprovalResult{InvoiceID: "220ddca8-3144-4085-9a88-2d72c5133734", TenantID: "tenant-1", InvoiceNumber: "INV-0001", Type: "ACCREC", Status: "AUTHORISED", UpdatedAt: "2026-03-11T12:30:00Z", StatusObserved: true}}
	deps, stdout, stderr := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "approve", "--invoice-id", "220DDCA8-3144-4085-9A88-2D72C5133734", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices approve: %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"summary": "invoice approved"`) || !strings.Contains(stdout.String(), `"invoiceNumber": "INV-0001"`) || !strings.Contains(stdout.String(), `"tenantId": "tenant-1"`) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if lister.approveReq.TenantID != "tenant-1" {
		t.Fatalf("expected default tenant to be used, got %q", lister.approveReq.TenantID)
	}
	if lister.approveReq.InvoiceID != "220ddca8-3144-4085-9a88-2d72c5133734" {
		t.Fatalf("expected normalized invoice ID, got %q", lister.approveReq.InvoiceID)
	}
}

func TestInvoicesApproveCommandPrintsSuccessLine(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{approveResult: xeroapi.InvoiceApprovalResult{InvoiceID: "220ddca8-3144-4085-9a88-2d72c5133734", TenantID: "tenant-1", InvoiceNumber: "INV-0001", Status: "AUTHORISED", StatusObserved: true}}
	deps, stdout, stderr := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "approve", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices approve: %v", err)
	}
	if got := stdout.String(); got != "Approved invoice INV-0001 (220ddca8-3144-4085-9a88-2d72c5133734) for tenant tenant-1 (AUTHORISED)\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestInvoicesApproveCommandRejectsInvalidInvoiceID(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "approve", "--invoice-id", "not-a-uuid", "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if lister.approveReq.InvoiceID != "" {
		t.Fatalf("expected client not to be called, got request %+v", lister.approveReq)
	}
}

func TestInvoicesApproveCommandRequiresInvoiceIDFlag(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "approve", "--json"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "invoice-id" not set`) {
		t.Fatalf("expected required-flag error, got %v", err)
	}
}

func TestInvoicesApproveCommandUsesTokenTenant(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	sessionPath := filepath.Join(tempDir, "session.json")
	store := auth.NewSessionStore(sessionPath)
	if err := store.Save(auth.SessionMetadata{Authenticated: true, AuthMode: "browser_oauth", KnownTenants: []auth.Tenant{{ID: "tenant-1", Name: "Acme"}, {ID: "tenant-2", Name: "Other"}}}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	tokenStore := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth", TenantID: "tenant-2", TenantName: "Other"}}
	lister := &fakeLister{approveResult: xeroapi.InvoiceApprovalResult{InvoiceID: "220ddca8-3144-4085-9a88-2d72c5133734", TenantID: "tenant-2", Status: "AUTHORISED", StatusObserved: true}}
	deps, _, _ := testDependencies(configPath, tokenStore, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "approve", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute invoices approve with token tenant: %v", err)
	}
	if lister.approveReq.TenantID != "tenant-2" {
		t.Fatalf("expected token tenant to be used, got %q", lister.approveReq.TenantID)
	}
}

func TestInvoicesApproveCommandPropagatesTypedUpstreamError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))

	store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
	lister := &fakeLister{err: clierrors.New(clierrors.KindXeroAPI, "invoice cannot be authorised")}
	deps, _, _ := testDependencies(configPath, store, lister, false)

	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "approve", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--json"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindXeroAPI {
		t.Fatalf("expected Xero API error, got %v", err)
	}
}

func TestInvoiceAndBillCreateCommandsUseNamespaceOwnedTypes(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		wantType      string
		wantResource  string
		inputExtra    string
		idempotency   string
		wantGenerated bool
	}{
		{name: "sales invoice", namespace: "invoices", wantType: "ACCREC", wantResource: "invoice", inputExtra: `,"sentToContact":false`, idempotency: "create-key"},
		{name: "purchase bill", namespace: "bills", wantType: "ACCPAY", wantResource: "bill", inputExtra: `,"plannedPaymentDate":"2026-07-31"`, wantGenerated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.json")
			prepareConfig(t, configPath)
			prepareSession(t, filepath.Join(tempDir, "session.json"))
			inputPath := filepath.Join(tempDir, "create.json")
			body := `{"contactId":"220ddca8-3144-4085-9a88-2d72c5133734","reference":"PO-1","lineItems":[{"description":"Service","quantity":1.234567890123456789,"unitAmount":0}]` + tt.inputExtra + `}`
			if err := os.WriteFile(inputPath, []byte(body), 0o600); err != nil {
				t.Fatalf("write input: %v", err)
			}

			store := &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC(), AuthMode: "browser_oauth"}}
			client := &fakeLister{mutationResult: xeroapi.InvoiceMutationResult{InvoiceID: "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0", InvoiceNumber: "DOC-1", Status: "DRAFT"}}
			deps, stdout, stderr := testDependencies(configPath, store, client, false)
			args := []string{"--config", configPath, tt.namespace, "create", "--file", inputPath, "--json"}
			if tt.idempotency != "" {
				args = append(args, "--idempotency-key", tt.idempotency)
			}
			cmd := commands.NewRootCommand(deps)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute create: %v", err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
			if client.createReq.Invoice.Type != tt.wantType || client.createReq.Resource != tt.wantResource || client.createReq.Namespace != tt.namespace {
				t.Fatalf("unexpected namespace mapping: %+v", client.createReq)
			}
			if client.createReq.Invoice.Status == nil || *client.createReq.Invoice.Status != "DRAFT" {
				t.Fatalf("expected explicit DRAFT, got %+v", client.createReq.Invoice.Status)
			}
			if client.createReq.Invoice.LineItems == nil || len(*client.createReq.Invoice.LineItems) != 1 || (*client.createReq.Invoice.LineItems)[0].Quantity.String() != "1.234567890123456789" {
				t.Fatalf("expected lossless line-item input, got %+v", client.createReq.Invoice.LineItems)
			}
			if tt.wantGenerated {
				if len(client.createReq.IdempotencyKey) != 64 {
					t.Fatalf("expected generated key, got %q", client.createReq.IdempotencyKey)
				}
			} else if client.createReq.IdempotencyKey != tt.idempotency {
				t.Fatalf("unexpected idempotency key: %q", client.createReq.IdempotencyKey)
			}
			if !strings.Contains(stdout.String(), `"operation": "created"`) || !strings.Contains(stdout.String(), `"cmd": "xero `+tt.namespace+` --invoice-id 88192a99-cbc5-4a66-bf1a-2f9fea2d36d0 --json"`) {
				t.Fatalf("unexpected output: %s", stdout.String())
			}
		})
	}
}

func TestInvoiceUpdateLineItemGateRunsBeforeRuntime(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "missing-config.json")
	inputPath := filepath.Join(tempDir, "update.json")
	if err := os.WriteFile(inputPath, []byte(`{"lineItems":[{"description":"Service"}]}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	client := &fakeLister{}
	deps, _, _ := testDependencies(configPath, &fakeStore{}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "update", "--invoice-id", "220ddca8-3144-4085-9a88-2d72c5133734", "--file", inputPath})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), "--replace-line-items") {
		t.Fatalf("expected replacement confirmation error, got %v", err)
	}
	if client.getRequest.InvoiceID != "" || client.updateReq.InvoiceID != "" {
		t.Fatalf("expected no client calls, get=%+v update=%+v", client.getRequest, client.updateReq)
	}
}

func TestBillUpdateRejectsWrongTypeBeforeMutation(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))
	inputPath := filepath.Join(tempDir, "update.json")
	if err := os.WriteFile(inputPath, []byte(`{"reference":"new"}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	invoiceID := "220ddca8-3144-4085-9a88-2d72c5133734"
	client := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: invoiceID, Type: "ACCREC"}}}
	deps, _, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "bills", "update", "--invoice-id", invoiceID, "--file", inputPath})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), "ACCPAY") {
		t.Fatalf("expected wrong-Type error, got %v", err)
	}
	if client.getRequest.InvoiceID != invoiceID || client.updateReq.InvoiceID != "" {
		t.Fatalf("expected one preflight and zero mutations: get=%+v update=%+v", client.getRequest, client.updateReq)
	}
}

func TestInvoiceUpdatePreservesAbsentFieldsAndReportsRemovedLines(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))
	keptID := "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"
	removedID := "c7c1f7ca-3793-4f66-a4e2-77858365bcfa"
	inputPath := filepath.Join(tempDir, "update.json")
	if err := os.WriteFile(inputPath, []byte(`{"sentToContact":false,"lineItems":[{"lineItemId":"`+keptID+`","description":"Kept","quantity":0}]}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	invoiceID := "220ddca8-3144-4085-9a88-2d72c5133734"
	client := &fakeLister{
		invoices:       []xeroapi.Invoice{{InvoiceID: invoiceID, Type: "ACCREC", LineItems: []xeroapi.InvoiceLineItem{{LineItemID: keptID}, {LineItemID: removedID}}}},
		mutationResult: xeroapi.InvoiceMutationResult{Operation: "updated", Resource: "invoice", InvoiceID: invoiceID, TenantID: "tenant-1", Type: "ACCREC", Status: "DRAFT", IdempotencyKey: "update-key"},
	}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "update", "--invoice-id", strings.ToUpper(invoiceID), "--file", inputPath, "--replace-line-items", "--idempotency-key", "update-key", "--quiet"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute update: %v", err)
	}
	if client.updateReq.InvoiceID != invoiceID || client.updateReq.Invoice.Type != "ACCREC" || client.updateReq.Invoice.InvoiceID != invoiceID {
		t.Fatalf("expected normalized injected identity, got %+v", client.updateReq)
	}
	if client.updateReq.Invoice.Contact != nil || client.updateReq.Invoice.Reference != nil || client.updateReq.Invoice.Status != nil {
		t.Fatalf("expected absent scalars omitted, got %+v", client.updateReq.Invoice)
	}
	if client.updateReq.Invoice.SentToContact == nil || *client.updateReq.Invoice.SentToContact {
		t.Fatalf("expected explicit false to remain present")
	}
	if !strings.Contains(stdout.String(), `"lineItemsReplaced": true`) || !strings.Contains(stdout.String(), `"removedLineItemCount": 1`) || strings.Contains(stdout.String(), `"ok"`) {
		t.Fatalf("unexpected quiet result: %s", stdout.String())
	}
}

func TestInvoiceCreateHumanOutput(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))
	inputPath := filepath.Join(tempDir, "create.json")
	if err := os.WriteFile(inputPath, []byte(`{"contactId":"220ddca8-3144-4085-9a88-2d72c5133734","lineItems":[{"description":"Service"}]}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	client := &fakeLister{mutationResult: xeroapi.InvoiceMutationResult{Operation: "created", Resource: "invoice", InvoiceID: "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0", TenantID: "tenant-1", InvoiceNumber: "INV-1", Type: "ACCREC", Status: "DRAFT", IdempotencyKey: "key"}}
	deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "invoices", "create", "--file", inputPath, "--idempotency-key", "key"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute create: %v", err)
	}
	if got, want := stdout.String(), "Created invoice INV-1 (88192a99-cbc5-4a66-bf1a-2f9fea2d36d0) for tenant tenant-1 (DRAFT)\nIdempotency key: key\n"; got != want {
		t.Fatalf("unexpected human output: got %q want %q", got, want)
	}
}

func testDependencies(configPath string, store auth.TokenStore, lister xeroapi.InvoiceClient, interactive bool) (commands.Dependencies, *bytes.Buffer, *bytes.Buffer) {
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
		NewInvoiceClient: func(appconfig.Settings) xeroapi.InvoiceClient {
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
	if err := manager.AddProfile("acme", "client-acme", false); err != nil {
		t.Fatalf("add profile: %v", err)
	}
}

func prepareSession(t *testing.T, sessionPath string) {
	t.Helper()
	store := auth.NewSessionStore(sessionPath)
	if err := store.Save(auth.SessionMetadata{Authenticated: true, AuthMode: "browser_oauth", KnownTenants: []auth.Tenant{{ID: "tenant-1", Name: "Acme"}}}); err != nil {
		t.Fatalf("save session: %v", err)
	}
}
