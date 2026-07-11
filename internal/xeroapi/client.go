package xeroapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

const defaultBaseURL = "https://api.xero.com"

var xeroDatePattern = regexp.MustCompile(`^/Date\((\d+)([+-]\d{4})?\)/$`)

type Invoice struct {
	InvoiceID           string              `json:"invoiceId"`
	Type                string              `json:"type"`
	InvoiceNumber       string              `json:"invoiceNumber"`
	Reference           string              `json:"reference"`
	Contact             InvoiceContact      `json:"contact,omitempty"`
	Date                string              `json:"date"`
	DueDate             string              `json:"dueDate"`
	Status              string              `json:"status"`
	LineAmountTypes     string              `json:"lineAmountTypes"`
	LineItems           []InvoiceLineItem   `json:"lineItems"`
	SubTotal            float64             `json:"subTotal"`
	TotalTax            float64             `json:"totalTax"`
	Total               float64             `json:"total"`
	TotalDiscount       float64             `json:"totalDiscount"`
	AmountDue           float64             `json:"amountDue"`
	AmountPaid          float64             `json:"amountPaid"`
	AmountCredited      float64             `json:"amountCredited"`
	CurrencyCode        string              `json:"currencyCode"`
	CurrencyRate        float64             `json:"currencyRate"`
	UpdatedAt           string              `json:"updatedAt"`
	BrandingThemeID     string              `json:"brandingThemeId"`
	URL                 string              `json:"url"`
	SentToContact       bool                `json:"sentToContact"`
	ExpectedPaymentDate string              `json:"expectedPaymentDate"`
	PlannedPaymentDate  string              `json:"plannedPaymentDate"`
	HasAttachments      bool                `json:"hasAttachments"`
	Attachments         []InvoiceAttachment `json:"attachments,omitempty"`
	HasErrors           bool                `json:"hasErrors,omitempty"`
	ValidationErrors    []string            `json:"validationErrors,omitempty"`
	Warnings            []string            `json:"warnings,omitempty"`
	Payments            []InvoicePayment    `json:"payments"`
	CreditNotes         []InvoiceAllocation `json:"creditNotes"`
	Prepayments         []InvoiceAllocation `json:"prepayments"`
	Overpayments        []InvoiceAllocation `json:"overpayments"`
	ContactName         string              `json:"contactName,omitempty"`
	Currency            string              `json:"currency,omitempty"`
}

type ListInvoicesRequest struct {
	TenantID   string
	Type       string
	InvoiceIDs []string
	Statuses   []string
	Since      string
	Where      string
	Order      string
	Page       int
	PageSize   int
}

type GetOnlineInvoiceRequest struct {
	TenantID  string
	InvoiceID string
}

type GetInvoiceRequest struct {
	TenantID  string
	InvoiceID string
}

type GetInvoicePDFRequest struct {
	TenantID  string
	InvoiceID string
}

type ApproveInvoiceRequest struct {
	TenantID  string
	InvoiceID string
}

type OnlineInvoiceResult struct {
	InvoiceID        string `json:"invoiceId"`
	OnlineInvoiceURL string `json:"onlineInvoiceUrl,omitempty"`
	Available        bool   `json:"available"`
}

type InvoicePDFResult struct {
	InvoiceID   string `json:"invoiceId"`
	ContentType string `json:"contentType"`
	Bytes       int64  `json:"bytes"`
	Output      string `json:"output"`
	SavedTo     string `json:"savedTo,omitempty"`
	Streamed    bool   `json:"streamed"`
}

