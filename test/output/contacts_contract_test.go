package output_test

import (
	"bytes"
	"testing"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

const outputContactID = "220ddca8-3144-4085-9a88-2d72c5133734"

func TestWriteJSONContactsListEnvelopeContract(t *testing.T) {
	var buffer bytes.Buffer
	contacts := []xeroapi.Contact{{
		ContactID: outputContactID, ContactNumber: "CRM-1", AccountNumber: "ACME-1",
		ContactStatus: "ACTIVE", Name: "Acme", FirstName: "Alex", LastName: "Morgan",
		CompanyNumber: "COMP-1", EmailAddress: "acme@example.invalid",
		Phones:     []xeroapi.ContactPhone{{PhoneType: "DEFAULT", PhoneNumber: "111", PhoneAreaCode: "2", PhoneCountryCode: "852"}},
		IsSupplier: false, IsCustomer: true, UpdatedAt: "2026-07-11T10:30:00Z",
	}}
	breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: "xero contacts list --json"}}
	if err := output.WriteJSON(&buffer, contacts, "1 contact", breadcrumbs, false); err != nil {
		t.Fatalf("write contacts JSON: %v", err)
	}
	want := "{\n  \"ok\": true,\n  \"data\": [\n    {\n      \"contactId\": \"" + outputContactID + "\",\n      \"contactNumber\": \"CRM-1\",\n      \"accountNumber\": \"ACME-1\",\n      \"contactStatus\": \"ACTIVE\",\n      \"name\": \"Acme\",\n      \"firstName\": \"Alex\",\n      \"lastName\": \"Morgan\",\n      \"companyNumber\": \"COMP-1\",\n      \"emailAddress\": \"acme@example.invalid\",\n      \"phones\": [\n        {\n          \"phoneType\": \"DEFAULT\",\n          \"phoneNumber\": \"111\",\n          \"phoneAreaCode\": \"2\",\n          \"phoneCountryCode\": \"852\"\n        }\n      ],\n      \"isSupplier\": false,\n      \"isCustomer\": true,\n      \"updatedAt\": \"2026-07-11T10:30:00Z\"\n    }\n  ],\n  \"summary\": \"1 contact\",\n  \"breadcrumbs\": [\n    {\n      \"action\": \"show\",\n      \"cmd\": \"xero contacts list --json\"\n    }\n  ]\n}\n"
	if buffer.String() != want {
		t.Fatalf("unexpected contacts envelope:\n%s", buffer.String())
	}
}

func TestWriteJSONContactMutationQuietContract(t *testing.T) {
	var buffer bytes.Buffer
	result := xeroapi.ContactMutationResult{
		Operation: "updated", Resource: "contact", ContactID: outputContactID,
		TenantID: "tenant-1", Name: "Acme", Status: "ACTIVE",
		UpdatedAt: "2026-07-11T10:30:00Z", IdempotencyKey: "contact-key",
	}
	if err := output.WriteJSON(&buffer, result, "contact updated", nil, true); err != nil {
		t.Fatalf("write contact mutation JSON: %v", err)
	}
	want := "{\n  \"operation\": \"updated\",\n  \"resource\": \"contact\",\n  \"contactId\": \"" + outputContactID + "\",\n  \"tenantId\": \"tenant-1\",\n  \"name\": \"Acme\",\n  \"status\": \"ACTIVE\",\n  \"updatedAt\": \"2026-07-11T10:30:00Z\",\n  \"idempotencyKey\": \"contact-key\"\n}\n"
	if buffer.String() != want {
		t.Fatalf("unexpected contact quiet payload:\n%s", buffer.String())
	}
}

func TestWriteContactMutationHumanContract(t *testing.T) {
	var buffer bytes.Buffer
	result := xeroapi.ContactMutationResult{
		Operation: "created", ContactID: outputContactID, TenantID: "tenant-1",
		Name: "Acme", Status: "ACTIVE", IdempotencyKey: "contact-key",
	}
	if err := output.WriteContactMutation(&buffer, result); err != nil {
		t.Fatalf("write contact human output: %v", err)
	}
	want := "Created contact Acme (" + outputContactID + ") for tenant tenant-1 (ACTIVE)\nIdempotency key: contact-key\n"
	if buffer.String() != want {
		t.Fatalf("unexpected contact human output: %q", buffer.String())
	}
}

func TestWriteContactUncertainErrorEnvelopeContract(t *testing.T) {
	var buffer bytes.Buffer
	err := clierrors.NewWithMetadata(clierrors.KindMutationUncertain, "contact response was lost", clierrors.Metadata{
		MayHaveSucceeded: true,
		Operation:        "updated",
		Resource:         "contact",
		TenantID:         "tenant-1",
		ContactID:        outputContactID,
		IdempotencyKey:   "contact-key",
		RecoveryCommand:  "xero contacts list --contact-id " + outputContactID + " --include-archived --json",
	})
	if err := output.WriteErrorJSON(&buffer, err, false); err != nil {
		t.Fatalf("write contact error JSON: %v", err)
	}
	want := "{\n  \"ok\": false,\n  \"error\": {\n    \"kind\": \"MutationUncertainError\",\n    \"message\": \"contact response was lost\",\n    \"exitCode\": 20,\n    \"mayHaveSucceeded\": true,\n    \"operation\": \"updated\",\n    \"resource\": \"contact\",\n    \"tenantId\": \"tenant-1\",\n    \"contactId\": \"" + outputContactID + "\",\n    \"idempotencyKey\": \"contact-key\",\n    \"recoveryCommand\": \"xero contacts list --contact-id " + outputContactID + " --include-archived --json\"\n  }\n}\n"
	if buffer.String() != want {
		t.Fatalf("unexpected contact error envelope:\n%s", buffer.String())
	}
}
