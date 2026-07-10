package commands

import (
	"fmt"
	"io"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInvoiceUpdateCommand(deps Dependencies, v *viper.Viper, config invoiceCommandConfig) *cobra.Command {
	var invoiceID string
	var filePath string
	var idempotencyKey string
	var replaceLineItems bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update one Xero " + config.Singular,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedInvoiceID, err := normalizeInvoiceID(invoiceID)
			if err != nil {
				return err
			}
			var input invoiceUpdateInput
			if err := decodeJSONInput(filePath, deps.IO.In, deps.IsTerminal(0), &input); err != nil {
				return err
			}
			if err := validateUpdateInvoiceInput(input, config.Type); err != nil {
				return err
			}
			if input.LineItems != nil && !replaceLineItems {
				return clierrors.New(clierrors.KindValidation, "lineItems is a complete replacement; pass --replace-line-items to confirm")
			}
			if input.LineItems == nil && replaceLineItems {
				return clierrors.New(clierrors.KindValidation, "--replace-line-items requires lineItems in the input")
			}
			effectiveKey, err := resolveIdempotencyKey(idempotencyKey, cmd.Flags().Changed("idempotency-key"))
			if err != nil {
				return err
			}

			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			token, err := rt.LoadToken()
			if err != nil {
				return err
			}
			token, err = rt.EnsureToken(token)
			if err != nil {
				return err
			}
			tenant, err := rt.Tenants.ResolveTokenTenant(token)
			if err != nil {
				return err
			}

			ctx, cancel := rt.Context()
			defer cancel()
			current, err := rt.Xero.GetInvoice(ctx, token, xeroapi.GetInvoiceRequest{TenantID: tenant.ID, InvoiceID: normalizedInvoiceID})
			if err != nil {
				return err
			}
			if err := validateInvoicePreflight(current, normalizedInvoiceID, config); err != nil {
				return err
			}

			removed := 0
			if input.LineItems != nil {
				removed = countRemovedLineItems(current.LineItems, *input.LineItems)
			}
			result, err := rt.Xero.UpdateInvoice(ctx, token, xeroapi.UpdateInvoiceRequest{
				TenantID:       tenant.ID,
				Resource:       config.Singular,
				Namespace:      config.Namespace,
				InvoiceID:      normalizedInvoiceID,
				IdempotencyKey: effectiveKey,
				Invoice:        updateInvoiceWrite(input, normalizedInvoiceID, config.Type),
			})
			if err != nil {
				return err
			}
			result.LineItemsReplaced = input.LineItems != nil
			result.RemovedLineItemCount = removed

			summary := config.Singular + " updated"
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: fmt.Sprintf("xero %s --invoice-id %s --json", config.Namespace, normalizedInvoiceID)}}
			return rt.WriteData(result, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoiceMutation(w, result)
			})
		},
	}
	cmd.Flags().StringVar(&invoiceID, "invoice-id", "", config.Singular+" invoice ID")
	cmd.Flags().StringVar(&filePath, "file", "", "JSON input file path or - for stdin")
	cmd.Flags().BoolVar(&replaceLineItems, "replace-line-items", false, "confirm complete line-item replacement")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for an exact retry (1-128 bytes)")
	_ = cmd.MarkFlagRequired("invoice-id")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