type InvoiceApprovalResult struct {
	InvoiceID      string `json:"invoiceId"`
	TenantID       string `json:"tenantId"`
	InvoiceNumber  string `json:"invoiceNumber,omitempty"`
	Type           string `json:"type,omitempty"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	StatusObserved bool   `json:"statusObserved"`
}

type InvoiceContact struct {
	ContactID     string `json:"contactId,omitempty"`
	Name          string `json:"name,omitempty"`
	ContactNumber string `json:"contactNumber,omitempty"`
}

type InvoiceAttachment struct {
	AttachmentID     string   `json:"attachmentId"`
	FileName         string   `json:"fileName"`
	URL              string   `json:"url,omitempty"`
	MimeType         string   `json:"mimeType,omitempty"`
	ContentLength    int64    `json:"contentLength"`
	IncludeOnline    bool     `json:"includeOnline,omitempty"`
	HasErrors        bool     `json:"hasErrors,omitempty"`
	ValidationErrors []string `json:"validationErrors,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

type InvoiceLineItem struct {
	Description string             `json:"description"`
	Quantity    float64            `json:"quantity"`
	UnitAmount  float64            `json:"unitAmount"`
	ItemCode    string             `json:"itemCode"`
	AccountCode string             `json:"accountCode"`
	AccountID   string             `json:"accountId"`
	TaxType     string             `json:"taxType"`
	TaxAmount   float64            `json:"taxAmount"`
	LineAmount  float64            `json:"lineAmount"`
	LineItemID  string             `json:"lineItemId"`
	Tracking    []TrackingCategory `json:"tracking"`
}

type TrackingCategory struct {
	Name   string `json:"name"`
	Option string `json:"option"`
}

type InvoicePayment struct {
	PaymentID    string  `json:"paymentId"`
	Date         string  `json:"date"`
	Amount       float64 `json:"amount"`
	Reference    string  `json:"reference"`
	CurrencyRate float64 `json:"currencyRate"`
	PaymentType  string  `json:"paymentType"`
	Status       string  `json:"status"`
}

type InvoiceAllocation struct {
	AllocationID string  `json:"allocationId"`
	Type         string  `json:"type"`
	Date         string  `json:"date"`
	Amount       float64 `json:"amount"`
	Status       string  `json:"status"`
}

type InvoiceClient interface {
	ListInvoices(context.Context, auth.TokenSet, ListInvoicesRequest) ([]Invoice, error)
	GetInvoice(context.Context, auth.TokenSet, GetInvoiceRequest) (Invoice, error)
	CreateInvoice(context.Context, auth.TokenSet, CreateInvoiceRequest) (InvoiceMutationResult, error)
	UpdateInvoice(context.Context, auth.TokenSet, UpdateInvoiceRequest) (InvoiceMutationResult, error)
	UploadInvoiceAttachment(context.Context, auth.TokenSet, UploadInvoiceAttachmentRequest) (InvoiceAttachmentMutationResult, error)
	GetOnlineInvoice(context.Context, auth.TokenSet, GetOnlineInvoiceRequest) (OnlineInvoiceResult, error)
	GetInvoicePDF(context.Context, auth.TokenSet, GetInvoicePDFRequest, io.Writer) (InvoicePDFResult, error)
	ApproveInvoice(context.Context, auth.TokenSet, ApproveInvoiceRequest) (InvoiceApprovalResult, error)
}

type writeFailure struct {
	err error
}

func (w *writeFailure) Error() string {
	if w == nil || w.err == nil {
		return ""
	}
	return w.err.Error()
}

func (w *writeFailure) Unwrap() error {
	if w == nil {
		return nil
	}
	return w.err
}

type classifiedWriter struct {
	writer io.Writer
}

func (w classifiedWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err != nil {
		return n, &writeFailure{err: err}
	}
	return n, nil
}

type ClientOptions struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	settings   appconfig.Settings
	baseURL    string
	httpClient *http.Client
}

type invoicesResponse struct {
	Invoices []invoicePayload `json:"Invoices"`
}

type onlineInvoicesResponse struct {
	OnlineInvoices []onlineInvoicePayload `json:"OnlineInvoices"`
}

type onlineInvoicePayload struct {
	OnlineInvoiceURL string `json:"OnlineInvoiceUrl"`
}

