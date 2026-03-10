package commands

import (
	"fmt"
	"io"
	"time"

	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInvoicesCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var request xeroapi.ListInvoicesRequest
	cmd := &cobra.Command{
		Use:   "invoices",
		Short: "List Xero invoices",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if request.Since != "" {
				if _, err := time.Parse("2006-01-02", request.Since); err != nil {
					return clierrors.New(clierrors.KindValidation, "--since must use YYYY-MM-DD")
				}
			}
			if request.Page < 0 || request.Limit < 0 {
				return clierrors.New(clierrors.KindValidation, "--page and --limit must be positive")
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
			request.TenantID = tenant.ID
			ctx, cancel := rt.Context()
			defer cancel()
			invoices, err := rt.Xero.ListInvoices(ctx, token, request)
			if err != nil {
				return err
			}
			summary := xeroapi.NewRequestSummary(len(invoices))
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: fmt.Sprintf("xero invoices --tenant %s --json", tenant.ID)}}
			return rt.WriteData(invoices, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoices(w, invoices, summary, breadcrumbs)
			})
		},
	}
	cmd.Flags().StringVar(&request.Status, "status", "", "invoice status filter")
	cmd.Flags().StringVar(&request.Contact, "contact", "", "contact name or ID filter")
	cmd.Flags().StringVar(&request.Since, "since", "", "updated since date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&request.Page, "page", 0, "page number")
	cmd.Flags().IntVar(&request.Limit, "limit", 0, "page size")
	return cmd
}
