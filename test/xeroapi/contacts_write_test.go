package xeroapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/inscoder/xero-cli/internal/auth"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

func TestCreateContactBuildsExactNonReplayableRequest(t *testing.T) {
	requestCount := 0
	transport := contactRoundTripper(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodPut || request.URL.Path != "/api.xro/2.0/Contacts" || request.URL.RawQuery != "summarizeErrors=true" {
			t.Fatalf("unexpected request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.GetBody != nil {
			t.Fatal("expected a non-replayable request body")
		}
		for header, want := range map[string]string{
			"Authorization":   "Bearer token-123",
			"Xero-tenant-id":  "tenant-1",
			"Accept":          "application/json",
			"Content-Type":    "application/json",
			"Idempotency-Key": "create-key",
		} {
			if got := request.Header.Get(header); got != want {
				t.Fatalf("unexpected %s header: got %q want %q", header, got, want)
			}
		}
		body, _ := io.ReadAll(request.Body)
		wantBody := `{"Contacts":[{"Name":"Acme","ContactNumber":"CRM-1","EmailAddress":"acme@example.invalid","Phones":[{"PhoneType":"DEFAULT","PhoneNumber":"111"}]}]}`
		if string(body) != wantBody {
			t.Fatalf("unexpected request body:\n got %s\nwant %s", body, wantBody)
		}
		return contactResponse(http.StatusOK, `{"Contacts":[{"ContactID":"`+testContactID+`","Name":"Acme","ContactStatus":"ACTIVE","UpdatedDateUTC":"2026-07-11T10:30:00Z"}]}`), nil
	})
	name := "Acme"
	contactNumber := "CRM-1"
	email := "acme@example.invalid"
	phones := []xeroapi.ContactWritePhone{{PhoneType: "DEFAULT", PhoneNumber: "111"}}
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: &http.Client{Transport: transport}})
	result, err := client.CreateContact(context.Background(), auth.TokenSet{AccessToken: "token-123"}, xeroapi.CreateContactRequest{
		TenantID:       "tenant-1",
		IdempotencyKey: "create-key",
		Contact: xeroapi.ContactWrite{
			Name:          &name,
			ContactNumber: &contactNumber,
			EmailAddress:  &email,
			Phones:        &phones,
		},
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	if requestCount != 1 || result.Operation != "created" || result.Resource != "contact" || result.ContactID != testContactID || result.TenantID != "tenant-1" || result.Name != "Acme" || result.Status != "ACTIVE" || result.UpdatedAt != "2026-07-11T10:30:00Z" || result.IdempotencyKey != "create-key" {
		t.Fatalf("unexpected request count/result: count=%d result=%+v", requestCount, result)
	}
}

func TestUpdateContactUsesIDPathWithoutQueryAndOmitsAbsentFields(t *testing.T) {
	requestCount := 0
	transport := contactRoundTripper(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodPost || request.URL.Path != "/api.xro/2.0/Contacts/"+testContactID || request.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		body, _ := io.ReadAll(request.Body)
		wantBody := `{"Contacts":[{"ContactID":"` + testContactID + `","FirstName":"","EmailAddress":""}]}`
		if string(body) != wantBody {
			t.Fatalf("unexpected request body:\n got %s\nwant %s", body, wantBody)
		}
		return contactResponse(http.StatusOK, `{"Contacts":[{"ContactID":"`+testContactID+`","Name":"Acme","ContactStatus":"ACTIVE"}]}`), nil
	})
	empty := ""
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: &http.Client{Transport: transport}})
	result, err := client.UpdateContact(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UpdateContactRequest{
		TenantID:       "tenant-1",
		ContactID:      testContactID,
		IdempotencyKey: "update-key",
		Contact:        xeroapi.ContactWrite{ContactID: testContactID, EmailAddress: &empty, FirstName: &empty},
	})
	if err != nil {
		t.Fatalf("update contact: %v", err)
	}
	if requestCount != 1 || result.Operation != "updated" || result.ContactID != testContactID || result.IdempotencyKey != "update-key" {
		t.Fatalf("unexpected request count/result: count=%d result=%+v", requestCount, result)
	}
}