type invoicePayload struct {
	InvoiceID           string                `json:"InvoiceID"`
	Type                string                `json:"Type"`
	InvoiceNumber       string                `json:"InvoiceNumber"`
	Reference           string                `json:"Reference"`
	Contact             invoiceContactPayload `json:"Contact"`
	Date                string                `json:"Date"`
	DueDate             string                `json:"DueDate"`
	Status              string                `json:"Status"`
	LineAmountTypes     string                `json:"LineAmountTypes"`
	LineItems           []lineItemPayload     `json:"LineItems"`
	SubTotal            float64               `json:"SubTotal"`
	TotalTax            float64               `json:"TotalTax"`
	Total               float64               `json:"Total"`
	TotalDiscount       float64               `json:"TotalDiscount"`
	AmountDue           float64               `json:"AmountDue"`
	AmountPaid          float64               `json:"AmountPaid"`
	AmountCredited      float64               `json:"AmountCredited"`
	CurrencyCode        string                `json:"CurrencyCode"`
	CurrencyRate        float64               `json:"CurrencyRate"`
	UpdatedDateUTC      string                `json:"UpdatedDateUTC"`
	BrandingThemeID     string                `json:"BrandingThemeID"`
	URL                 string                `json:"Url"`
	SentToContact       bool                  `json:"SentToContact"`
	ExpectedPaymentDate string                `json:"ExpectedPaymentDate"`
	PlannedPaymentDate  string                `json:"PlannedPaymentDate"`
	HasAttachments      bool                  `json:"HasAttachments"`
	Attachments         []attachmentPayload   `json:"Attachments"`
	HasErrors           bool                  `json:"HasErrors"`
	ValidationErrors    []messagePayload      `json:"ValidationErrors"`
	Warnings            []messagePayload      `json:"Warnings"`
	Payments            []paymentPayload      `json:"Payments"`
	CreditNotes         []creditNotePayload   `json:"CreditNotes"`
	Prepayments         []allocationPayload   `json:"Prepayments"`
	Overpayments        []allocationPayload   `json:"Overpayments"`
}

type attachmentPayload struct {
	AttachmentID     string           `json:"AttachmentID"`
	FileName         string           `json:"FileName"`
	URL              string           `json:"Url"`
	MimeType         string           `json:"MimeType"`
	ContentLength    int64            `json:"ContentLength"`
	IncludeOnline    bool             `json:"IncludeOnline"`
	HasErrors        bool             `json:"HasErrors"`
	ValidationErrors []messagePayload `json:"ValidationErrors"`
	Warnings         []messagePayload `json:"Warnings"`
}

type messagePayload struct {
	Message string `json:"Message"`
}

type invoiceContactPayload struct {
	ContactID     string `json:"ContactID"`
	Name          string `json:"Name"`
	ContactNumber string `json:"ContactNumber"`
}

type lineItemPayload struct {
	Description string            `json:"Description"`
	Quantity    float64           `json:"Quantity"`
	UnitAmount  float64           `json:"UnitAmount"`
	ItemCode    string            `json:"ItemCode"`
	AccountCode string            `json:"AccountCode"`
	AccountID   string            `json:"AccountID"`
	TaxType     string            `json:"TaxType"`
	TaxAmount   float64           `json:"TaxAmount"`
	LineAmount  float64           `json:"LineAmount"`
	LineItemID  string            `json:"LineItemID"`
	Tracking    []trackingPayload `json:"Tracking"`
}

type trackingPayload struct {
	Name   string `json:"Name"`
	Option string `json:"Option"`
}

type paymentPayload struct {
	PaymentID    string  `json:"PaymentID"`
	Date         string  `json:"Date"`
	Amount       float64 `json:"Amount"`
	Reference    string  `json:"Reference"`
	CurrencyRate float64 `json:"CurrencyRate"`
	PaymentType  string  `json:"PaymentType"`
	Status       string  `json:"Status"`
}

type creditNotePayload struct {
	CreditNoteID  string  `json:"CreditNoteID"`
	Type          string  `json:"Type"`
	Date          string  `json:"Date"`
	AppliedAmount float64 `json:"AppliedAmount"`
	Status        string  `json:"Status"`
}

type allocationPayload struct {
	PrepaymentID  string  `json:"PrepaymentID"`
	OverpaymentID string  `json:"OverpaymentID"`
	Type          string  `json:"Type"`
	Date          string  `json:"Date"`
	AppliedAmount float64 `json:"AppliedAmount"`
	Status        string  `json:"Status"`
}

type apiErrorPayload struct {
	Type             string                   `json:"Type"`
	Title            string                   `json:"Title"`
	Detail           string                   `json:"Detail"`
	Message          string                   `json:"Message"`
	Error            string                   `json:"error"`
	ValidationErrors []messagePayload         `json:"ValidationErrors"`
	Elements         []apiErrorElementPayload `json:"Elements"`
	Invoices         []invoicePayload         `json:"Invoices"`
	Attachments      []attachmentPayload      `json:"Attachments"`
	Contacts         []contactResponsePayload `json:"Contacts"`
}

type apiErrorElementPayload struct {
	ValidationErrors []messagePayload `json:"ValidationErrors"`
}

