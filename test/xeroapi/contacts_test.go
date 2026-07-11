package xeroapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

const testContactID = "220ddca8-3144-4085-9a88-2d72c5133734"

type contactRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip contactRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func contactResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestListContactsBuildsExactRequestAndNormalizesConservativeModel(t *testing.T) {
	requestCount := 0
	transport := contactRoundTripper(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodGet || request.URL.Path != "/api.xro/2.0/Contacts" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		wantQuery := map[string]string{
			"searchTerm":      "Acme & Sons",
			"IDs":             testContactID + ",88192a99-cbc5-4a66-bf1a-2f9fea2d36d0",
			"page":            "2",
			"pageSize":        "50",
			"includeArchived": "true",
			"summaryOnly":     "true",
			"where":           `ContactStatus=="ACTIVE"`,
			"order":           "Name DESC",
		}
		for key, want := range wantQuery {
			if got := query.Get(key); got != want {
				t.Fatalf("unexpected %s query: got %q want %q", key, got, want)
			}
		}
		if len(query) != len(wantQuery) {
			t.Fatalf("unexpected query parameters: %v", query)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		if got := request.Header.Get("Xero-tenant-id"); got != "tenant-1" {
			t.Fatalf("unexpected tenant: %q", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected accept: %q", got)
		}
		if got := request.Header.Get("If-Modified-Since"); got != "Sun, 01 Mar 2026 00:00:00 UTC" {
			t.Fatalf("unexpected modified-since: %q", got)
		}
		return contactResponse(http.StatusOK, `{
			"Contacts":[{
				"ContactID":"`+testContactID+`",
				"ContactNumber":"CRM-1",
				"AccountNumber":"ACME-1",
				"ContactStatus":"ACTIVE",
				"Name":"Acme & Sons",
				"FirstName":"Alex",
				"LastName":"Morgan",
				"CompanyNumber":"COMP-1",
				"EmailAddress":"alex@example.invalid",
				"Phones":[{"PhoneType":"MOBILE","PhoneNumber":"222"},{"PhoneType":"DEFAULT","PhoneNumber":"111","PhoneAreaCode":"2","PhoneCountryCode":"852"}],
				"IsSupplier":true,
				"IsCustomer":false,
				"UpdatedDateUTC":"/Date(1773059400000+0000)/",
				"BankAccountDetails":"secret",
				"TaxNumber":"secret",
				"Balances":{"AccountsReceivable":{"Outstanding":99}}
			}],
			"pagination":{"page":2,"pageSize":50,"pageCount":3,"itemCount":101}
		}`), nil
	})
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{
		BaseURL:    "https://xero.example",
		HTTPClient: &http.Client{Transport: transport},
	})
	result, err := client.ListContacts(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.ListContactsRequest{
		TenantID:        "tenant-1",
		Search:          "Acme & Sons",
		ContactIDs:      []string{testContactID, "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"},
		Page:            2,
		PageSize:        50,
		IncludeArchived: true,
		SummaryOnly:     true,
		Since:           "2026-03-01",
		Where:           `ContactStatus=="ACTIVE"`,
		Order:           "Name DESC",
	})
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected one request, got %d", requestCount)
	}
	if len(result.Contacts) != 1 {
		t.Fatalf("expected one contact, got %+v", result)
	}
	contact := result.Contacts[0]
	if contact.ContactID != testContactID || contact.Name != "Acme & Sons" || contact.ContactStatus != "ACTIVE" || len(contact.Phones) != 2 {
		t.Fatalf("unexpected normalized contact: %+v", contact)
	}
	if contact.UpdatedAt != time.UnixMilli(1773059400000).UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected updated timestamp: %q", contact.UpdatedAt)
	}
	if result.Pagination == nil || result.Pagination.Page != 2 || result.Pagination.PageSize != 50 || result.Pagination.PageCount != 3 || result.Pagination.ItemCount != 101 {
		t.Fatalf("unexpected pagination: %+v", result.Pagination)
	}
	encoded, err := json.Marshal(contact)
	if err != nil {
		t.Fatalf("marshal contact: %v", err)
	}
	if strings.Contains(string(encoded), "BankAccount") || strings.Contains(string(encoded), "TaxNumber") || strings.Contains(string(encoded), "Balances") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("sensitive response fields leaked: %s", encoded)
	}
}

