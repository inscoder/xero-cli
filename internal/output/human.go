package output

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/inscoder/xero-cli/internal/auth"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

func WriteInvoices(writer io.Writer, invoices []xeroapi.Invoice, summary string, breadcrumbs []Breadcrumb) error {
	tw := tabwriter.NewWriter(writer, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NUMBER\tCONTACT\tSTATUS\tTOTAL\tAMOUNT DUE\tUPDATED"); err != nil {
		return err
	}
	for _, invoice := range invoices {
		contactName := invoice.Contact.Name
		if contactName == "" {
			contactName = invoice.ContactName
		}
		currencyCode := invoice.CurrencyCode
		if currencyCode == "" {
			currencyCode = invoice.Currency
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%.2f %s\t%.2f\t%s\n", invoice.InvoiceNumber, contactName, invoice.Status, invoice.Total, currencyCode, invoice.AmountDue, invoice.UpdatedAt); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, summary); err != nil {
		return err
	}
	for _, breadcrumb := range breadcrumbs {
		if _, err := fmt.Fprintf(writer, "Next: %s (%s)\n", breadcrumb.Action, breadcrumb.Cmd); err != nil {
			return err
		}
	}
	return nil
}

func WriteOnlineInvoiceURL(writer io.Writer, result xeroapi.OnlineInvoiceResult) error {
	if result.Available {
		_, err := fmt.Fprintln(writer, result.OnlineInvoiceURL)
		return err
	}
	_, err := fmt.Fprintf(writer, "No online invoice URL available for invoice %s\n", result.InvoiceID)
	return err
}

func WriteInvoicePDFSaved(writer io.Writer, result xeroapi.InvoicePDFResult) error {
	_, err := fmt.Fprintf(writer, "Saved invoice PDF to %s (%d bytes)\n", result.SavedTo, result.Bytes)
	return err
}

func WriteInvoiceApproved(writer io.Writer, result xeroapi.InvoiceApprovalResult) error {
	label := result.InvoiceID
	if result.InvoiceNumber != "" {
		label = fmt.Sprintf("%s (%s)", result.InvoiceNumber, result.InvoiceID)
	}
	_, err := fmt.Fprintf(writer, "Approved invoice %s for tenant %s (%s)\n", label, result.TenantID, result.Status)
	return err
}

func WriteStatus(writer io.Writer, authenticated bool, session auth.SessionMetadata, defaultTenantID, defaultTenantName string, refreshNeeded bool) error {
	status := "Not authenticated"
	if authenticated {
		status = "Authenticated"
	}
	_, err := fmt.Fprintf(writer, "Status: %s\nAuth mode: %s\nToken generated: %s\nToken expires: %s\nRefresh needed: %t\nDefault tenant: %s (%s)\nStorage: %s\n",
		status,
		blankOr(session.AuthMode, "none"),
		formatTime(session.GeneratedAt),
		formatTime(session.ExpiresAt),
		refreshNeeded,
		blankOr(defaultTenantName, "none"),
		blankOr(defaultTenantID, "none"),
		blankOr(session.StorageMode, "unknown"),
	)
	return err
}

func WriteDoctor(writer io.Writer, results []DoctorCheck) error {
	for _, result := range results {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\n", result.Status, result.Name, result.Detail); err != nil {
			return err
		}
	}
	return nil
}

type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func blankOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func formatTime(value interface {
	IsZero() bool
	String() string
}) string {
	if value.IsZero() {
		return "n/a"
	}
	return value.String()
}