func NewClient(settings appconfig.Settings, options ClientOptions) *Client {
	baseURL := strings.TrimRight(options.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{settings: settings, baseURL: baseURL, httpClient: httpClient}
}

func (c *Client) ListInvoices(ctx context.Context, token auth.TokenSet, request ListInvoicesRequest) ([]Invoice, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Invoices")
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoices URL", err)
	}

	query := endpoint.Query()
	if len(request.InvoiceIDs) > 0 {
		query.Set("IDs", strings.Join(request.InvoiceIDs, ","))
	}
	if len(request.Statuses) > 0 {
		query.Set("Statuses", strings.Join(request.Statuses, ","))
	}
	if where := combineInvoiceWhere(request.Type, request.Where); where != "" {
		query.Set("where", where)
	}
	if request.Order != "" {
		query.Set("order", request.Order)
	}
	if request.Page > 0 {
		query.Set("page", strconv.Itoa(request.Page))
	}
	if request.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(request.PageSize))
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoices request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)
	if request.Since != "" {
		since, err := time.Parse("2006-01-02", request.Since)
		if err != nil {
			return nil, clierrors.Wrap(clierrors.KindValidation, "parse --since date", err)
		}
		req.Header.Set("If-Modified-Since", since.UTC().Format(time.RFC1123))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindNetwork, "send Xero invoices request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp)
	}

	var payload invoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero invoices response", err)
	}

	invoices := make([]Invoice, 0, len(payload.Invoices))
	for _, item := range payload.Invoices {
		invoices = append(invoices, normalizeInvoice(item))
	}
	return invoices, nil
}

