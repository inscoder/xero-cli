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

func TestListInvoicesBuildsAdvancedRequestAndNormalizesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.xro/2.0/Invoices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("IDs"); got != "220ddca8-3144-4085-9a88-2d72c5133734,88192a99-cbc5-4a66-bf1a-2f9fea2d36d0" {
			t.Fatalf("unexpected IDs query: %q", got)
		}
		if got := r.URL.Query().Get("Statuses"); got != "AUTHORISED,PAID" {
			t.Fatalf("unexpected statuses query: %q", got)
		}
		if got := r.URL.Query().Get("SearchTerm"); got != "Acme" {
			t.Fatalf("unexpected contact query: %q", got)
		}
		if got := r.URL.Query().Get("where"); got != `Type=="ACCPAY" AND AmountDue>=5000` {
			t.Fatalf("unexpected where query: %q", got)
		}
		if got := r.URL.Query().Get("order"); got != "UpdatedDateUTC DESC" {
			t.Fatalf("unexpected order query: %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("unexpected page query: %q", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize query: %q", got)
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
		_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"1","Type":"ACCREC","InvoiceNumber":"INV-0001","Reference":"PO-123","Contact":{"ContactID":"contact-1","Name":"Acme Ltd","ContactNumber":"C-001"},"Date":"2026-03-05T00:00:00","DueDate":"2026-03-10T00:00:00","Status":"AUTHORISED","LineAmountTypes":"Exclusive","LineItems":[{"Description":"Consulting","Quantity":2,"UnitAmount":61.725,"ItemCode":"SERV","AccountCode":"200","AccountID":"acc-1","TaxType":"OUTPUT2","TaxAmount":12.34,"LineAmount":123.45,"LineItemID":"line-1","Tracking":[{"Name":"Region","Option":"APAC"}]}],"SubTotal":123.45,"TotalTax":12.34,"Total":135.79,"TotalDiscount":0,"AmountDue":23.45,"AmountPaid":112.34,"AmountCredited":0,"CurrencyCode":"USD","CurrencyRate":1,"UpdatedDateUTC":"/Date(1773059400000+0000)/","BrandingThemeID":"brand-1","Url":"https://example.com/invoice/1","SentToContact":true,"ExpectedPaymentDate":"2026-03-11T00:00:00","PlannedPaymentDate":"2026-03-12T00:00:00","HasAttachments":true,"Payments":[{"PaymentID":"payment-1","Date":"2026-03-09T00:00:00","Amount":112.34,"Reference":"PAY-1","CurrencyRate":1,"PaymentType":"ACCRECPAYMENT","Status":"AUTHORISED"}],"CreditNotes":[{"CreditNoteID":"credit-1","Type":"ACCRECCREDIT","Date":"2026-03-08T00:00:00","AppliedAmount":1.23,"Status":"APPLIED"}],"Prepayments":[{"PrepaymentID":"prepay-1","Type":"PREPAYMENT","Date":"2026-03-07T00:00:00","AppliedAmount":2.34,"Status":"AUTHORISED"}],"Overpayments":[{"OverpaymentID":"overpay-1","Type":"OVERPAYMENT","Date":"2026-03-06T00:00:00","AppliedAmount":3.45,"Status":"AUTHORISED"}]}]}`)
	}))
	defer server.Close()

	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	invoices, err := client.ListInvoices(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.ListInvoicesRequest{
		TenantID:   "tenant-1",
		InvoiceIDs: []string{"220ddca8-3144-4085-9a88-2d72c5133734", "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"},
		Statuses:   []string{"AUTHORISED", "PAID"},
		Contact:    "Acme",
		Since:      "2026-03-01",
		Where:      `Type=="ACCPAY" AND AmountDue>=5000`,
		Order:      "UpdatedDateUTC DESC",
		Page:       2,
		PageSize:   50,
	})
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	if len(invoices) != 1 {
		t.Fatalf("expected one invoice, got %d", len(invoices))
	}
	invoice := invoices[0]
	if invoice.InvoiceNumber != "INV-0001" {
		t.Fatalf("unexpected invoice number: %q", invoice.InvoiceNumber)
	}
	if invoice.Contact.Name != "Acme Ltd" || invoice.ContactName != "Acme Ltd" {
		t.Fatalf("unexpected contact normalization: %+v", invoice.Contact)
	}
	if invoice.Date != "2026-03-05" || invoice.DueDate != "2026-03-10" {
		t.Fatalf("unexpected date normalization: date=%q due=%q", invoice.Date, invoice.DueDate)
	}
	if invoice.UpdatedAt != time.UnixMilli(1773059400000).UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected updatedAt normalization: %q", invoice.UpdatedAt)
	}
	if invoice.CurrencyCode != "USD" || invoice.Currency != "USD" {
		t.Fatalf("unexpected currency normalization: code=%q alias=%q", invoice.CurrencyCode, invoice.Currency)
	}
	if len(invoice.LineItems) != 1 || len(invoice.LineItems[0].Tracking) != 1 {
		t.Fatalf("expected line items and tracking to be preserved: %+v", invoice.LineItems)
	}
	if len(invoice.Payments) != 1 || invoice.Payments[0].Date != "2026-03-09" {
		t.Fatalf("expected payment normalization, got %+v", invoice.Payments)
	}
	if len(invoice.CreditNotes) != 1 || invoice.CreditNotes[0].AllocationID != "credit-1" {
		t.Fatalf("expected credit note normalization, got %+v", invoice.CreditNotes)
	}
	if len(invoice.Prepayments) != 1 || invoice.Prepayments[0].AllocationID != "prepay-1" {
		t.Fatalf("expected prepayment normalization, got %+v", invoice.Prepayments)
	}
	if len(invoice.Overpayments) != 1 || invoice.Overpayments[0].AllocationID != "overpay-1" {
		t.Fatalf("expected overpayment normalization, got %+v", invoice.Overpayments)
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
