package commands

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

const (
	invoiceTypeSales    = "ACCREC"
	invoiceTypePurchase = "ACCPAY"
)

type invoiceCreateInput struct {
	ContactID           string                  `json:"contactId"`
	Date                *string                 `json:"date,omitempty"`
	DueDate             *string                 `json:"dueDate,omitempty"`
	LineAmountTypes     *string                 `json:"lineAmountTypes,omitempty"`
	InvoiceNumber       *string                 `json:"invoiceNumber,omitempty"`
	Reference           *string                 `json:"reference,omitempty"`
	BrandingThemeID     *string                 `json:"brandingThemeId,omitempty"`
	URL                 *string                 `json:"url,omitempty"`
	CurrencyCode        *string                 `json:"currencyCode,omitempty"`
	CurrencyRate        *json.Number            `json:"currencyRate,omitempty"`
	Status              *string                 `json:"status,omitempty"`
	SentToContact       *bool                   `json:"sentToContact,omitempty"`
	ExpectedPaymentDate *string                 `json:"expectedPaymentDate,omitempty"`
	PlannedPaymentDate  *string                 `json:"plannedPaymentDate,omitempty"`
	LineItems           *[]invoiceLineItemInput `json:"lineItems"`
}

type invoiceUpdateInput struct {
	ContactID           *string                 `json:"contactId,omitempty"`
	Date                *string                 `json:"date,omitempty"`
	DueDate             *string                 `json:"dueDate,omitempty"`
	LineAmountTypes     *string                 `json:"lineAmountTypes,omitempty"`
	InvoiceNumber       *string                 `json:"invoiceNumber,omitempty"`
	Reference           *string                 `json:"reference,omitempty"`
	BrandingThemeID     *string                 `json:"brandingThemeId,omitempty"`
	URL                 *string                 `json:"url,omitempty"`
	CurrencyCode        *string                 `json:"currencyCode,omitempty"`
	CurrencyRate        *json.Number            `json:"currencyRate,omitempty"`
	Status              *string                 `json:"status,omitempty"`
	SentToContact       *bool                   `json:"sentToContact,omitempty"`
	ExpectedPaymentDate *string                 `json:"expectedPaymentDate,omitempty"`
	PlannedPaymentDate  *string                 `json:"plannedPaymentDate,omitempty"`
	LineItems           *[]invoiceLineItemInput `json:"lineItems,omitempty"`
}

type invoiceLineItemInput struct {
	LineItemID     *string                 `json:"lineItemId,omitempty"`
	Description    string                  `json:"description"`
	Quantity       *json.Number            `json:"quantity,omitempty"`
	UnitAmount     *json.Number            `json:"unitAmount,omitempty"`
	ItemCode       *string                 `json:"itemCode,omitempty"`
	AccountCode    *string                 `json:"accountCode,omitempty"`
	AccountID      *string                 `json:"accountId,omitempty"`
	TaxType        *string                 `json:"taxType,omitempty"`
	TaxAmount      *json.Number            `json:"taxAmount,omitempty"`
	LineAmount     *json.Number            `json:"lineAmount,omitempty"`
	DiscountRate   *json.Number            `json:"discountRate,omitempty"`
	DiscountAmount *json.Number            `json:"discountAmount,omitempty"`
	Tracking       *[]invoiceTrackingInput `json:"tracking,omitempty"`
}

type invoiceTrackingInput struct {
	TrackingCategoryID *string `json:"trackingCategoryId,omitempty"`
	TrackingOptionID   *string `json:"trackingOptionId,omitempty"`
	Name               *string `json:"name,omitempty"`
	Option             *string `json:"option,omitempty"`
}

func validateCreateInvoiceInput(input invoiceCreateInput, invoiceType string) error {
	if err := validateInputNamespace(invoiceType); err != nil {
		return err
	}
	if err := validateUUIDValue("contactId", input.ContactID); err != nil {
		return err
	}
	if input.LineItems == nil || len(*input.LineItems) == 0 {
		return clierrors.New(clierrors.KindValidation, "lineItems is required and must contain at least one item")
	}
	if err := validateCommonInvoiceFields(input.Date, input.DueDate, input.LineAmountTypes, input.Reference, input.BrandingThemeID, input.URL, input.CurrencyCode, input.CurrencyRate); err != nil {
		return err
	}
	if input.Status != nil && !oneOf(*input.Status, "DRAFT", "SUBMITTED", "AUTHORISED") {
		return clierrors.New(clierrors.KindValidation, "status must be one of DRAFT, SUBMITTED, AUTHORISED for create")
	}
	if err := validateNamespaceFields(invoiceType, input.SentToContact, input.ExpectedPaymentDate, input.PlannedPaymentDate); err != nil {
		return err
	}
	return validateInvoiceLineItems(*input.LineItems, invoiceType, false)
}

