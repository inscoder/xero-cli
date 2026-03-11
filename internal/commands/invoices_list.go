package commands

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	invoiceIDPattern  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	orderFieldPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.]*$`)
)

var validInvoiceStatuses = map[string]struct{}{
	"DRAFT":      {},
	"SUBMITTED":  {},
	"DELETED":    {},
	"AUTHORISED": {},
	"PAID":       {},
	"VOIDED":     {},
}

const defaultInvoiceOrder = "UpdatedDateUTC DESC"

func newInvoicesCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var request xeroapi.ListInvoicesRequest
	cmd := &cobra.Command{
		Use:   "invoices",
		Short: "List Xero invoices and related actions",
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
			if request.PageSize > 0 && request.Page <= 0 {
				return clierrors.New(clierrors.KindValidation, "--page-size requires --page")
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
	cmd.Flags().StringSliceVar(&request.InvoiceIDs, "invoice-id", nil, "invoice ID filter (repeatable or comma-separated)")
	cmd.Flags().StringSliceVar(&request.Statuses, "status", nil, "invoice status filter (repeatable or comma-separated)")
	cmd.Flags().StringVar(&request.Since, "since", "", "updated since date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&request.Where, "where", "", "advanced Xero where clause")
	cmd.Flags().StringVar(&request.Order, "order", defaultInvoiceOrder, "order clause (for example: 'UpdatedDateUTC DESC')")
	cmd.Flags().IntVar(&request.Page, "page", 0, "page number")
	cmd.Flags().IntVar(&request.PageSize, "page-size", 0, "page size (requires --page)")
	cmd.AddCommand(newInvoicesApproveCommand(deps, v))
	cmd.AddCommand(newInvoicesPDFCommand(deps, v))
	cmd.AddCommand(newInvoicesOnlineURLCommand(deps, v))
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
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		candidate := strings.TrimSpace(value)
		if candidate == "" {
			return nil, clierrors.New(clierrors.KindValidation, "--invoice-id values must not be empty")
		}
		if !invoiceIDPattern.MatchString(candidate) {
			return nil, clierrors.New(clierrors.KindValidation, "--invoice-id must be a valid UUID")
		}
		normalized = append(normalized, strings.ToLower(candidate))
	}
	return normalized, nil
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
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if changed {
			return "", clierrors.New(clierrors.KindValidation, "--order must not be empty")
		}
		return defaultInvoiceOrder, nil
	}
	parts := strings.Fields(trimmed)
	if len(parts) != 2 || !orderFieldPattern.MatchString(parts[0]) {
		return "", clierrors.New(clierrors.KindValidation, "--order must use '<Field> <ASC|DESC>'")
	}
	direction := strings.ToUpper(parts[1])
	if direction != "ASC" && direction != "DESC" {
		return "", clierrors.New(clierrors.KindValidation, "--order must use '<Field> <ASC|DESC>'")
	}
	return parts[0] + " " + direction, nil
}
