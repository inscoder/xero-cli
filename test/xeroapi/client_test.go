package xeroapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cesar/xero-cli/internal/auth"
	appconfig "github.com/cesar/xero-cli/internal/config"
	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/cesar/xero-cli/internal/xeroapi"
)

func TestListInvoicesBuildsRequestAndNormalizesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.xro/2.0/Invoices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("Statuses"); got != "AUTHORISED" {
			t.Fatalf("unexpected status query: %q", got)
		}
		if got := r.URL.Query().Get("searchTerm"); got != "Acme" {
			t.Fatalf("unexpected contact query: %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("unexpected page query: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get("Xero-tenant-id"); got != "tenant-1" {
			t.Fatalf("unexpected tenant header: %q", got)
		}
		if got := r.Header.Get("If-Modified-Since"); got == "" {
			t.Fatal("expected If-Modified-Since header")
		}
		_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"1","InvoiceNumber":"INV-0001","Contact":{"Name":"Acme Ltd"},"Status":"AUTHORISED","Total":123.45,"AmountDue":23.45,"CurrencyCode":"USD","DueDate":"2026-03-10T00:00:00","UpdatedDateUTC":"/Date(1773059400000+0000)/"},{"InvoiceID":"2","InvoiceNumber":"INV-0002"}]}`)
	}))
	defer server.Close()

	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	invoices, err := client.ListInvoices(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.ListInvoicesRequest{TenantID: "tenant-1", Status: "AUTHORISED", Contact: "Acme", Since: "2026-03-01", Page: 2, Limit: 1})
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	if len(invoices) != 1 {
		t.Fatalf("expected limit to trim results, got %d", len(invoices))
	}
	if invoices[0].DueDate != "2026-03-10" {
		t.Fatalf("unexpected due date normalization: %q", invoices[0].DueDate)
	}
	if invoices[0].UpdatedAt != time.UnixMilli(1773059400000).UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected updatedAt normalization: %q", invoices[0].UpdatedAt)
	}
}

func TestListInvoicesMapsRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"Message":"slow down"}`)
	}))
	defer server.Close()

	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.ListInvoices(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.ListInvoicesRequest{TenantID: "tenant-1"})
	if clierrors.KindOf(err) != clierrors.KindRateLimit {
		t.Fatalf("expected rate limit error, got %v", err)
	}
}
