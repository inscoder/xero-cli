package xeroapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/inscoder/xero-cli/internal/auth"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

// InvoiceWrite is the write-only representation accepted by Xero's Invoices
// endpoints. It deliberately remains separate from Invoice so response-only
// fields can never leak into a mutation payload.
type InvoiceWrite struct {
	InvoiceID           string                  `json:"InvoiceID,omitempty"`
	Type                string                  `json:"Type"`
	Contact             *InvoiceWriteContact    `json:"Contact,omitempty"`
	Date                *string                 `json:"Date,omitempty"`
	DueDate             *string                 `json:"DueDate,omitempty"`
	LineAmountTypes     *string                 `json:"LineAmountTypes,omitempty"`
	InvoiceNumber       *string                 `json:"InvoiceNumber,omitempty"`
	Reference           *string                 `json:"Reference,omitempty"`
	BrandingThemeID     *string                 `json:"BrandingThemeID,omitempty"`
	URL                 *string                 `json:"Url,omitempty"`
	CurrencyCode        *string                 `json:"CurrencyCode,omitempty"`
	CurrencyRate        *json.Number            `json:"CurrencyRate,omitempty"`
	Status              *string                 `json:"Status,omitempty"`
	SentToContact       *bool                   `json:"SentToContact,omitempty"`
	ExpectedPaymentDate *string                 `json:"ExpectedPaymentDate,omitempty"`
	PlannedPaymentDate  *string                 `json:"PlannedPaymentDate,omitempty"`
	LineItems           *[]InvoiceWriteLineItem `json:"LineItems,omitempty"`
}

type InvoiceWriteContact struct {
	ContactID string `json:"ContactID"`
}

type InvoiceWriteLineItem struct {
	LineItemID     *string                 `json:"LineItemID,omitempty"`
	Description    string                  `json:"Description"`
	Quantity       *json.Number            `json:"Quantity,omitempty"`
	UnitAmount     *json.Number            `json:"UnitAmount,omitempty"`
	ItemCode       *string                 `json:"ItemCode,omitempty"`
	AccountCode    *string                 `json:"AccountCode,omitempty"`
	AccountID      *string                 `json:"AccountID,omitempty"`
	TaxType        *string                 `json:"TaxType,omitempty"`
	TaxAmount      *json.Number            `json:"TaxAmount,omitempty"`
	LineAmount     *json.Number            `json:"LineAmount,omitempty"`
	DiscountRate   *json.Number            `json:"DiscountRate,omitempty"`
	DiscountAmount *json.Number            `json:"DiscountAmount,omitempty"`
	Tracking       *[]InvoiceWriteTracking `json:"Tracking,omitempty"`
}

type InvoiceWriteTracking struct {
	TrackingCategoryID *string `json:"TrackingCategoryID,omitempty"`
	TrackingOptionID   *string `json:"TrackingOptionID,omitempty"`
	Name               *string `json:"Name,omitempty"`
	Option             *string `json:"Option,omitempty"`
}

type CreateInvoiceRequest struct {
	TenantID       string
	Resource       string
	Namespace      string
	IdempotencyKey string
	Invoice        InvoiceWrite
}

type UpdateInvoiceRequest struct {
	TenantID       string
	Resource       string
	Namespace      string
	InvoiceID      string
	IdempotencyKey string
	Invoice        InvoiceWrite
}

type InvoiceMutationResult struct {
	Operation            string `json:"operation"`
	Resource             string `json:"resource"`
	InvoiceID            string `json:"invoiceId"`
	TenantID             string `json:"tenantId"`
	InvoiceNumber        string `json:"invoiceNumber,omitempty"`
	Type                 string `json:"type"`
	Status               string `json:"status"`
	UpdatedAt            string `json:"updatedAt,omitempty"`
	LineItemsReplaced    bool   `json:"lineItemsReplaced"`
	RemovedLineItemCount int    `json:"removedLineItemCount"`
	IdempotencyKey       string `json:"idempotencyKey"`
}

func (c *Client) CreateInvoice(ctx context.Context, token auth.TokenSet, request CreateInvoiceRequest) (InvoiceMutationResult, error) {
	return c.mutateInvoice(ctx, token, invoiceMutationRequest{
		method:         http.MethodPut,
		path:           "/api.xro/2.0/Invoices",
		operation:      "created",
		resource:       request.Resource,
		namespace:      request.Namespace,
		tenantID:       request.TenantID,
		idempotencyKey: request.IdempotencyKey,
		invoice:        request.Invoice,
	})
}

func (c *Client) UpdateInvoice(ctx context.Context, token auth.TokenSet, request UpdateInvoiceRequest) (InvoiceMutationResult, error) {
	return c.mutateInvoice(ctx, token, invoiceMutationRequest{
		method:         http.MethodPost,
		path:           "/api.xro/2.0/Invoices/" + url.PathEscape(request.InvoiceID),
		operation:      "updated",
		resource:       request.Resource,
		namespace:      request.Namespace,
		tenantID:       request.TenantID,
		invoiceID:      request.InvoiceID,
		idempotencyKey: request.IdempotencyKey,
		invoice:        request.Invoice,
	})
}

type invoiceMutationRequest struct {
	method         string
	path           string
	operation      string
	resource       string
	namespace      string
	tenantID       string
	invoiceID      string
	idempotencyKey string
	invoice        InvoiceWrite
}

