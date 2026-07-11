package commands

import (
	"io"
	"strings"
	"time"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultContactPage = 1

func newContactsCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contacts",
		Short: "Manage Xero contacts",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newContactsListCommand(deps, v))
	cmd.AddCommand(newContactsCreateCommand(deps, v))
	cmd.AddCommand(newContactsUpdateCommand(deps, v))
	return cmd
}

func newContactsListCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var request xeroapi.ListContactsRequest
	request.Page = defaultContactPage

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Xero contacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			request.Search = strings.TrimSpace(request.Search)
			if cmd.Flags().Changed("search") && request.Search == "" {
				return clierrors.New(clierrors.KindValidation, "--search must not be empty")
			}
			if cmd.Flags().Changed("contact-id") && len(request.ContactIDs) == 0 {
				return clierrors.New(clierrors.KindValidation, "--contact-id values must not be empty")
			}
			request.ContactIDs, err = normalizeUUIDs(request.ContactIDs, "--contact-id")
			if err != nil {
				return err
			}
			request.Where = strings.TrimSpace(request.Where)
			if cmd.Flags().Changed("where") && request.Where == "" {
				return clierrors.New(clierrors.KindValidation, "--where must not be empty")
			}
			request.Order, err = normalizeOrderClause(request.Order, cmd.Flags().Changed("order"), "")
			if err != nil {
				return err
			}
			request.Since = strings.TrimSpace(request.Since)
			if request.Since != "" {
				if _, err := time.Parse("2006-01-02", request.Since); err != nil {
					return clierrors.New(clierrors.KindValidation, "--since must use YYYY-MM-DD")
				}
			} else if cmd.Flags().Changed("since") {
				return clierrors.New(clierrors.KindValidation, "--since must use YYYY-MM-DD")
			}
			if request.Page <= 0 {
				return clierrors.New(clierrors.KindValidation, "--page must be positive")
			}
			if cmd.Flags().Changed("page-size") && request.PageSize <= 0 {
				return clierrors.New(clierrors.KindValidation, "--page-size must be positive")
			}

			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if rt.Contacts == nil {
				return clierrors.New(clierrors.KindInternal, "contact client is not configured")
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
			request.TenantID = tenant.ID

			ctx, cancel := rt.Context()
			defer cancel()
			result, err := rt.Contacts.ListContacts(ctx, token, request)
			if err != nil {
				return err
			}
			summary := xeroapi.NewTypedRequestSummary(len(result.Contacts), "contact", "contacts")
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: "xero contacts list --json"}}
			return rt.WriteData(result.Contacts, summary, breadcrumbs, func(writer io.Writer) error {
				return output.WriteContacts(writer, result.Contacts, summary, breadcrumbs)
			})
		},
	}
	cmd.Flags().StringVar(&request.Search, "search", "", "search contact name, number, company number, or email")
	cmd.Flags().StringSliceVar(&request.ContactIDs, "contact-id", nil, "contact ID filter (repeatable or comma-separated)")
	cmd.Flags().IntVar(&request.Page, "page", defaultContactPage, "page number")
	cmd.Flags().IntVar(&request.PageSize, "page-size", 0, "page size")
	cmd.Flags().BoolVar(&request.IncludeArchived, "include-archived", false, "include archived contacts")
	cmd.Flags().BoolVar(&request.SummaryOnly, "summary-only", false, "request Xero's summary contact projection")
	cmd.Flags().StringVar(&request.Since, "since", "", "updated since date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&request.Where, "where", "", "advanced Xero where clause")
	cmd.Flags().StringVar(&request.Order, "order", "", "order clause (for example: 'Name ASC')")
	return cmd
}
