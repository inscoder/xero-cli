package xeroapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inscoder/xero-cli/internal/auth"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

const mutationInvoiceID = "220ddca8-3144-4085-9a88-2d72c5133734"

func TestCreateInvoiceBuildsExactSingleItemRequest(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPut || r.URL.Path != "/api.xro/2.0/Invoices" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		if got := r.Header.Get("Xero-tenant-id"); got != "tenant-1" {
			t.Fatalf("unexpected tenant: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected accept: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "create-key" {
			t.Fatalf("unexpected idempotency key: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		want := `{"Invoices":[{"Type":"ACCREC","Contact":{"ContactID":"contact-1"},"Reference":"PO-1","CurrencyRate":1.234567890123456789,"Status":"DRAFT","LineItems":[{"Description":"Service","Quantity":0,"UnitAmount":99.990000000000000001,"Tracking":[{"Name":"Region","Option":"APAC"}]}]}]}`
		if string(body) != want {
			t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
		}
		_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"`+mutationInvoiceID+`","Type":"ACCREC","InvoiceNumber":"INV-1","Status":"DRAFT","UpdatedDateUTC":"2026-07-10T10:00:00Z"}]}`)
	}))
	defer server.Close()

	reference := "PO-1"
	status := "DRAFT"
	currencyRate := json.Number("1.234567890123456789")
	quantity := json.Number("0")
	unitAmount := json.Number("99.990000000000000001")
	trackingName := "Region"
	trackingOption := "APAC"
	lineItems := []xeroapi.InvoiceWriteLineItem{{
		Description: "Service",
		Quantity:    &quantity,
		UnitAmount:  &unitAmount,
		Tracking:    &[]xeroapi.InvoiceWriteTracking{{Name: &trackingName, Option: &trackingOption}},
	}}
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	result, err := client.CreateInvoice(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.CreateInvoiceRequest{
		TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", IdempotencyKey: "create-key",
		Invoice: xeroapi.InvoiceWrite{Type: "ACCREC", Contact: &xeroapi.InvoiceWriteContact{ContactID: "contact-1"}, Reference: &reference, CurrencyRate: &currencyRate, Status: &status, LineItems: &lineItems},
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected exactly one request, got %d", requestCount)
	}
	if result.Operation != "created" || result.Resource != "invoice" || result.InvoiceID != mutationInvoiceID || result.TenantID != "tenant-1" || result.Type != "ACCREC" || result.Status != "DRAFT" || result.IdempotencyKey != "create-key" || result.UpdatedAt != "2026-07-10T10:00:00Z" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUpdateInvoiceBuildsExactPartialRequestWithoutAbsentFields(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPost || r.URL.Path != "/api.xro/2.0/Invoices/"+mutationInvoiceID {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		want := `{"Invoices":[{"InvoiceID":"` + mutationInvoiceID + `","Type":"ACCREC","Reference":"","SentToContact":false}]}`
		if string(body) != want {
			t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
		}
		_, _ = io.WriteString(w, `{"Invoices":[{"InvoiceID":"`+mutationInvoiceID+`","Type":"ACCREC","Status":"AUTHORISED"}]}`)
	}))
	defer server.Close()

	reference := ""
	sent := false
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	result, err := client.UpdateInvoice(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.UpdateInvoiceRequest{
		TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", InvoiceID: mutationInvoiceID, IdempotencyKey: "update-key",
		Invoice: xeroapi.InvoiceWrite{InvoiceID: mutationInvoiceID, Type: "ACCREC", Reference: &reference, SentToContact: &sent},
	})
	if err != nil {
		t.Fatalf("update invoice: %v", err)
	}
	if requestCount != 1 || result.Operation != "updated" || result.InvoiceID != mutationInvoiceID || result.IdempotencyKey != "update-key" {
		t.Fatalf("unexpected request count/result: count=%d result=%+v", requestCount, result)
	}
}

func TestInvoiceMutationsClassifyUnusableOutcomesAsUncertainWithoutRetry(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
	}{
		{name: "server error", code: http.StatusServiceUnavailable, body: `{"Message":"unavailable"}`},
		{name: "malformed", code: http.StatusOK, body: `{"Invoices":[`},
		{name: "trailing", code: http.StatusOK, body: `{"Invoices":[{"InvoiceID":"` + mutationInvoiceID + `","Type":"ACCREC"}]} {}`},
		{name: "empty", code: http.StatusOK, body: `{"Invoices":[]}`},
		{name: "multiple", code: http.StatusOK, body: `{"Invoices":[{"InvoiceID":"` + mutationInvoiceID + `","Type":"ACCREC"},{"InvoiceID":"other","Type":"ACCREC"}]}`},
		{name: "missing ID", code: http.StatusOK, body: `{"Invoices":[{"Type":"ACCREC"}]}`},
		{name: "wrong type", code: http.StatusOK, body: `{"Invoices":[{"InvoiceID":"` + mutationInvoiceID + `","Type":"ACCPAY"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.WriteHeader(tt.code)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			invoiceNumber := "INV-RECOVER"
			client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
			_, err := client.CreateInvoice(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.CreateInvoiceRequest{
				TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", IdempotencyKey: "same-key",
				Invoice: xeroapi.InvoiceWrite{Type: "ACCREC", InvoiceNumber: &invoiceNumber},
			})
			if clierrors.KindOf(err) != clierrors.KindMutationUncertain {
				t.Fatalf("expected uncertain error, got %v", err)
			}
			metadata := clierrors.MetadataOf(err)
			hasRecoveryIdentity := strings.Contains(metadata.RecoveryCommand, "INV-RECOVER") || metadata.InvoiceID != "" && strings.Contains(metadata.RecoveryCommand, metadata.InvoiceID)
			if !metadata.MayHaveSucceeded || metadata.Operation != "created" || metadata.Resource != "invoice" || metadata.TenantID != "tenant-1" || metadata.IdempotencyKey != "same-key" || !hasRecoveryIdentity {
				t.Fatalf("unexpected metadata: %+v", metadata)
			}
			if requestCount != 1 {
				t.Fatalf("expected no retry, got %d requests", requestCount)
			}
		})
	}
}

func TestInvoiceMutationTransportErrorIsUncertain(t *testing.T) {
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: errorTransport{err: errors.New("connection reset")}}})
	_, err := client.UpdateInvoice(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UpdateInvoiceRequest{
		TenantID: "tenant-1", Resource: "bill", Namespace: "bills", InvoiceID: mutationInvoiceID, IdempotencyKey: "update-key",
		Invoice: xeroapi.InvoiceWrite{InvoiceID: mutationInvoiceID, Type: "ACCPAY"},
	})
	metadata := clierrors.MetadataOf(err)
	if clierrors.KindOf(err) != clierrors.KindMutationUncertain || !metadata.MayHaveSucceeded || metadata.InvoiceID != mutationInvoiceID || metadata.RecoveryCommand != "xero bills --invoice-id "+mutationInvoiceID+" --json" {
		t.Fatalf("unexpected error/metadata: %v %+v", err, metadata)
	}
}

func TestInvoiceMutationValidationFailuresAreKnownXeroErrors(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
	}{
		{name: "http validation", code: http.StatusBadRequest, body: `{"Message":"validation","Elements":[{"ValidationErrors":[{"Message":"bad account"}]}]}`},
		{name: "semantic validation", code: http.StatusOK, body: `{"Invoices":[{"InvoiceID":"` + mutationInvoiceID + `","Type":"ACCREC","HasErrors":true,"ValidationErrors":[{"Message":"locked period"}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.WriteHeader(tt.code)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
			_, err := client.CreateInvoice(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.CreateInvoiceRequest{
				TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", IdempotencyKey: "key", Invoice: xeroapi.InvoiceWrite{Type: "ACCREC"},
			})
			if clierrors.KindOf(err) != clierrors.KindXeroAPI || clierrors.MetadataOf(err).MayHaveSucceeded {
				t.Fatalf("expected known Xero API error, got %v metadata=%+v", err, clierrors.MetadataOf(err))
			}
			if requestCount != 1 {
				t.Fatalf("expected exactly one request, got %d", requestCount)
			}
		})
	}
}

type errorTransport struct {
	err error
}

func (transport errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, transport.err
}
