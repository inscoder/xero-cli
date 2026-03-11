package commands

import (
	"fmt"
	"io"

	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInvoicesOnlineURLCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var request xeroapi.GetOnlineInvoiceRequest

	cmd := &cobra.Command{
		Use:   "online-url",
		Short: "Get the online invoice URL for an invoice",
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

			result, err := rt.Xero.GetOnlineInvoice(ctx, token, request)
			if err != nil {
				return err
			}

			summary := "online invoice URL unavailable"
			if result.Available {
				summary = "online invoice URL available"
			}

			breadcrumbs := []output.Breadcrumb{{
				Action: "show",
				Cmd:    fmt.Sprintf("xero invoices online-url --invoice-id %s --tenant %s --json", result.InvoiceID, tenant.ID),
			}}

			return rt.WriteData(result, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteOnlineInvoiceURL(w, result)
			})
		},
	}

	cmd.Flags().StringVar(&request.InvoiceID, "invoice-id", "", "invoice ID")
	_ = cmd.MarkFlagRequired("invoice-id")
	return cmd
}