func validateUpdateInvoiceInput(input invoiceUpdateInput, invoiceType string) error {
	if err := validateInputNamespace(invoiceType); err != nil {
		return err
	}
	if !hasInvoiceUpdateFields(input) {
		return clierrors.New(clierrors.KindValidation, "invoice update must contain at least one supported field")
	}
	if input.ContactID != nil {
		if err := validateUUIDValue("contactId", *input.ContactID); err != nil {
			return err
		}
	}
	if err := validateCommonInvoiceFields(input.Date, input.DueDate, input.LineAmountTypes, input.Reference, input.BrandingThemeID, input.URL, input.CurrencyCode, input.CurrencyRate); err != nil {
		return err
	}
	if input.Status != nil && !oneOf(*input.Status, "DRAFT", "SUBMITTED", "AUTHORISED", "DELETED", "VOIDED") {
		return clierrors.New(clierrors.KindValidation, "status must be one of DRAFT, SUBMITTED, AUTHORISED, DELETED, VOIDED for update")
	}
	if err := validateNamespaceFields(invoiceType, input.SentToContact, input.ExpectedPaymentDate, input.PlannedPaymentDate); err != nil {
		return err
	}
	if input.LineItems != nil {
		if len(*input.LineItems) == 0 {
			return clierrors.New(clierrors.KindValidation, "lineItems must contain at least one item when present")
		}
		if err := validateInvoiceLineItems(*input.LineItems, invoiceType, true); err != nil {
			return err
		}
	}
	return nil
}

func validateCommonInvoiceFields(date, dueDate, lineAmountTypes, reference, brandingThemeID, rawURL, currencyCode *string, currencyRate *json.Number) error {
	if err := validateOptionalDate("date", date); err != nil {
		return err
	}
	if err := validateOptionalDate("dueDate", dueDate); err != nil {
		return err
	}
	if lineAmountTypes != nil && !oneOf(*lineAmountTypes, "Exclusive", "Inclusive", "NoTax") {
		return clierrors.New(clierrors.KindValidation, "lineAmountTypes must be one of Exclusive, Inclusive, NoTax")
	}
	if reference != nil && utf8.RuneCountInString(*reference) > 255 {
		return clierrors.New(clierrors.KindValidation, "reference must not exceed 255 characters")
	}
	if brandingThemeID != nil {
		if err := validateUUIDValue("brandingThemeId", *brandingThemeID); err != nil {
			return err
		}
	}
	if rawURL != nil {
		parsed, err := url.Parse(*rawURL)
		if err != nil || parsed.IsAbs() == false || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return clierrors.New(clierrors.KindValidation, "url must be an absolute HTTP(S) URL")
		}
	}
	if currencyCode != nil {
		code := *currencyCode
		if len(code) != 3 || code != strings.ToUpper(code) || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' || code[2] < 'A' || code[2] > 'Z' {
			return clierrors.New(clierrors.KindValidation, "currencyCode must be a three-letter uppercase code")
		}
	}
	if currencyRate != nil && !positiveJSONNumber(*currencyRate) {
		return clierrors.New(clierrors.KindValidation, "currencyRate must be positive")
	}
	return nil
}

func validateNamespaceFields(invoiceType string, sentToContact *bool, expectedPaymentDate, plannedPaymentDate *string) error {
	switch invoiceType {
	case invoiceTypeSales:
		if plannedPaymentDate != nil {
			return clierrors.New(clierrors.KindValidation, "plannedPaymentDate is only valid for bills")
		}
		return validateOptionalDate("expectedPaymentDate", expectedPaymentDate)
	case invoiceTypePurchase:
		if sentToContact != nil {
			return clierrors.New(clierrors.KindValidation, "sentToContact is only valid for sales invoices")
		}
		if expectedPaymentDate != nil {
			return clierrors.New(clierrors.KindValidation, "expectedPaymentDate is only valid for sales invoices")
		}
		return validateOptionalDate("plannedPaymentDate", plannedPaymentDate)
	default:
		return clierrors.New(clierrors.KindInternal, fmt.Sprintf("unsupported invoice namespace type %q", invoiceType))
	}
}

