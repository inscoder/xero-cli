package xeroapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

type ContactClient interface {
	ListContacts(context.Context, auth.TokenSet, ListContactsRequest) (ContactListResult, error)
	GetContact(context.Context, auth.TokenSet, GetContactRequest) (Contact, error)
	CreateContact(context.Context, auth.TokenSet, CreateContactRequest) (ContactMutationResult, error)
	UpdateContact(context.Context, auth.TokenSet, UpdateContactRequest) (ContactMutationResult, error)
}

type ListContactsRequest struct {
	TenantID        string
	Search          string
	ContactIDs      []string
	Page            int
	PageSize        int
	IncludeArchived bool
	SummaryOnly     bool
	Since           string
	Where           string
	Order           string
}

type GetContactRequest struct {
	TenantID  string
	ContactID string
}

type ContactListResult struct {
	Contacts   []Contact
	Pagination *ContactPagination
}

type Contact struct {
	ContactID     string         `json:"contactId"`
	ContactNumber string         `json:"contactNumber"`
	AccountNumber string         `json:"accountNumber"`
	ContactStatus string         `json:"contactStatus"`
	Name          string         `json:"name"`
	FirstName     string         `json:"firstName"`
	LastName      string         `json:"lastName"`
	CompanyNumber string         `json:"companyNumber"`
	EmailAddress  string         `json:"emailAddress"`
	Phones        []ContactPhone `json:"phones"`
	IsSupplier    bool           `json:"isSupplier"`
	IsCustomer    bool           `json:"isCustomer"`
	UpdatedAt     string         `json:"updatedAt"`
}

type ContactPhone struct {
	PhoneType        string `json:"phoneType"`
	PhoneNumber      string `json:"phoneNumber"`
	PhoneAreaCode    string `json:"phoneAreaCode"`
	PhoneCountryCode string `json:"phoneCountryCode"`
}

type ContactPagination struct {
	Page      int `json:"page"`
	PageSize  int `json:"pageSize"`
	PageCount int `json:"pageCount"`
	ItemCount int `json:"itemCount"`
}

type contactsResponse struct {
	Contacts   []contactResponsePayload  `json:"Contacts"`
	Pagination *contactPaginationPayload `json:"pagination"`
}

type contactResponsePayload struct {
	ContactID           string                `json:"ContactID"`
	ContactNumber       string                `json:"ContactNumber"`
	AccountNumber       string                `json:"AccountNumber"`
	ContactStatus       string                `json:"ContactStatus"`
	Name                string                `json:"Name"`
	FirstName           string                `json:"FirstName"`
	LastName            string                `json:"LastName"`
	CompanyNumber       string                `json:"CompanyNumber"`
	EmailAddress        string                `json:"EmailAddress"`
	Phones              []contactPhonePayload `json:"Phones"`
	IsSupplier          bool                  `json:"IsSupplier"`
	IsCustomer          bool                  `json:"IsCustomer"`
	UpdatedDateUTC      string                `json:"UpdatedDateUTC"`
	HasValidationErrors bool                  `json:"HasValidationErrors"`
	ValidationErrors    []messagePayload      `json:"ValidationErrors"`
}

type contactPhonePayload struct {
	PhoneType        string `json:"PhoneType"`
	PhoneNumber      string `json:"PhoneNumber"`
	PhoneAreaCode    string `json:"PhoneAreaCode"`
	PhoneCountryCode string `json:"PhoneCountryCode"`
}

type contactPaginationPayload struct {
	Page      int `json:"page"`
	PageSize  int `json:"pageSize"`
	PageCount int `json:"pageCount"`
	ItemCount int `json:"itemCount"`
}

