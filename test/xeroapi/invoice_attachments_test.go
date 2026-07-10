package xeroapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/inscoder/xero-cli/internal/auth"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

func TestUploadInvoiceAttachmentBuildsExactCreateRequest(t *testing.T) {
	fileName := "résumé #1%[final].pdf"
	content := []byte("%PDF-1.7\nattachment\n")
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		wantPath := "/api.xro/2.0/Invoices/" + mutationInvoiceID + "/Attachments/" + url.PathEscape(fileName)
		if r.URL.EscapedPath() != wantPath {
			t.Fatalf("unexpected escaped path: got %q want %q", r.URL.EscapedPath(), wantPath)
		}
		if r.URL.Query().Get("IncludeOnline") != "true" {
			t.Fatalf("expected IncludeOnline query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("Xero-tenant-id") != "tenant-1" || r.Header.Get("Idempotency-Key") != "attachment-key" {
			t.Fatalf("unexpected auth headers: %+v", r.Header)
		}
		if r.Header.Get("Content-Type") != "application/pdf" || r.Header.Get("Accept") != "application/json" || r.ContentLength != int64(len(content)) {
			t.Fatalf("unexpected content headers: type=%q accept=%q length=%d", r.Header.Get("Content-Type"), r.Header.Get("Accept"), r.ContentLength)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !bytes.Equal(body, content) {
			t.Fatalf("unexpected body: %q", body)
		}
		_, _ = io.WriteString(w, `{"Attachments":[{"AttachmentID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0","FileName":"résumé #1%[final].pdf","MimeType":"application/pdf","ContentLength":20,"IncludeOnline":true}]}`)
	}))
	defer server.Close()

	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	result, err := client.UploadInvoiceAttachment(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UploadInvoiceAttachmentRequest{
		TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", Type: "ACCREC", InvoiceID: mutationInvoiceID,
		FileName: fileName, ContentType: "application/pdf", ContentLength: int64(len(content)), IdempotencyKey: "attachment-key",
		IncludeOnline: true, Body: bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("upload attachment: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected exactly one request, got %d", requestCount)
	}
	if result.Operation != "uploaded" || result.Resource != "invoice" || result.InvoiceID != mutationInvoiceID || result.AttachmentID == "" || result.FileName != fileName || result.Bytes != int64(len(content)) || result.IncludeOnline == nil || !*result.IncludeOnline || result.Overwritten || result.IdempotencyKey != "attachment-key" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUploadInvoiceAttachmentUsesPostForReplacement(t *testing.T) {
	content := []byte("replacement")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("replacement must not send IncludeOnline: %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"Attachments":[{"AttachmentID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0","FileName":"receipt.pdf","ContentLength":11}]}`)
	}))
	defer server.Close()

	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	result, err := client.UploadInvoiceAttachment(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UploadInvoiceAttachmentRequest{
		TenantID: "tenant-1", Resource: "bill", Namespace: "bills", Type: "ACCPAY", InvoiceID: mutationInvoiceID,
		FileName: "receipt.pdf", ContentType: "application/pdf", ContentLength: int64(len(content)), IdempotencyKey: "replace-key",
		Replace: true, Body: bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("replace attachment: %v", err)
	}
	if result.Operation != "replaced" || !result.Overwritten || result.IncludeOnline != nil || result.Type != "ACCPAY" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAttachmentMutationUnusableResponsesAreUncertainWithoutRetry(t *testing.T) {
	content := []byte("abc")
	tests := []struct {
		name string
		code int
		body string
	}{
		{name: "server error", code: http.StatusServiceUnavailable, body: `{"Message":"offline"}`},
		{name: "malformed", code: http.StatusOK, body: `{"Attachments":[`},
		{name: "empty", code: http.StatusOK, body: `{"Attachments":[]}`},
		{name: "multiple", code: http.StatusOK, body: `{"Attachments":[{"AttachmentID":"a"},{"AttachmentID":"b"}]}`},
		{name: "missing ID", code: http.StatusOK, body: `{"Attachments":[{"FileName":"receipt.pdf","ContentLength":3}]}`},
		{name: "wrong filename", code: http.StatusOK, body: `{"Attachments":[{"AttachmentID":"a","FileName":"other.pdf","ContentLength":3}]}`},
		{name: "wrong length", code: http.StatusOK, body: `{"Attachments":[{"AttachmentID":"a","FileName":"receipt.pdf","ContentLength":2}]}`},
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
			_, err := client.UploadInvoiceAttachment(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UploadInvoiceAttachmentRequest{
				TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", Type: "ACCREC", InvoiceID: mutationInvoiceID,
				FileName: "receipt.pdf", ContentType: "application/pdf", ContentLength: 3, IdempotencyKey: "same-key", Body: bytes.NewReader(content),
			})
			metadata := clierrors.MetadataOf(err)
			if clierrors.KindOf(err) != clierrors.KindMutationUncertain || !metadata.MayHaveSucceeded || metadata.FileName != "receipt.pdf" || metadata.IdempotencyKey != "same-key" || metadata.RecoveryCommand != "xero invoices --invoice-id "+mutationInvoiceID+" --json" {
				t.Fatalf("unexpected error/metadata: %v %+v", err, metadata)
			}
			if requestCount != 1 {
				t.Fatalf("expected no retry, got %d requests", requestCount)
			}
		})
	}
}

