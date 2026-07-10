package xeroapi

import (
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

type UploadInvoiceAttachmentRequest struct {
	TenantID       string
	Resource       string
	Namespace      string
	Type           string
	InvoiceID      string
	FileName       string
	ContentType    string
	ContentLength  int64
	IdempotencyKey string
	IncludeOnline  bool
	Replace        bool
	Body           io.Reader
}

type InvoiceAttachmentMutationResult struct {
	Operation      string `json:"operation"`
	Resource       string `json:"resource"`
	InvoiceID      string `json:"invoiceId"`
	TenantID       string `json:"tenantId"`
	Type           string `json:"type"`
	AttachmentID   string `json:"attachmentId"`
	FileName       string `json:"fileName"`
	ContentType    string `json:"contentType"`
	Bytes          int64  `json:"bytes"`
	IncludeOnline  *bool  `json:"includeOnline,omitempty"`
	Overwritten    bool   `json:"overwritten"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type attachmentsResponse struct {
	Attachments []attachmentPayload `json:"Attachments"`
}

func (c *Client) UploadInvoiceAttachment(ctx context.Context, token auth.TokenSet, request UploadInvoiceAttachmentRequest) (InvoiceAttachmentMutationResult, error) {
	if request.Body == nil {
		return InvoiceAttachmentMutationResult{}, clierrors.New(clierrors.KindXeroRequest, "build Xero attachment request: attachment body is required")
	}
	if request.ContentLength <= 0 {
		return InvoiceAttachmentMutationResult{}, clierrors.New(clierrors.KindXeroRequest, "build Xero attachment request: content length must be positive")
	}

	path := "/api.xro/2.0/Invoices/" + url.PathEscape(request.InvoiceID) + "/Attachments/" + url.PathEscape(request.FileName)
	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return InvoiceAttachmentMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero attachment URL", err)
	}
	method := http.MethodPut
	operation := "uploaded"
	if request.Replace {
		method = http.MethodPost
		operation = "replaced"
	} else if request.IncludeOnline {
		query := endpoint.Query()
		query.Set("IncludeOnline", "true")
		endpoint.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), request.Body)
	if err != nil {
		return InvoiceAttachmentMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero attachment request", err)
	}
	req.ContentLength = request.ContentLength
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", request.ContentType)
	req.Header.Set("Xero-tenant-id", request.TenantID)
	req.Header.Set("Idempotency-Key", request.IdempotencyKey)

	mutation := attachmentMutationRequest{
		operation:      operation,
		resource:       request.Resource,
		namespace:      request.Namespace,
		tenantID:       request.TenantID,
		invoiceID:      request.InvoiceID,
		fileName:       request.FileName,
		idempotencyKey: request.IdempotencyKey,
		includeOnline:  request.IncludeOnline,
		replace:        request.Replace,
	}
	resp, err := c.doAttachmentMutation(req)
	if err != nil {
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, "Xero attachment mutation was dispatched but its outcome could not be confirmed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 || resp.StatusCode < 200 || resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, fmt.Sprintf("Xero attachment mutation returned %s and may have succeeded", resp.Status), nil)
	}
	if resp.StatusCode >= 400 {
		return InvoiceAttachmentMutationResult{}, decodeAPIError(resp)
	}

	var payload attachmentsResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&payload); err != nil {
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, "Xero attachment mutation response could not be decoded", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, "Xero attachment mutation response contained trailing data", err)
	}
	if len(payload.Attachments) != 1 {
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, fmt.Sprintf("Xero attachment mutation response must contain exactly one attachment; got %d", len(payload.Attachments)), nil)
	}
	attachment := payload.Attachments[0]
	if err := attachmentPayloadValidationError(attachment); err != nil {
		return InvoiceAttachmentMutationResult{}, err
	}
	if strings.TrimSpace(attachment.AttachmentID) == "" {
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, "Xero attachment mutation response did not include an attachment ID", nil)
	}
	if attachment.FileName != request.FileName {
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, "Xero attachment mutation response did not match the expected filename", nil)
	}
	if attachment.ContentLength != request.ContentLength {
		return InvoiceAttachmentMutationResult{}, attachmentMutationUncertain(mutation, "Xero attachment mutation response did not match the uploaded byte count", nil)
	}

	result := InvoiceAttachmentMutationResult{
		Operation:      operation,
		Resource:       request.Resource,
		InvoiceID:      request.InvoiceID,
		TenantID:       request.TenantID,
		Type:           request.Type,
		AttachmentID:   attachment.AttachmentID,
		FileName:       request.FileName,
		ContentType:    request.ContentType,
		Bytes:          request.ContentLength,
		Overwritten:    request.Replace,
		IdempotencyKey: request.IdempotencyKey,
	}
	if strings.EqualFold(request.Resource, "invoice") {
		includeOnline := request.IncludeOnline
		result.IncludeOnline = &includeOnline
	}
	return result, nil
}

// doAttachmentMutation disables redirect following so one invocation can
// dispatch at most one request. The non-replayable request body also prevents
// net/http from retrying a PUT on a reused connection.
func (c *Client) doAttachmentMutation(request *http.Request) (*http.Response, error) {
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client.Do(request)
}

type attachmentMutationRequest struct {
	operation      string
	resource       string
	namespace      string
	tenantID       string
	invoiceID      string
	fileName       string
	idempotencyKey string
	includeOnline  bool
	replace        bool
}

func attachmentMutationUncertain(mutation attachmentMutationRequest, message string, cause error) error {
	metadata := clierrors.Metadata{
		MayHaveSucceeded: true,
		Operation:        mutation.operation,
		Resource:         mutation.resource,
		TenantID:         mutation.tenantID,
		InvoiceID:        mutation.invoiceID,
		FileName:         mutation.fileName,
		IdempotencyKey:   mutation.idempotencyKey,
		RecoveryCommand:  attachmentRecoveryCommand(mutation),
	}
	if cause != nil {
		return clierrors.WrapWithMetadata(clierrors.KindMutationUncertain, message, cause, metadata)
	}
	return clierrors.NewWithMetadata(clierrors.KindMutationUncertain, message, metadata)
}

func attachmentRecoveryCommand(mutation attachmentMutationRequest) string {
	return fmt.Sprintf("xero %s --invoice-id %s --json", mutation.namespace, mutation.invoiceID)
}

func attachmentPayloadValidationError(payload attachmentPayload) error {
	validationErrors := uniqueMessages(appendMessagePayloads(nil, payload.ValidationErrors))
	if !payload.HasErrors && len(validationErrors) == 0 {
		return nil
	}
	return clierrors.NewWithMetadata(
		clierrors.KindXeroAPI,
		appendValidationErrors("Xero could not upload attachment", validationErrors),
		clierrors.Metadata{ValidationErrors: validationErrors},
	)
}