func TestListContactsTreatsNotModifiedAsEmptySuccess(t *testing.T) {
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{
		BaseURL: "https://xero.example",
		HTTPClient: &http.Client{Transport: contactRoundTripper(func(*http.Request) (*http.Response, error) {
			return contactResponse(http.StatusNotModified, ""), nil
		})},
	})
	result, err := client.ListContacts(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.ListContactsRequest{TenantID: "tenant-1", Page: 1})
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if result.Contacts == nil || len(result.Contacts) != 0 || result.Pagination != nil {
		t.Fatalf("expected explicit empty result, got %+v", result)
	}
}

func TestListContactsRejectsInvalidSinceBeforeDispatch(t *testing.T) {
	requestCount := 0
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{
		BaseURL: "https://xero.example",
		HTTPClient: &http.Client{Transport: contactRoundTripper(func(*http.Request) (*http.Response, error) {
			requestCount++
			return contactResponse(http.StatusOK, `{"Contacts":[]}`), nil
		})},
	})
	_, err := client.ListContacts(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.ListContactsRequest{TenantID: "tenant-1", Since: "March 1"})
	if clierrors.KindOf(err) != clierrors.KindValidation || requestCount != 0 {
		t.Fatalf("expected pre-dispatch validation, got err=%v requests=%d", err, requestCount)
	}
}

func TestGetContactRequiresExactlyMatchingIdentity(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "valid", body: `{"Contacts":[{"ContactID":"` + testContactID + `","Name":"Acme"}]}`},
		{name: "empty", body: `{"Contacts":[]}`, wantError: true},
		{name: "multiple", body: `{"Contacts":[{"ContactID":"` + testContactID + `"},{"ContactID":"other"}]}`, wantError: true},
		{name: "missing ID", body: `{"Contacts":[{"Name":"Acme"}]}`, wantError: true},
		{name: "mismatched ID", body: `{"Contacts":[{"ContactID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"}]}`, wantError: true},
		{name: "validation", body: `{"Contacts":[{"ContactID":"` + testContactID + `","HasValidationErrors":true,"ValidationErrors":[{"Message":"bad contact"}]}]}`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{
				BaseURL: "https://xero.example",
				HTTPClient: &http.Client{Transport: contactRoundTripper(func(request *http.Request) (*http.Response, error) {
					if request.Method != http.MethodGet || request.URL.Path != "/api.xro/2.0/Contacts/"+testContactID || request.URL.RawQuery != "" {
						t.Fatalf("unexpected request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
					}
					return contactResponse(http.StatusOK, test.body), nil
				})},
			})
			contact, err := client.GetContact(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.GetContactRequest{TenantID: "tenant-1", ContactID: testContactID})
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error, got contact %+v", contact)
				}
				return
			}
			if err != nil || contact.ContactID != testContactID {
				t.Fatalf("unexpected result: contact=%+v err=%v", contact, err)
			}
		})
	}
}

func TestContactErrorsCollectNestedContactValidationMessages(t *testing.T) {
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{
		BaseURL: "https://xero.example",
		HTTPClient: &http.Client{Transport: contactRoundTripper(func(*http.Request) (*http.Response, error) {
			return contactResponse(http.StatusBadRequest, `{"Message":"validation","Contacts":[{"ValidationErrors":[{"Message":"bad email"},{"Message":"duplicate"}]},{"ValidationErrors":[{"Message":"duplicate"},{"Message":"bad name"}]}]}`), nil
		})},
	})
	_, err := client.ListContacts(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.ListContactsRequest{TenantID: "tenant-1", Page: 1})
	if clierrors.KindOf(err) != clierrors.KindXeroAPI {
		t.Fatalf("expected Xero API error, got %v", err)
	}
	want := []string{"bad email", "duplicate", "bad name"}
	got := clierrors.MetadataOf(err).ValidationErrors
	if len(got) != len(want) {
		t.Fatalf("unexpected validation messages: got=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected validation messages: got=%v want=%v", got, want)
		}
	}
}