func (c *Client) GetInvoice(ctx context.Context, token auth.TokenSet, request GetInvoiceRequest) (Invoice, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Invoices/" + url.PathEscape(request.InvoiceID))
	if err != nil {
		return Invoice{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoice URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Invoice{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoice request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Invoice{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero invoice request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Invoice{}, decodeAPIError(resp)
	}

	var payload invoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Invoice{}, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero invoice response", err)
	}
	if len(payload.Invoices) != 1 {
		return Invoice{}, clierrors.New(clierrors.KindXeroRequest, fmt.Sprintf("Xero invoice response must contain exactly one invoice; got %d", len(payload.Invoices)))
	}
	item := payload.Invoices[0]
	if strings.TrimSpace(item.InvoiceID) == "" || !strings.EqualFold(item.InvoiceID, request.InvoiceID) {
		return Invoice{}, clierrors.New(clierrors.KindXeroRequest, "Xero invoice response did not match the requested invoice ID")
	}
	if err := invoicePayloadValidationError(item, "get invoice"); err != nil {
		return Invoice{}, err
	}
	return normalizeInvoice(item), nil
}

func (c *Client) GetOnlineInvoice(ctx context.Context, token auth.TokenSet, request GetOnlineInvoiceRequest) (OnlineInvoiceResult, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Invoices/" + url.PathEscape(request.InvoiceID) + "/OnlineInvoice")
	if err != nil {
		return OnlineInvoiceResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero online invoice URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return OnlineInvoiceResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero online invoice request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return OnlineInvoiceResult{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero online invoice request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return OnlineInvoiceResult{}, decodeAPIError(resp)
	}

	var payload onlineInvoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return OnlineInvoiceResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero online invoice response", err)
	}

	result := OnlineInvoiceResult{InvoiceID: request.InvoiceID}
	for _, item := range payload.OnlineInvoices {
		if strings.TrimSpace(item.OnlineInvoiceURL) == "" {
			continue
		}
		result.OnlineInvoiceURL = item.OnlineInvoiceURL
		result.Available = true
		break
	}
	return result, nil
}

func (c *Client) GetInvoicePDF(ctx context.Context, token auth.TokenSet, request GetInvoicePDFRequest, writer io.Writer) (InvoicePDFResult, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Invoices/" + url.PathEscape(request.InvoiceID))
	if err != nil {
		return InvoicePDFResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoice PDF URL", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return InvoicePDFResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoice PDF request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set("Xero-tenant-id", request.TenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return InvoicePDFResult{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero invoice PDF request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return InvoicePDFResult{}, decodeAPIError(resp)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(strings.ToLower(contentType), "application/pdf") {
		return InvoicePDFResult{}, clierrors.New(clierrors.KindXeroRequest, fmt.Sprintf("unexpected invoice PDF content type %q", contentType))
	}

	bytesWritten, err := io.Copy(classifiedWriter{writer: writer}, resp.Body)
	if err != nil {
		var writeErr *writeFailure
		if errors.As(err, &writeErr) {
			return InvoicePDFResult{}, clierrors.Wrap(clierrors.KindInternal, "write invoice PDF output", writeErr.Unwrap())
		}
		return InvoicePDFResult{}, clierrors.Wrap(clierrors.KindNetwork, "stream Xero invoice PDF response", err)
	}

	return InvoicePDFResult{
		InvoiceID:   request.InvoiceID,
		ContentType: contentType,
		Bytes:       bytesWritten,
	}, nil
}

func (c *Client) ApproveInvoice(ctx context.Context, token auth.TokenSet, request ApproveInvoiceRequest) (InvoiceApprovalResult, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Invoices")
	if err != nil {
		return InvoiceApprovalResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero approve invoice URL", err)
	}

	body, err := json.Marshal(map[string]any{
		"Invoices": []map[string]string{{
			"InvoiceID": request.InvoiceID,
			"Status":    "AUTHORISED",
		}},
	})
	if err != nil {
		return InvoiceApprovalResult{}, clierrors.Wrap(clierrors.KindInternal, "encode invoice approval payload", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return InvoiceApprovalResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero approve invoice request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return InvoiceApprovalResult{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero approve invoice request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return InvoiceApprovalResult{}, decodeAPIError(resp)
	}

	var payload invoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return InvoiceApprovalResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero approve invoice response", err)
	}
	if len(payload.Invoices) == 0 {
		return InvoiceApprovalResult{}, clierrors.New(clierrors.KindXeroRequest, "Xero approve invoice response did not include an invoice")
	}

	invoice := payload.Invoices[0]
	if err := invoicePayloadValidationError(invoice, "approve invoice"); err != nil {
		return InvoiceApprovalResult{}, err
	}
	result := InvoiceApprovalResult{
		InvoiceID:      firstNonEmpty(invoice.InvoiceID, request.InvoiceID),
		TenantID:       request.TenantID,
		InvoiceNumber:  invoice.InvoiceNumber,
		Type:           invoice.Type,
		Status:         invoice.Status,
		UpdatedAt:      normalizeTimestamp(invoice.UpdatedDateUTC),
		StatusObserved: strings.EqualFold(invoice.Status, "AUTHORISED"),
	}
	if !result.StatusObserved {
		message := "invoice approval was not confirmed by Xero"
		if status := strings.TrimSpace(invoice.Status); status != "" {
			message = fmt.Sprintf("invoice approval was not confirmed by Xero: expected AUTHORISED, got %s", status)
		}
		return InvoiceApprovalResult{}, clierrors.New(clierrors.KindXeroAPI, message)
	}
	return result, nil
}

func decodeAPIError(resp *http.Response) error {
	var payload apiErrorPayload
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	message := firstNonEmpty(payload.Detail, payload.Message, payload.Error, payload.Title, resp.Status)
	validationErrors := collectAPIValidationErrors(payload)
	if len(validationErrors) > 0 {
		message = appendValidationErrors(message, validationErrors)
	}
	metadata := clierrors.Metadata{ValidationErrors: validationErrors}
	if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds > 0 {
		metadata.RetryAfterSeconds = seconds
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return clierrors.NewWithMetadata(clierrors.KindRateLimit, message, metadata)
	case http.StatusUnauthorized:
		return clierrors.NewWithMetadata(clierrors.KindAuthRequired, message, metadata)
	case http.StatusForbidden:
		return clierrors.NewWithMetadata(clierrors.KindPermissionDenied, message, metadata)
	default:
		return clierrors.NewWithMetadata(clierrors.KindXeroAPI, message, metadata)
	}
}

func collectAPIValidationErrors(payload apiErrorPayload) []string {
	messages := make([]string, 0)
	messages = appendMessagePayloads(messages, payload.ValidationErrors)
	for _, element := range payload.Elements {
		messages = appendMessagePayloads(messages, element.ValidationErrors)
	}
	for _, invoice := range payload.Invoices {
		messages = appendMessagePayloads(messages, invoice.ValidationErrors)
	}
	for _, attachment := range payload.Attachments {
		messages = appendMessagePayloads(messages, attachment.ValidationErrors)
	}
	for _, contact := range payload.Contacts {
		messages = appendMessagePayloads(messages, contact.ValidationErrors)
	}
	return uniqueMessages(messages)
}

func invoicePayloadValidationError(payload invoicePayload, action string) error {
	validationErrors := uniqueMessages(appendMessagePayloads(nil, payload.ValidationErrors))
	if !payload.HasErrors && len(validationErrors) == 0 {
		return nil
	}
	message := "Xero reported invoice validation errors"
	if strings.TrimSpace(action) != "" {
		message = "Xero could not " + action
	}
	return clierrors.NewWithMetadata(clierrors.KindXeroAPI, appendValidationErrors(message, validationErrors), clierrors.Metadata{
		ValidationErrors: validationErrors,
	})
}

func appendValidationErrors(message string, validationErrors []string) string {
	if len(validationErrors) == 0 {
		return message
	}
	remaining := make([]string, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		if strings.Contains(message, validationError) {
			continue
		}
		remaining = append(remaining, validationError)
	}
	if len(remaining) == 0 {
		return message
	}
	return message + ": " + strings.Join(remaining, "; ")
}

func appendMessagePayloads(messages []string, payloads []messagePayload) []string {
	for _, payload := range payloads {
		if message := strings.TrimSpace(payload.Message); message != "" {
			messages = append(messages, message)
		}
	}
	return messages
}

func uniqueMessages(messages []string) []string {
	unique := make([]string, 0, len(messages))
	seen := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		if _, ok := seen[message]; ok {
			continue
		}
		seen[message] = struct{}{}
		unique = append(unique, message)
	}
	return unique
}

func normalizeInvoice(item invoicePayload) Invoice {
	return Invoice{
		InvoiceID:           item.InvoiceID,
		Type:                item.Type,
		InvoiceNumber:       item.InvoiceNumber,
		Reference:           item.Reference,
		Contact:             normalizeInvoiceContact(item.Contact),
		Date:                normalizeDate(item.Date),
		DueDate:             normalizeDate(item.DueDate),
		Status:              item.Status,
		LineAmountTypes:     item.LineAmountTypes,
		LineItems:           normalizeLineItems(item.LineItems),
		SubTotal:            item.SubTotal,
		TotalTax:            item.TotalTax,
		Total:               item.Total,
		TotalDiscount:       item.TotalDiscount,
		AmountDue:           item.AmountDue,
		AmountPaid:          item.AmountPaid,
		AmountCredited:      item.AmountCredited,
		CurrencyCode:        item.CurrencyCode,
		CurrencyRate:        item.CurrencyRate,
		UpdatedAt:           normalizeTimestamp(item.UpdatedDateUTC),
		BrandingThemeID:     item.BrandingThemeID,
		URL:                 item.URL,
		SentToContact:       item.SentToContact,
		ExpectedPaymentDate: normalizeDate(item.ExpectedPaymentDate),
		PlannedPaymentDate:  normalizeDate(item.PlannedPaymentDate),
		HasAttachments:      item.HasAttachments,
		Attachments:         normalizeAttachments(item.Attachments),
		HasErrors:           item.HasErrors,
		ValidationErrors:    uniqueMessages(appendMessagePayloads(nil, item.ValidationErrors)),
		Warnings:            uniqueMessages(appendMessagePayloads(nil, item.Warnings)),
		Payments:            normalizePayments(item.Payments),
		CreditNotes:         normalizeCreditNotes(item.CreditNotes),
		Prepayments:         normalizeAllocations(item.Prepayments),
		Overpayments:        normalizeAllocations(item.Overpayments),
		ContactName:         item.Contact.Name,
		Currency:            item.CurrencyCode,
	}
}

func normalizeAttachments(raw []attachmentPayload) []InvoiceAttachment {
	attachments := make([]InvoiceAttachment, 0, len(raw))
	for _, attachment := range raw {
		attachments = append(attachments, InvoiceAttachment{
			AttachmentID:     attachment.AttachmentID,
			FileName:         attachment.FileName,
			URL:              attachment.URL,
			MimeType:         attachment.MimeType,
			ContentLength:    attachment.ContentLength,
			IncludeOnline:    attachment.IncludeOnline,
			HasErrors:        attachment.HasErrors,
			ValidationErrors: uniqueMessages(appendMessagePayloads(nil, attachment.ValidationErrors)),
			Warnings:         uniqueMessages(appendMessagePayloads(nil, attachment.Warnings)),
		})
	}
	return attachments
}

func normalizeInvoiceContact(raw invoiceContactPayload) InvoiceContact {
	return InvoiceContact{
		ContactID:     raw.ContactID,
		Name:          raw.Name,
		ContactNumber: raw.ContactNumber,
	}
}

func normalizeLineItems(raw []lineItemPayload) []InvoiceLineItem {
	items := make([]InvoiceLineItem, 0, len(raw))
	for _, item := range raw {
		items = append(items, InvoiceLineItem{
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitAmount:  item.UnitAmount,
			ItemCode:    item.ItemCode,
			AccountCode: item.AccountCode,
			AccountID:   item.AccountID,
			TaxType:     item.TaxType,
			TaxAmount:   item.TaxAmount,
			LineAmount:  item.LineAmount,
			LineItemID:  item.LineItemID,
			Tracking:    normalizeTracking(item.Tracking),
		})
	}
	return items
}

func normalizeTracking(raw []trackingPayload) []TrackingCategory {
	tracking := make([]TrackingCategory, 0, len(raw))
	for _, item := range raw {
		tracking = append(tracking, TrackingCategory{Name: item.Name, Option: item.Option})
	}
	return tracking
}

func normalizePayments(raw []paymentPayload) []InvoicePayment {
	payments := make([]InvoicePayment, 0, len(raw))
	for _, item := range raw {
		payments = append(payments, InvoicePayment{
			PaymentID:    item.PaymentID,
			Date:         normalizeDate(item.Date),
			Amount:       item.Amount,
			Reference:    item.Reference,
			CurrencyRate: item.CurrencyRate,
			PaymentType:  item.PaymentType,
			Status:       item.Status,
		})
	}
	return payments
}

func normalizeCreditNotes(raw []creditNotePayload) []InvoiceAllocation {
	allocations := make([]InvoiceAllocation, 0, len(raw))
	for _, item := range raw {
		allocations = append(allocations, InvoiceAllocation{
			AllocationID: item.CreditNoteID,
			Type:         item.Type,
			Date:         normalizeDate(item.Date),
			Amount:       item.AppliedAmount,
			Status:       item.Status,
		})
	}
	return allocations
}

func normalizeAllocations(raw []allocationPayload) []InvoiceAllocation {
	allocations := make([]InvoiceAllocation, 0, len(raw))
	for _, item := range raw {
		allocations = append(allocations, InvoiceAllocation{
			AllocationID: firstNonEmpty(item.PrepaymentID, item.OverpaymentID),
			Type:         item.Type,
			Date:         normalizeDate(item.Date),
			Amount:       item.AppliedAmount,
			Status:       item.Status,
		})
	}
	return allocations
}

func normalizeDate(raw string) string {
	if raw == "" {
		return ""
	}
	if value, ok := parseXeroWrappedDate(raw); ok {
		return value.UTC().Format("2006-01-02")
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC().Format("2006-01-02")
		}
	}
	return raw
}

func normalizeTimestamp(raw string) string {
	if raw == "" {
		return ""
	}
	if value, ok := parseXeroWrappedDate(raw); ok {
		return value.UTC().Format(time.RFC3339)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return raw
}

func parseXeroWrappedDate(raw string) (time.Time, bool) {
	matches := xeroDatePattern.FindStringSubmatch(raw)
	if len(matches) == 0 {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

func NewRequestSummary(count int) string {
	return NewTypedRequestSummary(count, "invoice", "invoices")
}

func NewTypedRequestSummary(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func combineInvoiceWhere(invoiceType, where string) string {
	trimmedWhere := strings.TrimSpace(where)
	trimmedType := strings.TrimSpace(invoiceType)
	if trimmedType == "" {
		return trimmedWhere
	}
	typeWhere := fmt.Sprintf(`Type=="%s"`, trimmedType)
	if trimmedWhere == "" {
		return typeWhere
	}
	return typeWhere + " AND (" + trimmedWhere + ")"
}

func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 90*time.Second)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