func (c *Client) ListContacts(ctx context.Context, token auth.TokenSet, request ListContactsRequest) (ContactListResult, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Contacts")
	if err != nil {
		return ContactListResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero contacts URL", err)
	}
	query := endpoint.Query()
	if request.Search != "" {
		query.Set("searchTerm", request.Search)
	}
	if len(request.ContactIDs) > 0 {
		query.Set("IDs", strings.Join(request.ContactIDs, ","))
	}
	if request.Page > 0 {
		query.Set("page", strconv.Itoa(request.Page))
	}
	if request.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(request.PageSize))
	}
	if request.IncludeArchived {
		query.Set("includeArchived", "true")
	}
	if request.SummaryOnly {
		query.Set("summaryOnly", "true")
	}
	if request.Where != "" {
		query.Set("where", request.Where)
	}
	if request.Order != "" {
		query.Set("order", request.Order)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ContactListResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero contacts request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)
	if request.Since != "" {
		since, err := time.Parse("2006-01-02", request.Since)
		if err != nil {
			return ContactListResult{}, clierrors.Wrap(clierrors.KindValidation, "parse --since date", err)
		}
		req.Header.Set("If-Modified-Since", since.UTC().Format(time.RFC1123))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ContactListResult{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero contacts request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return ContactListResult{Contacts: []Contact{}}, nil
	}
	if resp.StatusCode >= 400 {
		return ContactListResult{}, decodeAPIError(resp)
	}

	var payload contactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ContactListResult{}, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero contacts response", err)
	}
	return normalizeContactList(payload), nil
}

func (c *Client) GetContact(ctx context.Context, token auth.TokenSet, request GetContactRequest) (Contact, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Contacts/" + url.PathEscape(request.ContactID))
	if err != nil {
		return Contact{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero contact URL", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Contact{}, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero contact request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Contact{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero contact request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Contact{}, decodeAPIError(resp)
	}

	var payload contactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Contact{}, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero contact response", err)
	}
	if len(payload.Contacts) != 1 {
		return Contact{}, clierrors.New(clierrors.KindXeroRequest, fmt.Sprintf("Xero contact response must contain exactly one contact; got %d", len(payload.Contacts)))
	}
	item := payload.Contacts[0]
	if strings.TrimSpace(item.ContactID) == "" || !strings.EqualFold(item.ContactID, request.ContactID) {
		return Contact{}, clierrors.New(clierrors.KindXeroRequest, "Xero contact response did not match the requested contact ID")
	}
	if err := contactPayloadValidationError(item, "get contact"); err != nil {
		return Contact{}, err
	}
	return normalizeContact(item), nil
}

func normalizeContactList(payload contactsResponse) ContactListResult {
	contacts := make([]Contact, 0, len(payload.Contacts))
	for _, item := range payload.Contacts {
		contacts = append(contacts, normalizeContact(item))
	}
	result := ContactListResult{Contacts: contacts}
	if payload.Pagination != nil {
		result.Pagination = &ContactPagination{
			Page:      payload.Pagination.Page,
			PageSize:  payload.Pagination.PageSize,
			PageCount: payload.Pagination.PageCount,
			ItemCount: payload.Pagination.ItemCount,
		}
	}
	return result
}

func normalizeContact(item contactResponsePayload) Contact {
	phones := make([]ContactPhone, 0, len(item.Phones))
	for _, phone := range item.Phones {
		phones = append(phones, ContactPhone{
			PhoneType:        phone.PhoneType,
			PhoneNumber:      phone.PhoneNumber,
			PhoneAreaCode:    phone.PhoneAreaCode,
			PhoneCountryCode: phone.PhoneCountryCode,
		})
	}
	return Contact{
		ContactID:     item.ContactID,
		ContactNumber: item.ContactNumber,
		AccountNumber: item.AccountNumber,
		ContactStatus: item.ContactStatus,
		Name:          item.Name,
		FirstName:     item.FirstName,
		LastName:      item.LastName,
		CompanyNumber: item.CompanyNumber,
		EmailAddress:  item.EmailAddress,
		Phones:        phones,
		IsSupplier:    item.IsSupplier,
		IsCustomer:    item.IsCustomer,
		UpdatedAt:     normalizeTimestamp(item.UpdatedDateUTC),
	}
}

func contactPayloadValidationError(payload contactResponsePayload, action string) error {
	validationErrors := uniqueMessages(appendMessagePayloads(nil, payload.ValidationErrors))
	if !payload.HasValidationErrors && len(validationErrors) == 0 {
		return nil
	}
	message := "Xero reported contact validation errors"
	if strings.TrimSpace(action) != "" {
		message = "Xero could not " + action
	}
	return clierrors.NewWithMetadata(clierrors.KindXeroAPI, appendValidationErrors(message, validationErrors), clierrors.Metadata{
		ValidationErrors: validationErrors,
	})
}
