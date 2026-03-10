package output_test

import (
	"bytes"
	"testing"

	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
)

func TestWriteJSONEnvelopeContract(t *testing.T) {
	var buffer bytes.Buffer
	invoices := []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "INV-0001", ContactName: "Acme Ltd", Status: "AUTHORISED", Total: 123.45, AmountDue: 23.45, Currency: "USD", DueDate: "2026-03-10", UpdatedAt: "2026-03-09T12:30:00Z"}}
	breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: "xero invoices --tenant tenant-1 --json"}}

	if err := output.WriteJSON(&buffer, invoices, "1 invoice", breadcrumbs, false); err != nil {
		t.Fatalf("write json: %v", err)
	}

	expected := "{\n  \"ok\": true,\n  \"data\": [\n    {\n      \"invoiceId\": \"1\",\n      \"invoiceNumber\": \"INV-0001\",\n      \"contactName\": \"Acme Ltd\",\n      \"status\": \"AUTHORISED\",\n      \"total\": 123.45,\n      \"amountDue\": 23.45,\n      \"currency\": \"USD\",\n      \"dueDate\": \"2026-03-10\",\n      \"updatedAt\": \"2026-03-09T12:30:00Z\"\n    }\n  ],\n  \"summary\": \"1 invoice\",\n  \"breadcrumbs\": [\n    {\n      \"action\": \"show\",\n      \"cmd\": \"xero invoices --tenant tenant-1 --json\"\n    }\n  ]\n}\n"
	if buffer.String() != expected {
		t.Fatalf("unexpected envelope:\n%s", buffer.String())
	}
}

func TestWriteJSONQuietEmitsRawDataOnly(t *testing.T) {
	var buffer bytes.Buffer
	invoices := []xeroapi.Invoice{{InvoiceID: "1", InvoiceNumber: "INV-0001"}}

	if err := output.WriteJSON(&buffer, invoices, "1 invoice", nil, true); err != nil {
		t.Fatalf("write quiet json: %v", err)
	}
	expected := "[\n  {\n    \"invoiceId\": \"1\",\n    \"invoiceNumber\": \"INV-0001\"\n  }\n]\n"
	if buffer.String() != expected {
		t.Fatalf("unexpected quiet payload:\n%s", buffer.String())
	}
}
