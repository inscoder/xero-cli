package commands

import (
	"fmt"
	"io"

	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInvoiceCreateCommand(deps Dependencies, v *viper.Viper, config invoiceCommandConfig) *cobra.Command {
	var filePath string
	var idempotencyKey string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create one Xero " + config.Singular,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var input invoiceCreateInput
			if err := decodeJSONInput(filePath, deps.IO.In, deps.IsTerminal(0), &input); err != nil {
				return err
			}
			if err := validateCreateInvoiceInput(input, config.Type); err != nil {
				return err
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
			result, err := rt.Xero.CreateInvoice(ctx, token, xeroapi.CreateInvoiceRequest{
				TenantID:       tenant.ID,
				Resource:       config.Singular,
				Namespace:      config.Namespace,
				IdempotencyKey: effectiveKey,
				Invoice:        createInvoiceWrite(input, config.Type),
			})
			if err != nil {
				return err
			}

			summary := config.Singular + " created"
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: fmt.Sprintf("xero %s --invoice-id %s --json", config.Namespace, result.InvoiceID)}}
			return rt.WriteData(result, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoiceMutation(w, result)
			})
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "JSON input file path or - for stdin")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for an exact retry (1-128 bytes)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
