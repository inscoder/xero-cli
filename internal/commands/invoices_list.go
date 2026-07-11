package commands

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	whereTypePattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_.])Type\s*(?:==|!=|=)`)
)

var validInvoiceStatuses = map[string]struct{}{
	"DRAFT":      {},
	"SUBMITTED":  {},
	"DELETED":    {},
	"AUTHORISED": {},
	"PAID":       {},
	"VOIDED":     {},
}

const (
	defaultInvoiceOrder = "UpdatedDateUTC DESC"
	defaultInvoicePage  = 1
)

type listInvoicesCommandConfig struct {
	Use           string
	Short         string
	Type          string
	Singular      string
	Plural        string
	BreadcrumbCmd string
}

func newInvoicesCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	mutationConfig := salesInvoiceCommandConfig()
	cmd := newListInvoicesCommand(deps, v, listInvoicesCommandConfig{
		Use:           "invoices",
		Short:         "List Xero sales invoices and related actions",
		Type:          "ACCREC",
		Singular:      "invoice",
		Plural:        "invoices",
		BreadcrumbCmd: "xero invoices --json",
	})
	cmd.AddCommand(newInvoicesApproveCommand(deps, v))
	cmd.AddCommand(newInvoicesPDFCommand(deps, v))
	cmd.AddCommand(newInvoicesOnlineURLCommand(deps, v))
	cmd.AddCommand(newInvoiceCreateCommand(deps, v, mutationConfig))
	cmd.AddCommand(newInvoiceUpdateCommand(deps, v, mutationConfig))
	cmd.AddCommand(newInvoiceAttachmentsCommand(deps, v, mutationConfig))
	return cmd
}

func newBillsCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	cmd := newListInvoicesCommand(deps, v, listInvoicesCommandConfig{
		Use:           "bills",
		Short:         "List and manage Xero purchase bills",
		Type:          "ACCPAY",
		Singular:      "bill",
		Plural:        "bills",
		BreadcrumbCmd: "xero bills --json",
	})
	mutationConfig := purchaseInvoiceCommandConfig()
	cmd.AddCommand(newInvoiceCreateCommand(deps, v, mutationConfig))
	cmd.AddCommand(newInvoiceUpdateCommand(deps, v, mutationConfig))
	cmd.AddCommand(newInvoiceAttachmentsCommand(deps, v, mutationConfig))
	return cmd
}

func newListInvoicesCommand(deps Dependencies, v *viper.Viper, config listInvoicesCommandConfig) *cobra.Command {
	var request xeroapi.ListInvoicesRequest
	request.Type = config.Type
	request.Page = defaultInvoicePage
	cmd := &cobra.Command{
		Use:   config.Use,
		Short: config.Short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			request.InvoiceIDs, err = normalizeInvoiceIDs(request.InvoiceIDs)
			if err != nil {
				return err
			}
			request.Statuses, err = normalizeStatuses(request.Statuses)
			if err != nil {
				return err
			}
			request.Where = strings.TrimSpace(request.Where)
			if cmd.Flags().Changed("where") && request.Where == "" {
				return clierrors.New(clierrors.KindValidation, "--where must not be empty")
			}
			if whereTypePattern.MatchString(request.Where) {
				return clierrors.New(clierrors.KindValidation, "invoice type is selected by the command; use `xero invoices` for sales invoices or `xero bills` for purchase bills")
			}
			request.Order, err = normalizeOrder(request.Order, cmd.Flags().Changed("order"))
			if err != nil {
				return err
			}
			if request.Since != "" {
				if _, err := time.Parse("2006-01-02", request.Since); err != nil {
					return clierrors.New(clierrors.KindValidation, "--since must use YYYY-MM-DD")
				}
			}
			if cmd.Flags().Changed("page") && request.Page <= 0 {
				return clierrors.New(clierrors.KindValidation, "--page must be positive")
			}
			if cmd.Flags().Changed("page-size") && request.PageSize <= 0 {
				return clierrors.New(clierrors.KindValidation, "--page-size must be positive")
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
			invoices, err := rt.Xero.ListInvoices(ctx, token, request)
			if err != nil {
				return err
			}
			summary := xeroapi.NewTypedRequestSummary(len(invoices), config.Singular, config.Plural)
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: config.BreadcrumbCmd}}
			return rt.WriteData(invoices, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoices(w, invoices, summary, breadcrumbs)
			})
		},
	}
	cmd.Flags().StringSliceVar(&request.InvoiceIDs, "invoice-id", nil, "invoice ID filter (repeatable or comma-separated)")
	cmd.Flags().StringSliceVar(&request.Statuses, "status", nil, "invoice status filter (repeatable or comma-separated)")
	cmd.Flags().StringVar(&request.Since, "since", "", "updated since date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&request.Where, "where", "", "advanced Xero where clause")
	cmd.Flags().StringVar(&request.Order, "order", defaultInvoiceOrder, "order clause (for example: 'UpdatedDateUTC DESC')")
	cmd.Flags().IntVar(&request.Page, "page", defaultInvoicePage, "page number")
	cmd.Flags().IntVar(&request.PageSize, "page-size", 0, "page size")
	return cmd
}

func normalizeInvoiceID(value string) (string, error) {
	normalized, err := normalizeInvoiceIDs([]string{value})
	if err != nil {
		return "", err
	}
	return normalized[0], nil
}

func normalizeInvoiceIDs(values []string) ([]string, error) {
	return normalizeUUIDs(values, "--invoice-id")
}

func normalizeStatuses(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		candidate := strings.ToUpper(strings.TrimSpace(value))
		if candidate == "" {
			return nil, clierrors.New(clierrors.KindValidation, "--status values must not be empty")
		}
		if _, ok := validInvoiceStatuses[candidate]; !ok {
			return nil, clierrors.New(clierrors.KindValidation, fmt.Sprintf("--status must be one of DRAFT, SUBMITTED, DELETED, AUTHORISED, PAID, VOIDED; got %q", value))
		}
		normalized = append(normalized, candidate)
	}
	return normalized, nil
}

func normalizeOrder(value string, changed bool) (string, error) {
	return normalizeOrderClause(value, changed, defaultInvoiceOrder)
}
