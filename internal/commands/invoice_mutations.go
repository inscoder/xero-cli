package commands

import (
	"fmt"
	"strings"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

type invoiceCommandConfig struct {
	Namespace string
	Singular  string
	Plural    string
	Type      string
}

func salesInvoiceCommandConfig() invoiceCommandConfig {
	return invoiceCommandConfig{Namespace: "invoices", Singular: "invoice", Plural: "invoices", Type: invoiceTypeSales}
}

func purchaseInvoiceCommandConfig() invoiceCommandConfig {
	return invoiceCommandConfig{Namespace: "bills", Singular: "bill", Plural: "bills", Type: invoiceTypePurchase}
}

func createInvoiceWrite(input invoiceCreateInput, invoiceType string) xeroapi.InvoiceWrite {
	status := "DRAFT"
	if input.Status != nil {
		status = *input.Status
	}
	lineItems := invoiceWriteLineItems(*input.LineItems)
	return xeroapi.InvoiceWrite{
		Type:                invoiceType,
		Contact:             &xeroapi.InvoiceWriteContact{ContactID: input.ContactID},
		Date:                input.Date,
		DueDate:             input.DueDate,
		LineAmountTypes:     input.LineAmountTypes,
		InvoiceNumber:       input.InvoiceNumber,
		Reference:           input.Reference,
		BrandingThemeID:     input.BrandingThemeID,
		URL:                 input.URL,
		CurrencyCode:        input.CurrencyCode,
		CurrencyRate:        input.CurrencyRate,
		Status:              &status,
		SentToContact:       input.SentToContact,
		ExpectedPaymentDate: input.ExpectedPaymentDate,
		PlannedPaymentDate:  input.PlannedPaymentDate,
		LineItems:           &lineItems,
	}
}

func updateInvoiceWrite(input invoiceUpdateInput, invoiceID, invoiceType string) xeroapi.InvoiceWrite {
	write := xeroapi.InvoiceWrite{
		InvoiceID:           invoiceID,
		Type:                invoiceType,
		Date:                input.Date,
		DueDate:             input.DueDate,
		LineAmountTypes:     input.LineAmountTypes,
		InvoiceNumber:       input.InvoiceNumber,
		Reference:           input.Reference,
		BrandingThemeID:     input.BrandingThemeID,
		URL:                 input.URL,
		CurrencyCode:        input.CurrencyCode,
		CurrencyRate:        input.CurrencyRate,
		Status:              input.Status,
		SentToContact:       input.SentToContact,
		ExpectedPaymentDate: input.ExpectedPaymentDate,
		PlannedPaymentDate:  input.PlannedPaymentDate,
	}
	if input.ContactID != nil {
		write.Contact = &xeroapi.InvoiceWriteContact{ContactID: *input.ContactID}
	}
	if input.LineItems != nil {
		lineItems := invoiceWriteLineItems(*input.LineItems)
		write.LineItems = &lineItems
	}
	return write
}

func invoiceWriteLineItems(inputs []invoiceLineItemInput) []xeroapi.InvoiceWriteLineItem {
	items := make([]xeroapi.InvoiceWriteLineItem, 0, len(inputs))
	for _, input := range inputs {
		item := xeroapi.InvoiceWriteLineItem{
			LineItemID:     input.LineItemID,
			Description:    input.Description,
			Quantity:       input.Quantity,
			UnitAmount:     input.UnitAmount,
			ItemCode:       input.ItemCode,
			AccountCode:    input.AccountCode,
			AccountID:      input.AccountID,
			TaxType:        input.TaxType,
			TaxAmount:      input.TaxAmount,
			LineAmount:     input.LineAmount,
			DiscountRate:   input.DiscountRate,
			DiscountAmount: input.DiscountAmount,
		}
		if input.Tracking != nil {
			tracking := make([]xeroapi.InvoiceWriteTracking, 0, len(*input.Tracking))
			for _, inputTracking := range *input.Tracking {
				tracking = append(tracking, xeroapi.InvoiceWriteTracking{
					TrackingCategoryID: inputTracking.TrackingCategoryID,
					TrackingOptionID:   inputTracking.TrackingOptionID,
					Name:               inputTracking.Name,
					Option:             inputTracking.Option,
				})
			}
			item.Tracking = &tracking
		}
		items = append(items, item)
	}
	return items
}

func validateInvoicePreflight(invoice xeroapi.Invoice, invoiceID string, config invoiceCommandConfig) error {
	if strings.TrimSpace(invoice.InvoiceID) == "" || !strings.EqualFold(invoice.InvoiceID, invoiceID) {
		return clierrors.New(clierrors.KindXeroRequest, "Xero invoice preflight did not match the requested invoice ID")
	}
	if !strings.EqualFold(invoice.Type, config.Type) {
		return clierrors.New(clierrors.KindValidation, fmt.Sprintf("invoice %s has Type %s and cannot be updated through `xero %s`; expected %s", invoiceID, invoice.Type, config.Namespace, config.Type))
	}
	return nil
}

func countRemovedLineItems(current []xeroapi.InvoiceLineItem, submitted []invoiceLineItemInput) int {
	submittedIDs := make(map[string]struct{}, len(submitted))
	for _, item := range submitted {
		if item.LineItemID != nil && strings.TrimSpace(*item.LineItemID) != "" {
			submittedIDs[strings.ToLower(*item.LineItemID)] = struct{}{}
		}
	}
	removed := 0
	for _, item := range current {
		if strings.TrimSpace(item.LineItemID) == "" {
			continue
		}
		if _, ok := submittedIDs[strings.ToLower(item.LineItemID)]; !ok {
			removed++
		}
	}
	return removed
}
