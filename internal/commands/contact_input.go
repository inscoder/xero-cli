package commands

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

type contactCreateInput struct {
	Name          string  `json:"name"`
	ContactNumber *string `json:"contactNumber,omitempty"`
	AccountNumber *string `json:"accountNumber,omitempty"`
	FirstName     *string `json:"firstName,omitempty"`
	LastName      *string `json:"lastName,omitempty"`
	CompanyNumber *string `json:"companyNumber,omitempty"`
	EmailAddress  *string `json:"emailAddress,omitempty"`
	Phone         *string `json:"phone,omitempty"`
}

type contactUpdateInput struct {
	ContactID     string  `json:"contactId"`
	Name          *string `json:"name,omitempty"`
	ContactNumber *string `json:"contactNumber,omitempty"`
	AccountNumber *string `json:"accountNumber,omitempty"`
	FirstName     *string `json:"firstName,omitempty"`
	LastName      *string `json:"lastName,omitempty"`
	CompanyNumber *string `json:"companyNumber,omitempty"`
	EmailAddress  *string `json:"emailAddress,omitempty"`
	ContactStatus *string `json:"contactStatus,omitempty"`
}

func validateContactCreateInput(input contactCreateInput) error {
	if err := validateContactName(input.Name); err != nil {
		return err
	}
	if err := validateContactOptionalFields(input.ContactNumber, input.AccountNumber, input.FirstName, input.LastName, input.CompanyNumber); err != nil {
		return err
	}
	if input.EmailAddress != nil {
		if err := validateContactEmail(*input.EmailAddress, false); err != nil {
			return err
		}
	}
	if input.Phone != nil && utf8.RuneCountInString(*input.Phone) > 50 {
		return clierrors.New(clierrors.KindValidation, "phone must not exceed 50 characters")
	}
	return nil
}

func validateContactUpdateInput(input *contactUpdateInput, confirmArchive bool) error {
	contactID, err := normalizeContactID(input.ContactID, "contactId")
	if err != nil {
		return err
	}
	input.ContactID = contactID
	if !hasContactUpdateFields(*input) {
		return clierrors.New(clierrors.KindValidation, "contact update must contain at least one mutable field")
	}
	if input.Name != nil {
		if err := validateContactName(*input.Name); err != nil {
			return err
		}
	}
	if err := validateContactOptionalFields(input.ContactNumber, input.AccountNumber, input.FirstName, input.LastName, input.CompanyNumber); err != nil {
		return err
	}
	if input.EmailAddress != nil {
		if err := validateContactEmail(*input.EmailAddress, true); err != nil {
			return err
		}
	}
	if input.ContactStatus != nil {
		status := strings.ToUpper(strings.TrimSpace(*input.ContactStatus))
		if status != "ACTIVE" && status != "ARCHIVED" {
			return clierrors.New(clierrors.KindValidation, "contactStatus must be ACTIVE or ARCHIVED")
		}
		input.ContactStatus = &status
	}
	return validateContactArchiveSafety(*input, confirmArchive)
}

func validateContactArchiveSafety(input contactUpdateInput, confirmArchive bool) error {
	archiving := input.ContactStatus != nil && *input.ContactStatus == "ARCHIVED"
	if confirmArchive && !archiving {
		return clierrors.New(clierrors.KindValidation, "--confirm-archive requires contactStatus ARCHIVED")
	}
	if !archiving {
		return nil
	}
	if !confirmArchive {
		return clierrors.New(clierrors.KindValidation, "archiving a contact requires --confirm-archive")
	}
	if contactUpdateFieldCount(input) != 1 {
		return clierrors.New(clierrors.KindValidation, "ARCHIVED must be the only mutable field in the update")
	}
	return nil
}

func validateContactName(value string) error {
	if value == "" || strings.TrimSpace(value) == "" {
		return clierrors.New(clierrors.KindValidation, "name is required")
	}
	if strings.TrimSpace(value) != value {
		return clierrors.New(clierrors.KindValidation, "name must not have leading or trailing whitespace")
	}
	if strings.Contains(value, "  ") {
		return clierrors.New(clierrors.KindValidation, "name must not contain repeated spaces")
	}
	if strings.ContainsAny(value, "<>") {
		return clierrors.New(clierrors.KindValidation, "name must not contain angle brackets")
	}
	if utf8.RuneCountInString(value) > 255 {
		return clierrors.New(clierrors.KindValidation, "name must not exceed 255 characters")
	}
	return nil
}

func validateContactOptionalFields(contactNumber, accountNumber, firstName, lastName, companyNumber *string) error {
	fields := []struct {
		name  string
		value *string
		max   int
	}{
		{name: "contactNumber", value: contactNumber, max: 50},
		{name: "accountNumber", value: accountNumber, max: 50},
		{name: "firstName", value: firstName, max: 255},
		{name: "lastName", value: lastName, max: 255},
		{name: "companyNumber", value: companyNumber, max: 50},
	}
	for _, field := range fields {
		if field.value != nil && utf8.RuneCountInString(*field.value) > field.max {
			return clierrors.New(clierrors.KindValidation, fmt.Sprintf("%s must not exceed %d characters", field.name, field.max))
		}
	}
	return nil
}

func validateContactEmail(value string, allowEmpty bool) error {
	if value == "" && allowEmpty {
		return nil
	}
	if value == "" {
		return clierrors.New(clierrors.KindValidation, "emailAddress must not be empty when present")
	}
	if utf8.RuneCountInString(value) > 255 {
		return clierrors.New(clierrors.KindValidation, "emailAddress must not exceed 255 characters")
	}
	for _, character := range value {
		if character > unicode.MaxASCII || unicode.IsControl(character) || unicode.IsSpace(character) {
			return clierrors.New(clierrors.KindValidation, "emailAddress must be a basic ASCII email address")
		}
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || strings.ContainsAny(value, "<>") {
		return clierrors.New(clierrors.KindValidation, "emailAddress must be a basic ASCII email address")
	}
	return nil
}

func normalizeContactID(value, field string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" || !uuidPattern.MatchString(candidate) {
		return "", clierrors.New(clierrors.KindValidation, field+" must be a valid UUID")
	}
	return strings.ToLower(candidate), nil
}

func hasContactUpdateFields(input contactUpdateInput) bool {
	return contactUpdateFieldCount(input) > 0
}

func contactUpdateFieldCount(input contactUpdateInput) int {
	count := 0
	for _, present := range []bool{
		input.Name != nil,
		input.ContactNumber != nil,
		input.AccountNumber != nil,
		input.FirstName != nil,
		input.LastName != nil,
		input.CompanyNumber != nil,
		input.EmailAddress != nil,
		input.ContactStatus != nil,
	} {
		if present {
			count++
		}
	}
	return count
}

func contactStringPointer(value string) *string {
	return &value
}