func TestContactMutationsClassifyUnusableResponsesAsUncertainWithoutRetry(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
	}{
		{name: "redirect", code: http.StatusFound, body: ""},
		{name: "server error", code: http.StatusServiceUnavailable, body: `{"Message":"unavailable"}`},
		{name: "malformed", code: http.StatusOK, body: `{"Contacts":[`},
		{name: "trailing", code: http.StatusOK, body: `{"Contacts":[{"ContactID":"` + testContactID + `"}]} {}`},
		{name: "empty", code: http.StatusOK, body: `{"Contacts":[]}`},
		{name: "multiple", code: http.StatusOK, body: `{"Contacts":[{"ContactID":"` + testContactID + `"},{"ContactID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0"}]}`},
		{name: "missing ID", code: http.StatusOK, body: `{"Contacts":[{"Name":"Acme"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestCount := 0
			transport := contactRoundTripper(func(*http.Request) (*http.Response, error) {
				requestCount++
				return contactResponse(test.code, test.body), nil
			})
			name := "O'Reilly Contacts"
			client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: &http.Client{Transport: transport}})
			_, err := client.CreateContact(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.CreateContactRequest{
				TenantID: "tenant-1", IdempotencyKey: "same-key", Contact: xeroapi.ContactWrite{Name: &name},
			})
			metadata := clierrors.MetadataOf(err)
			if clierrors.KindOf(err) != clierrors.KindMutationUncertain || !metadata.MayHaveSucceeded || metadata.Operation != "created" || metadata.Resource != "contact" || metadata.TenantID != "tenant-1" || metadata.IdempotencyKey != "same-key" || !strings.Contains(metadata.RecoveryCommand, `--search 'O'"'"'Reilly Contacts'`) || !strings.Contains(metadata.RecoveryCommand, "--include-archived") {
				t.Fatalf("unexpected uncertain error: err=%v metadata=%+v", err, metadata)
			}
			if requestCount != 1 {
				t.Fatalf("expected no retry, got %d requests", requestCount)
			}
		})
	}
}

func TestUpdateContactIdentityAndArchiveMismatchesAreUncertain(t *testing.T) {
	archived := "ARCHIVED"
	tests := []struct {
		name string
		body string
	}{
		{name: "wrong ID", body: `{"Contacts":[{"ContactID":"88192a99-cbc5-4a66-bf1a-2f9fea2d36d0","ContactStatus":"ARCHIVED"}]}`},
		{name: "archive not confirmed", body: `{"Contacts":[{"ContactID":"` + testContactID + `","ContactStatus":"ACTIVE"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestCount := 0
			client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: &http.Client{Transport: contactRoundTripper(func(*http.Request) (*http.Response, error) {
				requestCount++
				return contactResponse(http.StatusOK, test.body), nil
			})}})
			_, err := client.UpdateContact(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UpdateContactRequest{
				TenantID: "tenant-1", ContactID: testContactID, IdempotencyKey: "archive-key", Contact: xeroapi.ContactWrite{ContactID: testContactID, ContactStatus: &archived},
			})
			metadata := clierrors.MetadataOf(err)
			if clierrors.KindOf(err) != clierrors.KindMutationUncertain || metadata.ContactID != testContactID || metadata.RecoveryCommand != "xero contacts list --contact-id "+testContactID+" --include-archived --json" || requestCount != 1 {
				t.Fatalf("unexpected uncertain update: err=%v metadata=%+v requests=%d", err, metadata, requestCount)
			}
		})
	}
}

func TestContactMutationTransportFailureIsUncertainAndNotRetried(t *testing.T) {
	requestCount := 0
	transportError := errors.New("connection reset")
	client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: &http.Client{Transport: contactRoundTripper(func(*http.Request) (*http.Response, error) {
		requestCount++
		return nil, transportError
	})}})
	_, err := client.UpdateContact(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.UpdateContactRequest{
		TenantID: "tenant-1", ContactID: testContactID, IdempotencyKey: "update-key", Contact: xeroapi.ContactWrite{ContactID: testContactID},
	})
	if clierrors.KindOf(err) != clierrors.KindMutationUncertain || !errors.Is(err, transportError) || requestCount != 1 {
		t.Fatalf("unexpected transport outcome: err=%v requests=%d", err, requestCount)
	}
}

func TestContactMutationValidationFailuresAreKnown(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
		want string
	}{
		{name: "http validation", code: http.StatusBadRequest, body: `{"Message":"validation","Contacts":[{"ValidationErrors":[{"Message":"bad email"}]}]}`, want: "bad email"},
		{name: "semantic validation", code: http.StatusOK, body: `{"Contacts":[{"ContactID":"` + testContactID + `","HasValidationErrors":true,"ValidationErrors":[{"Message":"duplicate contact"}]}]}`, want: "duplicate contact"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestCount := 0
			name := "Acme"
			client := xeroapi.NewClient(appconfig.Settings{}, xeroapi.ClientOptions{BaseURL: "https://xero.example", HTTPClient: &http.Client{Transport: contactRoundTripper(func(*http.Request) (*http.Response, error) {
				requestCount++
				return contactResponse(test.code, test.body), nil
			})}})
			_, err := client.CreateContact(context.Background(), auth.TokenSet{AccessToken: "token"}, xeroapi.CreateContactRequest{TenantID: "tenant-1", IdempotencyKey: "key", Contact: xeroapi.ContactWrite{Name: &name}})
			if clierrors.KindOf(err) != clierrors.KindXeroAPI || clierrors.MetadataOf(err).MayHaveSucceeded || !strings.Contains(err.Error(), test.want) || requestCount != 1 {
				t.Fatalf("unexpected known failure: err=%v metadata=%+v requests=%d", err, clierrors.MetadataOf(err), requestCount)
			}
		})
	}
}

func TestContactWriteDTOContainsOnlyConservativeFields(t *testing.T) {
	email := ""
	encoded, err := json.Marshal(xeroapi.ContactWrite{ContactID: testContactID, EmailAddress: &email})
	if err != nil {
		t.Fatalf("marshal contact write: %v", err)
	}
	if got, want := string(encoded), `{"ContactID":"`+testContactID+`","EmailAddress":""}`; got != want {
		t.Fatalf("unexpected write DTO: got %s want %s", got, want)
	}
	for _, forbidden := range []string{"BankAccount", "TaxNumber", "Balances", "Attachments", "ContactPersons", "Addresses"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("write DTO exposed %s: %s", forbidden, encoded)
		}
	}
}
