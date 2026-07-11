package commands

import (
	"strings"
	"testing"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

const contactInputTestID = "220ddca8-3144-4085-9a88-2d72c5133734"

func TestValidateContactCreateInput(t *testing.T) {
	validEmail := "acme@example.invalid"
	validPhone := "+1234567890"
	if err := validateContactCreateInput(contactCreateInput{Name: "Acme Corp", EmailAddress: &validEmail, Phone: &validPhone}); err != nil {
		t.Fatalf("validate contact create: %v", err)
	}

	tests := []struct {
		name  string
		input contactCreateInput
		match string
	}{
		{name: "missing name", input: contactCreateInput{}, match: "name"},
		{name: "leading whitespace", input: contactCreateInput{Name: " Acme"}, match: "whitespace"},
		{name: "trailing whitespace", input: contactCreateInput{Name: "Acme "}, match: "whitespace"},
		{name: "repeated spaces", input: contactCreateInput{Name: "Acme  Corp"}, match: "repeated"},
		{name: "angle bracket", input: contactCreateInput{Name: "Acme <Corp>"}, match: "angle"},
		{name: "long name", input: contactCreateInput{Name: strings.Repeat("x", 256)}, match: "255"},
		{name: "bad email", input: contactCreateInput{Name: "Acme", EmailAddress: contactStringPointer("not-an-email")}, match: "emailAddress"},
		{name: "unicode email", input: contactCreateInput{Name: "Acme", EmailAddress: contactStringPointer("té@example.invalid")}, match: "ASCII"},
		{name: "long contact number", input: contactCreateInput{Name: "Acme", ContactNumber: contactStringPointer(strings.Repeat("x", 51))}, match: "contactNumber"},
		{name: "long phone", input: contactCreateInput{Name: "Acme", Phone: contactStringPointer(strings.Repeat("1", 51))}, match: "phone"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateContactCreateInput(test.input)
			if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("expected validation error containing %q, got %v", test.match, err)
			}
		})
	}
}

func TestValidateContactUpdateInputPreservesPresence(t *testing.T) {
	empty := ""
	input := contactUpdateInput{ContactID: strings.ToUpper(contactInputTestID), EmailAddress: &empty, FirstName: &empty}
	if err := validateContactUpdateInput(&input, false); err != nil {
		t.Fatalf("validate contact update: %v", err)
	}
	if input.ContactID != contactInputTestID || input.EmailAddress == nil || *input.EmailAddress != "" || input.FirstName == nil || *input.FirstName != "" {
		t.Fatalf("presence or identity was not preserved: %+v", input)
	}
	if input.Name != nil || input.ContactNumber != nil || input.ContactStatus != nil {
		t.Fatalf("omitted fields became present: %+v", input)
	}
}

func TestValidateContactUpdateArchiveSafety(t *testing.T) {
	archived := "ARCHIVED"
	active := "ACTIVE"
	name := "Acme"
	tests := []struct {
		name    string
		input   contactUpdateInput
		confirm bool
		ok      bool
		match   string
	}{
		{name: "confirmed archive", input: contactUpdateInput{ContactID: contactInputTestID, ContactStatus: &archived}, confirm: true, ok: true},
		{name: "missing confirmation", input: contactUpdateInput{ContactID: contactInputTestID, ContactStatus: &archived}, match: "confirm"},
		{name: "archive with another field", input: contactUpdateInput{ContactID: contactInputTestID, ContactStatus: &archived, Name: &name}, confirm: true, match: "only mutable"},
		{name: "confirmation without archive", input: contactUpdateInput{ContactID: contactInputTestID, ContactStatus: &active}, confirm: true, match: "requires"},
		{name: "GDPR request", input: contactUpdateInput{ContactID: contactInputTestID, ContactStatus: contactStringPointer("GDPRREQUEST")}, match: "ACTIVE or ARCHIVED"},
		{name: "no mutable field", input: contactUpdateInput{ContactID: contactInputTestID}, match: "mutable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := test.input
			err := validateContactUpdateInput(&input, test.confirm)
			if test.ok {
				if err != nil {
					t.Fatalf("expected valid input, got %v", err)
				}
				return
			}
			if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("expected validation error containing %q, got %v", test.match, err)
			}
		})
	}
}

func TestContactCreateJSONRejectsOwnedAndResponseFields(t *testing.T) {
	tests := []struct {
		name    string
		content string
		match   string
	}{
		{name: "contact ID", content: `{"name":"Acme","contactId":"` + contactInputTestID + `"}`, match: "contactId"},
		{name: "contact status", content: `{"name":"Acme","contactStatus":"ACTIVE"}`, match: "contactStatus"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var input contactCreateInput
			err := decodeJSONInput("-", strings.NewReader(test.content), false, &input)
			if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("expected strict create rejection containing %q, got %v", test.match, err)
			}
		})
	}
}

func TestContactUpdateJSONRejectsUnsupportedShapes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		match   string
	}{
		{name: "phone", content: `{"contactId":"` + contactInputTestID + `","phone":"111"}`, match: "phone"},
		{name: "phones collection", content: `{"contactId":"` + contactInputTestID + `","phones":[]}`, match: "phones"},
		{name: "addresses collection", content: `{"contactId":"` + contactInputTestID + `","addresses":[]}`, match: "addresses"},
		{name: "contact persons collection", content: `{"contactId":"` + contactInputTestID + `","contactPersons":[]}`, match: "contactPersons"},
		{name: "wrong case", content: `{"ContactID":"` + contactInputTestID + `","name":"Acme"}`, match: "ContactID"},
		{name: "null", content: `{"contactId":"` + contactInputTestID + `","name":null}`, match: "null"},
		{name: "duplicate", content: `{"contactId":"` + contactInputTestID + `","contactId":"` + contactInputTestID + `","name":"Acme"}`, match: "duplicate"},
		{name: "wrapper", content: `{"Contacts":[{"ContactID":"` + contactInputTestID + `"}]}`, match: "Contacts"},
		{name: "array", content: `[{"contactId":"` + contactInputTestID + `","name":"Acme"}]`, match: "one object"},
		{name: "trailing", content: `{"contactId":"` + contactInputTestID + `","name":"Acme"} {}`, match: "trailing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var input contactUpdateInput
			err := decodeJSONInput("-", strings.NewReader(test.content), false, &input)
			if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("expected strict update rejection containing %q, got %v", test.match, err)
			}
		})
	}
}

func TestContactUpdateJSONPreservesAndNormalizesFileIdentity(t *testing.T) {
	content := `{"contactId":"` + strings.ToUpper(contactInputTestID) + `","emailAddress":""}`
	var input contactUpdateInput
	if err := decodeJSONInput("-", strings.NewReader(content), false, &input); err != nil {
		t.Fatalf("decode update input: %v", err)
	}
	if input.ContactID != strings.ToUpper(contactInputTestID) || input.EmailAddress == nil || *input.EmailAddress != "" {
		t.Fatalf("file identity or explicit empty value was not decoded faithfully: %+v", input)
	}
	if err := validateContactUpdateInput(&input, false); err != nil {
		t.Fatalf("validate update input: %v", err)
	}
	if input.ContactID != contactInputTestID {
		t.Fatalf("expected normalized contact identity, got %q", input.ContactID)
	}
}
