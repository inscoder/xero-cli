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

// ContactWrite is a conservative, write-only Contact representation. It is
// intentionally separate from Contact so sensitive or response-only fields
// cannot be sent back to Xero by accident.
type ContactWrite struct {
	ContactID     string               `json:"ContactID,omitempty"`
	Name          *string              `json:"Name,omitempty"`
	ContactNumber *string              `json:"ContactNumber,omitempty"`
	AccountNumber *string              `json:"AccountNumber,omitempty"`
	FirstName     *string              `json:"FirstName,omitempty"`
	LastName      *string              `json:"LastName,omitempty"`
	CompanyNumber *string              `json:"CompanyNumber,omitempty"`
	EmailAddress  *string              `json:"EmailAddress,omitempty"`
	ContactStatus *string              `json:"ContactStatus,omitempty"`
	Phones        *[]ContactWritePhone `json:"Phones,omitempty"`
}

type ContactWritePhone struct {
	PhoneType   string `json:"PhoneType"`
	PhoneNumber string `json:"PhoneNumber"`
}

type CreateContactRequest struct {
	TenantID       string
	IdempotencyKey string
	Contact        ContactWrite
}

type UpdateContactRequest struct {
	TenantID       string
	ContactID      string
	IdempotencyKey string
	Contact        ContactWrite
}

type ContactMutationResult struct {
	Operation      string `json:"operation"`
	Resource       string `json:"resource"`
	ContactID      string `json:"contactId"`
	TenantID       string `json:"tenantId"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type contactWriteEnvelope struct {
	Contacts []ContactWrite `json:"Contacts"`
}

type contactMutationRequest struct {
	method         string
	path           string
	operation      string
	tenantID       string
	contactID      string
	idempotencyKey string
	contact        ContactWrite
}

func (c *Client) CreateContact(ctx context.Context, token auth.TokenSet, request CreateContactRequest) (ContactMutationResult, error) {
	return c.mutateContact(ctx, token, contactMutationRequest{
		method:         http.MethodPut,
		path:           "/api.xro/2.0/Contacts?summarizeErrors=true",
		operation:      "created",
		tenantID:       request.TenantID,
		idempotencyKey: request.IdempotencyKey,
		contact:        request.Contact,
	})
}

func (c *Client) UpdateContact(ctx context.Context, token auth.TokenSet, request UpdateContactRequest) (ContactMutationResult, error) {
	return c.mutateContact(ctx, token, contactMutationRequest{
		method:         http.MethodPost,
		path:           "/api.xro/2.0/Contacts/" + url.PathEscape(request.ContactID),
		operation:      "updated",
		tenantID:       request.TenantID,
		contactID:      request.ContactID,
		idempotencyKey: request.IdempotencyKey,
		contact:        request.Contact,
	})
}

func (c *Client) mutateContact(ctx context.Context, token auth.TokenSet, mutation contactMutationRequest) (ContactMutationResult, error) {
	body, err := json.Marshal(contactWriteEnvelope{Contacts: []ContactWrite{mutation.contact}})
	if err != nil {
		return ContactMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "encode Xero contact mutation", err)
	}
	endpoint, err := url.Parse(c.baseURL + mutation.path)
	if err != nil {
		return ContactMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero contact mutation URL", err)
	}
	// io.NopCloser prevents net/http from setting GetBody, making the request
	// non-replayable. Combined with redirects disabled below, one command can
	// dispatch at most one contact mutation.
	req, err := http.NewRequestWithContext(ctx, mutation.method, endpoint.String(), io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return ContactMutationResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero contact mutation request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xero-tenant-id", mutation.tenantID)
	req.Header.Set("Idempotency-Key", mutation.idempotencyKey)

	resp, err := c.doContactMutation(req)
	if err != nil {
		return ContactMutationResult{}, contactMutationUncertain(mutation, "Xero contact mutation was dispatched but its outcome could not be confirmed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode < 200 || resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return ContactMutationResult{}, contactMutationUncertain(mutation, fmt.Sprintf("Xero contact mutation returned %s and may have succeeded", resp.Status), nil)
	}
	if resp.StatusCode >= 400 {
		return ContactMutationResult{}, decodeAPIError(resp)
	}

	var payload contactsResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&payload); err != nil {
		return ContactMutationResult{}, contactMutationUncertain(mutation, "Xero contact mutation response could not be decoded", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return ContactMutationResult{}, contactMutationUncertain(mutation, "Xero contact mutation response contained trailing data", err)
	}
	if len(payload.Contacts) != 1 {
		return ContactMutationResult{}, contactMutationUncertain(mutation, fmt.Sprintf("Xero contact mutation response must contain exactly one contact; got %d", len(payload.Contacts)), nil)
	}
	contact := payload.Contacts[0]
	if err := contactPayloadValidationError(contact, strings.TrimSuffix(mutation.operation, "d")+" contact"); err != nil {
		return ContactMutationResult{}, err
	}
	if strings.TrimSpace(contact.ContactID) == "" || mutation.contactID != "" && !strings.EqualFold(contact.ContactID, mutation.contactID) {
		return ContactMutationResult{}, contactMutationUncertain(mutation, "Xero contact mutation response did not match the expected contact ID", nil)
	}
	if mutation.contactID == "" {
		mutation.contactID = contact.ContactID
	}
	if mutation.contact.ContactStatus != nil && *mutation.contact.ContactStatus == "ARCHIVED" && contact.ContactStatus != "ARCHIVED" {
		return ContactMutationResult{}, contactMutationUncertain(mutation, "Xero contact archive response did not confirm ARCHIVED status", nil)
	}

	return ContactMutationResult{
		Operation:      mutation.operation,
		Resource:       "contact",
		ContactID:      contact.ContactID,
		TenantID:       mutation.tenantID,
		Name:           contact.Name,
		Status:         contact.ContactStatus,
		UpdatedAt:      normalizeTimestamp(contact.UpdatedDateUTC),
		IdempotencyKey: mutation.idempotencyKey,
	}, nil
}

func (c *Client) doContactMutation(request *http.Request) (*http.Response, error) {
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client.Do(request)
}

func contactMutationUncertain(mutation contactMutationRequest, message string, cause error) error {
	if mutation.contactID == "" && mutation.contact.Name != nil && strings.TrimSpace(*mutation.contact.Name) != "" {
		message += "; verify search results manually because contact names are not a durable identity"
	}
	metadata := clierrors.Metadata{
		MayHaveSucceeded: true,
		Operation:        mutation.operation,
		Resource:         "contact",
		TenantID:         mutation.tenantID,
		ContactID:        mutation.contactID,
		IdempotencyKey:   mutation.idempotencyKey,
		RecoveryCommand:  contactRecoveryCommand(mutation),
	}
	if cause != nil {
		return clierrors.WrapWithMetadata(clierrors.KindMutationUncertain, message, cause, metadata)
	}
	return clierrors.NewWithMetadata(clierrors.KindMutationUncertain, message, metadata)
}

func contactRecoveryCommand(mutation contactMutationRequest) string {
	if mutation.contactID != "" {
		return fmt.Sprintf("xero contacts list --contact-id %s --include-archived --json", mutation.contactID)
	}
	if mutation.contact.Name != nil && strings.TrimSpace(*mutation.contact.Name) != "" {
		return fmt.Sprintf("xero contacts list --search %s --include-archived --json", shellSingleQuote(*mutation.contact.Name))
	}
	return "xero contacts list --include-archived --order 'UpdatedDateUTC DESC' --page-size 10 --json"
}
