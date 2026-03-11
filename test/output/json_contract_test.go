package output_test

import (
	"bytes"
	"testing"

	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
)

func TestWriteJSONEnvelopeContract(t *testing.T) {
	var buffer bytes.Buffer
	invoices := []xeroapi.Invoice{{
		InvoiceID:       "1",
		Type:            "ACCREC",
		InvoiceNumber:   "INV-0001",
		Reference:       "PO-123",
		Contact:         xeroapi.InvoiceContact{ContactID: "contact-1", Name: "Acme Ltd"},
		ContactName:     "Acme Ltd",
		Date:            "2026-03-01",
		DueDate:         "2026-03-10",
		Status:          "AUTHORISED",
		LineAmountTypes: "Exclusive",
		LineItems:       []xeroapi.InvoiceLineItem{},
		SubTotal:        123.45,
		TotalTax:        12.34,
		Total:           135.79,
		TotalDiscount:   0,
		AmountDue:       23.45,
		AmountPaid:      112.34,
		AmountCredited:  0,
		CurrencyCode:    "USD",
		Currency:        "USD",
		CurrencyRate:    1,
		UpdatedAt:       "2026-03-09T12:30:00Z",
		Payments:        []xeroapi.InvoicePayment{},
		CreditNotes:     []xeroapi.InvoiceAllocation{},
		Prepayments:     []xeroapi.InvoiceAllocation{},
		Overpayments:    []xeroapi.InvoiceAllocation{},
	}}
	breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: "xero invoices --tenant tenant-1 --json"}}

	if err := output.WriteJSON(&buffer, invoices, "1 invoice", breadcrumbs, false); err != nil {
		t.Fatalf("write json: %v", err)
	}

	expected := "{\n  \"ok\": true,\n  \"data\": [\n    {\n      \"invoiceId\": \"1\",\n      \"type\": \"ACCREC\",\n      \"invoiceNumber\": \"INV-0001\",\n      \"reference\": \"PO-123\",\n      \"contact\": {\n        \"contactId\": \"contact-1\",\n        \"name\": \"Acme Ltd\"\n      },\n      \"date\": \"2026-03-01\",\n      \"dueDate\": \"2026-03-10\",\n      \"status\": \"AUTHORISED\",\n      \"lineAmountTypes\": \"Exclusive\",\n      \"lineItems\": [],\n      \"subTotal\": 123.45,\n      \"totalTax\": 12.34,\n      \"total\": 135.79,\n      \"totalDiscount\": 0,\n      \"amountDue\": 23.45,\n      \"amountPaid\": 112.34,\n      \"amountCredited\": 0,\n      \"currencyCode\": \"USD\",\n      \"currencyRate\": 1,\n      \"updatedAt\": \"2026-03-09T12:30:00Z\",\n      \"brandingThemeId\": \"\",\n      \"url\": \"\",\n      \"sentToContact\": false,\n      \"expectedPaymentDate\": \"\",\n      \"plannedPaymentDate\": \"\",\n      \"hasAttachments\": false,\n      \"payments\": [],\n      \"creditNotes\": [],\n      \"prepayments\": [],\n      \"overpayments\": [],\n      \"contactName\": \"Acme Ltd\",\n      \"currency\": \"USD\"\n    }\n  ],\n  \"summary\": \"1 invoice\",\n  \"breadcrumbs\": [\n    {\n      \"action\": \"show\",\n      \"cmd\": \"xero invoices --tenant tenant-1 --json\"\n    }\n  ]\n}\n"
	if buffer.String() != expected {
		t.Fatalf("unexpected envelope:\n%s", buffer.String())
	}
}

func TestWriteJSONQuietEmitsRawDataOnly(t *testing.T) {
	var buffer bytes.Buffer
	invoices := []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "INV-0001", LineItems: []xeroapi.InvoiceLineItem{}, Payments: []xeroapi.InvoicePayment{}, CreditNotes: []xeroapi.InvoiceAllocation{}, Prepayments: []xeroapi.InvoiceAllocation{}, Overpayments: []xeroapi.InvoiceAllocation{}}}

	if err := output.WriteJSON(&buffer, invoices, "1 invoice", nil, true); err != nil {
		t.Fatalf("write quiet json: %v", err)
	}
	expected := "[\n  {\n    \"invoiceId\": \"1\",\n    \"type\": \"\",\n    \"invoiceNumber\": \"INV-0001\",\n    \"reference\": \"\",\n    \"contact\": {},\n    \"date\": \"\",\n    \"dueDate\": \"\",\n    \"status\": \"\",\n    \"lineAmountTypes\": \"\",\n    \"lineItems\": [],\n    \"subTotal\": 0,\n    \"totalTax\": 0,\n    \"total\": 0,\n    \"totalDiscount\": 0,\n    \"amountDue\": 0,\n    \"amountPaid\": 0,\n    \"amountCredited\": 0,\n    \"currencyCode\": \"\",\n    \"currencyRate\": 0,\n    \"updatedAt\": \"\",\n    \"brandingThemeId\": \"\",\n    \"url\": \"\",\n    \"sentToContact\": false,\n    \"expectedPaymentDate\": \"\",\n    \"plannedPaymentDate\": \"\",\n    \"hasAttachments\": false,\n    \"payments\": [],\n    \"creditNotes\": [],\n    \"prepayments\": [],\n    \"overpayments\": []\n  }\n]\n"
	if buffer.String() != expected {
		t.Fatalf("unexpected quiet payload:\n%s", buffer.String())
	}
}