func validateInvoiceLineItems(items []invoiceLineItemInput, invoiceType string, update bool) error {
	for index, item := range items {
		prefix := fmt.Sprintf("lineItems[%d]", index)
		if strings.TrimSpace(item.Description) == "" {
			return clierrors.New(clierrors.KindValidation, prefix+".description must not be empty")
		}
		if item.LineItemID != nil {
			if !update {
				return clierrors.New(clierrors.KindValidation, prefix+".lineItemId is only valid for update")
			}
			if err := validateUUIDValue(prefix+".lineItemId", *item.LineItemID); err != nil {
				return err
			}
		}
		if item.AccountID != nil {
			if err := validateUUIDValue(prefix+".accountId", *item.AccountID); err != nil {
				return err
			}
		}
		if invoiceType == invoiceTypePurchase && (item.DiscountRate != nil || item.DiscountAmount != nil) {
			return clierrors.New(clierrors.KindValidation, prefix+" discount fields are only valid for sales invoices")
		}
		if item.Tracking != nil {
			if len(*item.Tracking) > 2 {
				return clierrors.New(clierrors.KindValidation, prefix+".tracking must not contain more than two entries")
			}
			for trackingIndex, tracking := range *item.Tracking {
				if err := validateTrackingInput(tracking, fmt.Sprintf("%s.tracking[%d]", prefix, trackingIndex)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateTrackingInput(input invoiceTrackingInput, prefix string) error {
	usesIDs := input.TrackingCategoryID != nil || input.TrackingOptionID != nil
	usesNames := input.Name != nil || input.Option != nil
	if usesIDs && usesNames {
		return clierrors.New(clierrors.KindValidation, prefix+" must not mix ID and name selectors")
	}
	if usesIDs {
		if input.TrackingCategoryID == nil || input.TrackingOptionID == nil {
			return clierrors.New(clierrors.KindValidation, prefix+" requires both trackingCategoryId and trackingOptionId")
		}
		if err := validateUUIDValue(prefix+".trackingCategoryId", *input.TrackingCategoryID); err != nil {
			return err
		}
		return validateUUIDValue(prefix+".trackingOptionId", *input.TrackingOptionID)
	}
	if usesNames {
		if input.Name == nil || input.Option == nil || strings.TrimSpace(*input.Name) == "" || strings.TrimSpace(*input.Option) == "" {
			return clierrors.New(clierrors.KindValidation, prefix+" requires non-empty name and option")
		}
		return nil
	}
	return clierrors.New(clierrors.KindValidation, prefix+" requires either ID selectors or name selectors")
}

func validateOptionalDate(field string, value *string) error {
	if value == nil {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", *value)
	if err != nil || parsed.Format("2006-01-02") != *value {
		return clierrors.New(clierrors.KindValidation, field+" must use YYYY-MM-DD")
	}
	return nil
}

func validateUUIDValue(field, value string) error {
	if !invoiceIDPattern.MatchString(value) {
		return clierrors.New(clierrors.KindValidation, field+" must be a valid UUID")
	}
	return nil
}

func validateInputNamespace(invoiceType string) error {
	if invoiceType != invoiceTypeSales && invoiceType != invoiceTypePurchase {
		return clierrors.New(clierrors.KindInternal, fmt.Sprintf("unsupported invoice namespace type %q", invoiceType))
	}
	return nil
}

func hasInvoiceUpdateFields(input invoiceUpdateInput) bool {
	return input.ContactID != nil || input.Date != nil || input.DueDate != nil || input.LineAmountTypes != nil ||
		input.InvoiceNumber != nil || input.Reference != nil || input.BrandingThemeID != nil || input.URL != nil ||
		input.CurrencyCode != nil || input.CurrencyRate != nil || input.Status != nil || input.SentToContact != nil ||
		input.ExpectedPaymentDate != nil || input.PlannedPaymentDate != nil || input.LineItems != nil
}

func positiveJSONNumber(value json.Number) bool {
	rational, ok := new(big.Rat).SetString(value.String())
	return ok && rational.Sign() > 0
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func resolveIdempotencyKey(value string, supplied bool) (string, error) {
	if !supplied {
		return generateIdempotencyKey()
	}
	return validateIdempotencyKey(value)
}

func validateIdempotencyKey(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 || len(trimmed) > 128 {
		return "", clierrors.New(clierrors.KindValidation, "--idempotency-key must be 1-128 bytes after trimming")
	}
	for _, character := range trimmed {
		if unicode.IsControl(character) {
			return "", clierrors.New(clierrors.KindValidation, "--idempotency-key must not contain control characters")
		}
	}
	return trimmed, nil
}

func generateIdempotencyKey() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", clierrors.Wrap(clierrors.KindInternal, "generate idempotency key", err)
	}
	return hex.EncodeToString(random), nil
}