type invoiceWriteEnvelope struct {
	Invoices []InvoiceWrite `json:"Invoices"`
}

func (c *Client) mutateInvoice(ctx context.Context, token auth.TokenSet, mutation invoiceMutationRequest) (InvoiceMutationResult, error) {
	body, err := json.Marshal(invoiceWriteEnvelope{Invoices: []InvoiceWrite{mutation.invoice}})
	if err != nil {
		return InvoiceMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "encode Xero invoice mutation", err)
	}
	endpoint, err := url.Parse(c.baseURL + mutation.path)
	if err != nil {
		return InvoiceMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoice mutation URL", err)
	}
	// Wrap the buffer so net/http does not populate GetBody. A replayable body
	// plus Idempotency-Key can otherwise trigger a transparent transport retry.
	req, err := http.NewRequestWithContext(ctx, mutation.method, endpoint.String(), io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return InvoiceMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoice mutation request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xero-tenant-id", mutation.tenantID)
	req.Header.Set("Idempotency-Key", mutation.idempotencyKey)

	resp, err := c.doInvoiceMutation(req)
	if err != nil {
		return InvoiceMutationResult{}, mutationUncertain(mutation, "Xero invoice mutation was dispatched but its outcome could not be confirmed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 || resp.StatusCode < 200 || resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return InvoiceMutationResult{}, mutationUncertain(mutation, fmt.Sprintf("Xero invoice mutation returned %s and may have succeeded", resp.Status), nil)
	}
	if resp.StatusCode >= 400 {
		return InvoiceMutationResult{}, decodeAPIError(resp)
	}

	var payload invoicesResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&payload); err != nil {
		return InvoiceMutationResult{}, mutationUncertain(mutation, "Xero invoice mutation response could not be decoded", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return InvoiceMutationResult{}, mutationUncertain(mutation, "Xero invoice mutation response contained trailing data", err)
	}
	if len(payload.Invoices) != 1 {
		return InvoiceMutationResult{}, mutationUncertain(mutation, fmt.Sprintf("Xero invoice mutation response must contain exactly one invoice; got %d", len(payload.Invoices)), nil)
	}
	invoice := payload.Invoices[0]
	if err := invoicePayloadValidationError(invoice, strings.TrimSuffix(mutation.operation, "d")+" invoice"); err != nil {
		return InvoiceMutationResult{}, err
	}
	if strings.TrimSpace(invoice.InvoiceID) == "" || mutation.invoiceID != "" && !strings.EqualFold(invoice.InvoiceID, mutation.invoiceID) {
		return InvoiceMutationResult{}, mutationUncertain(mutation, "Xero invoice mutation response did not match the expected invoice ID", nil)
	}
	if mutation.invoiceID == "" {
		mutation.invoiceID = invoice.InvoiceID
	}
	if !strings.EqualFold(invoice.Type, mutation.invoice.Type) {
		return InvoiceMutationResult{}, mutationUncertain(mutation, "Xero invoice mutation response did not match the expected invoice type", nil)
	}

	return InvoiceMutationResult{
		Operation:      mutation.operation,
		Resource:       mutation.resource,
		InvoiceID:      invoice.InvoiceID,
		TenantID:       mutation.tenantID,
		InvoiceNumber:  invoice.InvoiceNumber,
		Type:           invoice.Type,
		Status:         invoice.Status,
		UpdatedAt:      normalizeTimestamp(invoice.UpdatedDateUTC),
		IdempotencyKey: mutation.idempotencyKey,
	}, nil
}

func (c *Client) doInvoiceMutation(request *http.Request) (*http.Response, error) {
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client.Do(request)
}

func mutationUncertain(mutation invoiceMutationRequest, message string, cause error) error {
	metadata := clierrors.Metadata{
		MayHaveSucceeded: true,
		Operation:        mutation.operation,
		Resource:         mutation.resource,
		TenantID:         mutation.tenantID,
		InvoiceID:        mutation.invoiceID,
		IdempotencyKey:   mutation.idempotencyKey,
		RecoveryCommand:  invoiceRecoveryCommand(mutation),
	}
	if cause != nil {
		return clierrors.WrapWithMetadata(clierrors.KindMutationUncertain, message, cause, metadata)
	}
	return clierrors.NewWithMetadata(clierrors.KindMutationUncertain, message, metadata)
}

func invoiceRecoveryCommand(mutation invoiceMutationRequest) string {
	if mutation.invoiceID != "" {
		return fmt.Sprintf("xero %s --invoice-id %s --json", mutation.namespace, mutation.invoiceID)
	}
	if mutation.invoice.InvoiceNumber != nil && strings.TrimSpace(*mutation.invoice.InvoiceNumber) != "" {
		return fmt.Sprintf("xero %s --where %s --json", mutation.namespace, shellSingleQuote(`InvoiceNumber=="`+escapeXeroWhere(*mutation.invoice.InvoiceNumber)+`"`))
	}
	if mutation.invoice.Reference != nil && strings.TrimSpace(*mutation.invoice.Reference) != "" {
		return fmt.Sprintf("xero %s --where %s --json", mutation.namespace, shellSingleQuote(`Reference=="`+escapeXeroWhere(*mutation.invoice.Reference)+`"`))
	}
	return fmt.Sprintf("xero %s --order 'UpdatedDateUTC DESC' --page-size 10 --json", mutation.namespace)
}

func escapeXeroWhere(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
