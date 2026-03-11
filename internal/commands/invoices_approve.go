package commands

import (
	"fmt"
	"io"

	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInvoicesApproveCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var request xeroapi.ApproveInvoiceRequest

	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve one sales invoice",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			invoiceID, err := normalizeInvoiceID(request.InvoiceID)
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

			tenant, err := rt.Tenants.Resolve(firstNonEmpty(request.TenantID, rt.Settings.TenantOverride), rt.SessionMeta.KnownTenants)
			if err != nil {
				return err
			}

			request.InvoiceID = invoiceID
			request.TenantID = tenant.ID

			ctx, cancel := rt.Context()
			defer cancel()

			result, err := rt.Xero.ApproveInvoice(ctx, token, request)
			if err != nil {
				return err
			}

			summary := "invoice approved"
			breadcrumbs := []output.Breadcrumb{{
				Action: "show",
				Cmd:    fmt.Sprintf("xero invoices --invoice-id %s --tenant %s --json", result.InvoiceID, result.TenantID),
			}}

			return rt.WriteData(result, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoiceApproved(w, result)
			})
		},
	}

	cmd.Flags().StringVar(&request.InvoiceID, "invoice-id", "", "invoice ID")
	_ = cmd.MarkFlagRequired("invoice-id")
	return cmd
}