func TestAttachmentMutationValidationFailureIsKnown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"Message":"validation","Attachments":[{"ValidationErrors":[{"Message":"filename rejected"}]}]}`)
	}))
	defer server.Close()
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.UploadInvoiceAttachment(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UploadInvoiceAttachmentRequest{
		TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", Type: "ACCREC", InvoiceID: mutationInvoiceID,
		FileName: "receipt.pdf", ContentType: "application/pdf", ContentLength: 3, IdempotencyKey: "key", Body: strings.NewReader("abc"),
	})
	if clierrors.KindOf(err) != clierrors.KindXeroAPI || !strings.Contains(err.Error(), "filename rejected") || clierrors.MetadataOf(err).MayHaveSucceeded {
		t.Fatalf("unexpected validation error: %v metadata=%+v", err, clierrors.MetadataOf(err))
	}
}

func TestAttachmentMutationTransportErrorIsUncertain(t *testing.T) {
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{
		BaseURL:    "https://example.invalid",
		HTTPClient: &http.Client{Transport: errorTransport{err: errors.New("connection reset")}},
	})
	_, err := client.UploadInvoiceAttachment(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UploadInvoiceAttachmentRequest{
		TenantID: "tenant-1", Resource: "bill", Namespace: "bills", Type: "ACCPAY", InvoiceID: mutationInvoiceID,
		FileName: "receipt.pdf", ContentType: "application/pdf", ContentLength: 3, IdempotencyKey: "key", Body: strings.NewReader("abc"),
	})
	if clierrors.KindOf(err) != clierrors.KindMutationUncertain || !clierrors.MetadataOf(err).MayHaveSucceeded {
		t.Fatalf("unexpected transport error: %v metadata=%+v", err, clierrors.MetadataOf(err))
	}
}

func TestAttachmentMutationDoesNotFollowRedirect(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: source.URL, HTTPClient: source.Client()})
	_, err := client.UploadInvoiceAttachment(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UploadInvoiceAttachmentRequest{
		TenantID: "tenant-1", Resource: "invoice", Namespace: "invoices", Type: "ACCREC", InvoiceID: mutationInvoiceID,
		FileName: "receipt.pdf", ContentType: "application/pdf", ContentLength: 3, IdempotencyKey: "key", Body: strings.NewReader("abc"),
	})
	if clierrors.KindOf(err) != clierrors.KindMutationUncertain || targetHits != 0 {
		t.Fatalf("expected uncertain redirect without follow, err=%v targetHits=%d", err, targetHits)
	}
}
