package commands

import (
	"encoding/json"
	"strings"
	"testing"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

const testUUID = "220ddca8-3144-4085-9a88-2d72c5133734"

func TestValidateCreateInvoiceInputAcceptsNamespaceFields(t *testing.T) {
	sales := validCreateInvoiceInput()
	sales.ExpectedPaymentDate = stringPointer("2026-08-01")
	sales.SentToContact = boolPointer(false)
	if err := validateCreateInvoiceInput(sales, invoiceTypeSales); err != nil {
		t.Fatalf("validate sales invoice: %v", err)
	}

	bill := validCreateInvoiceInput()
	bill.PlannedPaymentDate = stringPointer("2026-07-31")
	if err := validateCreateInvoiceInput(bill, invoiceTypePurchase); err != nil {
		t.Fatalf("validate bill: %v", err)
	}
}

func TestValidateCreateInvoiceInputRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name        string
		invoiceType string
		mutate      func(*invoiceCreateInput)
		match       string
	}{
		{name: "contact UUID", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.ContactID = "nope" }, match: "contactId"},
		{name: "missing line items", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.LineItems = nil }, match: "lineItems"},
		{name: "bad date", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.Date = stringPointer("07/10/2026") }, match: "YYYY-MM-DD"},
		{name: "paid status", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.Status = stringPointer("PAID") }, match: "status"},
		{name: "currency", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.CurrencyCode = stringPointer("hkd") }, match: "currencyCode"},
		{name: "currency rate", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.CurrencyRate = numberPointer("0") }, match: "currencyRate"},
		{name: "URL", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.URL = stringPointer("example.com") }, match: "absolute HTTP"},
		{name: "bill sales field", invoiceType: invoiceTypePurchase, mutate: func(input *invoiceCreateInput) { input.SentToContact = boolPointer(false) }, match: "sales invoices"},
		{name: "invoice bill field", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { input.PlannedPaymentDate = stringPointer("2026-07-31") }, match: "bills"},
		{name: "bill discount", invoiceType: invoiceTypePurchase, mutate: func(input *invoiceCreateInput) { (*input.LineItems)[0].DiscountAmount = numberPointer("1") }, match: "discount fields"},
		{name: "create line ID", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { (*input.LineItems)[0].LineItemID = stringPointer(testUUID) }, match: "only valid for update"},
		{name: "empty description", invoiceType: invoiceTypeSales, mutate: func(input *invoiceCreateInput) { (*input.LineItems)[0].Description = " " }, match: "description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateInvoiceInput()
			tt.mutate(&input)
			err := validateCreateInvoiceInput(input, tt.invoiceType)
			if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), tt.match) {
				t.Fatalf("expected validation error containing %q, got %v", tt.match, err)
			}
		})
	}
}

func TestValidateUpdateInvoiceInputUsesPresenceSemantics(t *testing.T) {
	if err := validateUpdateInvoiceInput(invoiceUpdateInput{}, invoiceTypeSales); clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected empty update rejection, got %v", err)
	}

	input := invoiceUpdateInput{SentToContact: boolPointer(false), Reference: stringPointer("")}
	if err := validateUpdateInvoiceInput(input, invoiceTypeSales); err != nil {
		t.Fatalf("expected explicit zero values to count as update fields: %v", err)
	}

	input = invoiceUpdateInput{LineItems: &[]invoiceLineItemInput{}}
	if err := validateUpdateInvoiceInput(input, invoiceTypeSales); clierrors.KindOf(err) != clierrors.KindValidation || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected empty line item rejection, got %v", err)
	}
}

func TestValidateInvoiceTrackingSelectors(t *testing.T) {
	valid := invoiceTrackingInput{Name: stringPointer("Region"), Option: stringPointer("APAC")}
	if err := validateTrackingInput(valid, "tracking"); err != nil {
		t.Fatalf("validate name selectors: %v", err)
	}
	valid = invoiceTrackingInput{TrackingCategoryID: stringPointer(testUUID), TrackingOptionID: stringPointer(testUUID)}
	if err := validateTrackingInput(valid, "tracking"); err != nil {
		t.Fatalf("validate ID selectors: %v", err)
	}

	mixed := invoiceTrackingInput{TrackingCategoryID: stringPointer(testUUID), TrackingOptionID: stringPointer(testUUID), Name: stringPointer("Region")}
	if err := validateTrackingInput(mixed, "tracking"); clierrors.KindOf(err) != clierrors.KindValidation || !strings.Contains(err.Error(), "mix") {
		t.Fatalf("expected mixed selector rejection, got %v", err)
	}
	partial := invoiceTrackingInput{Name: stringPointer("Region")}
	if err := validateTrackingInput(partial, "tracking"); clierrors.KindOf(err) != clierrors.KindValidation || !strings.Contains(err.Error(), "requires") {
		t.Fatalf("expected partial selector rejection, got %v", err)
	}
}

func TestResolveIdempotencyKey(t *testing.T) {
	key, err := resolveIdempotencyKey("  caller-key  ", true)
	if err != nil || key != "caller-key" {
		t.Fatalf("validate supplied key: key=%q err=%v", key, err)
	}
	for _, invalid := range []string{" ", strings.Repeat("a", 129), "abc\x00def"} {
		if _, err := resolveIdempotencyKey(invalid, true); clierrors.KindOf(err) != clierrors.KindValidation {
			t.Fatalf("expected invalid key %q to fail, got %v", invalid, err)
		}
	}
	generated1, err := resolveIdempotencyKey("", false)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	generated2, err := resolveIdempotencyKey("", false)
	if err != nil {
		t.Fatalf("generate second key: %v", err)
	}
	if len(generated1) != 64 || generated1 == generated2 {
		t.Fatalf("expected unique 64-character keys, got %q and %q", generated1, generated2)
	}
}

func validCreateInvoiceInput() invoiceCreateInput {
	items := []invoiceLineItemInput{{Description: "Consulting", Quantity: numberPointer("2"), UnitAmount: numberPointer("1500.00")}}
	return invoiceCreateInput{
		ContactID: testUUID,
		LineItems: &items,
	}
}

func stringPointer(value string) *string { return &value }
func boolPointer(value bool) *bool       { return &value }

func numberPointer(value string) *json.Number {
	number := json.Number(value)
	return &number
}
