package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cesar/xero-cli/internal/auth"
	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInvoicesPDFCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var request xeroapi.GetInvoicePDFRequest
	var outputPath string

	cmd := &cobra.Command{
		Use:   "pdf",
		Short: "Download an invoice as PDF",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			invoiceID, err := normalizeInvoiceID(request.InvoiceID)
			if err != nil {
				return err
			}

			outputPath = strings.TrimSpace(outputPath)
			if outputPath == "" {
				return clierrors.New(clierrors.KindValidation, "--output must not be empty")
			}

			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}

			if outputPath == "-" {
				if rt.Settings.OutputJSON || rt.Settings.Quiet {
					return clierrors.New(clierrors.KindValidation, "--output - cannot be combined with --json or --quiet")
				}
				if deps.IsTerminal(1) {
					return clierrors.New(clierrors.KindValidation, "refusing to write binary PDF to an interactive terminal; use --output <file> or pipe stdout")
				}
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

			if outputPath == "-" {
				_, err := rt.Xero.GetInvoicePDF(ctx, token, request, rt.IO.Out)
				return err
			}

			result, err := writeInvoicePDFToFile(ctx, rt, token, request, outputPath)
			if err != nil {
				return err
			}

			result.Output = "file"
			result.SavedTo = filepath.Clean(outputPath)
			result.Streamed = false
			summary := "invoice PDF saved"
			breadcrumbs := []output.Breadcrumb{{
				Action: "show",
				Cmd:    fmt.Sprintf("xero invoices pdf --invoice-id %s --output %s --tenant %s --json", result.InvoiceID, result.SavedTo, tenant.ID),
			}}

			return rt.WriteData(result, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoicePDFSaved(w, result)
			})
		},
	}

	cmd.Flags().StringVar(&request.InvoiceID, "invoice-id", "", "invoice ID")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "output file path, or - for stdout")
	_ = cmd.MarkFlagRequired("invoice-id")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func writeInvoicePDFToFile(ctx context.Context, rt *Runtime, token auth.TokenSet, request xeroapi.GetInvoicePDFRequest, outputPath string) (xeroapi.InvoicePDFResult, error) {
	destination := filepath.Clean(outputPath)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return xeroapi.InvoicePDFResult{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "create invoice PDF output directory", err)
	}

	tmpPath := destination + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return xeroapi.InvoicePDFResult{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "create invoice PDF output file", err)
	}

	result, writeErr := rt.Xero.GetInvoicePDF(ctx, token, request, file)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return xeroapi.InvoicePDFResult{}, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return xeroapi.InvoicePDFResult{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "close invoice PDF output file", closeErr)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Remove(tmpPath)
		return xeroapi.InvoicePDFResult{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "move invoice PDF output file into place", err)
	}
	return result, nil
}
