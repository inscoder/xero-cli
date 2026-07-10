package commands

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	maxAttachmentBytes = int64(10_000_000)
	maxAttachments     = 10
)

type attachmentUploadFile struct {
	file        *os.File
	fileName    string
	contentType string
	size        int64
}

func newInvoiceAttachmentsCommand(deps Dependencies, v *viper.Viper, config invoiceCommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachments",
		Short: "Manage attachments on a Xero " + config.Singular,
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newInvoiceAttachmentUploadCommand(deps, v, config))
	return cmd
}

func newInvoiceAttachmentUploadCommand(deps Dependencies, v *viper.Viper, config invoiceCommandConfig) *cobra.Command {
	var invoiceID string
	var filePath string
	var fileName string
	var contentType string
	var overwrite bool
	var includeOnline bool
	var idempotencyKey string

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload an attachment to one Xero " + config.Singular,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedInvoiceID, err := normalizeInvoiceID(invoiceID)
			if err != nil {
				return err
			}
			upload, err := openAttachmentUploadFile(
				filePath,
				fileName,
				cmd.Flags().Changed("filename"),
				contentType,
				cmd.Flags().Changed("content-type"),
			)
			if err != nil {
				return err
			}
			defer upload.file.Close()

			if includeOnline && overwrite {
				return clierrors.New(clierrors.KindValidation, "--include-online cannot be combined with --overwrite")
			}
			if includeOnline && config.Type != invoiceTypeSales {
				return clierrors.New(clierrors.KindValidation, "--include-online is available only for sales invoices")
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
			invoice, err := rt.Xero.GetInvoice(ctx, token, xeroapi.GetInvoiceRequest{TenantID: tenant.ID, InvoiceID: normalizedInvoiceID})
			if err != nil {
				return err
			}
			if err := validateAttachmentInvoicePreflight(invoice, normalizedInvoiceID, config); err != nil {
				return err
			}
			collision := attachmentNameCollision(invoice.Attachments, upload.fileName)
			switch {
			case collision && !overwrite:
				return clierrors.New(clierrors.KindValidation, fmt.Sprintf("attachment %q already exists; pass --overwrite to replace it", upload.fileName))
			case !collision && overwrite:
				return clierrors.New(clierrors.KindValidation, fmt.Sprintf("attachment %q does not exist; remove --overwrite to upload it", upload.fileName))
			case !collision && len(invoice.Attachments) >= maxAttachments:
				return clierrors.New(clierrors.KindValidation, "invoice already has 10 attachments; remove one in Xero before uploading another")
			}
			if err := resetAndRevalidateAttachmentUpload(upload); err != nil {
				return err
			}

			result, err := rt.Xero.UploadInvoiceAttachment(ctx, token, xeroapi.UploadInvoiceAttachmentRequest{
				TenantID:       tenant.ID,
				Resource:       config.Singular,
				Namespace:      config.Namespace,
				Type:           config.Type,
				InvoiceID:      normalizedInvoiceID,
				FileName:       upload.fileName,
				ContentType:    upload.contentType,
				ContentLength:  upload.size,
				IdempotencyKey: effectiveKey,
				IncludeOnline:  includeOnline,
				Replace:        overwrite,
				Body:           &exactSizeReader{reader: upload.file, remaining: upload.size},
			})
			if err != nil {
				return err
			}

			summary := fmt.Sprintf("%s attachment %s", config.Singular, result.Operation)
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: fmt.Sprintf("xero %s --invoice-id %s --json", config.Namespace, normalizedInvoiceID)}}
			return rt.WriteData(result, summary, breadcrumbs, func(w io.Writer) error {
				return output.WriteInvoiceAttachmentMutation(w, result)
			})
		},
	}
	cmd.Flags().StringVar(&invoiceID, "invoice-id", "", config.Singular+" invoice ID")
	cmd.Flags().StringVar(&filePath, "file", "", "local attachment file path")
	cmd.Flags().StringVar(&fileName, "filename", "", "remote attachment filename (defaults to the local basename)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "attachment MIME type override")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace an existing attachment with the same filename")
	if config.Type == invoiceTypeSales {
		cmd.Flags().BoolVar(&includeOnline, "include-online", false, "include a new attachment on the online sales invoice")
	}
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for an exact retry (1-128 bytes)")
	_ = cmd.MarkFlagRequired("invoice-id")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func openAttachmentUploadFile(path, overrideName string, overrideNameSet bool, contentTypeOverride string, contentTypeSet bool) (attachmentUploadFile, error) {
	if path == "-" {
		return attachmentUploadFile{}, clierrors.New(clierrors.KindValidation, "--file - is not supported for attachment uploads; provide a regular file path")
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		return attachmentUploadFile{}, clierrors.New(clierrors.KindValidation, "attachment file is missing or unreadable")
	}
	if !pathInfo.Mode().IsRegular() {
		return attachmentUploadFile{}, clierrors.New(clierrors.KindValidation, "attachment must be a regular file")
	}
	if pathInfo.Size() == 0 {
		return attachmentUploadFile{}, clierrors.New(clierrors.KindValidation, "attachment file must not be empty")
	}
	if pathInfo.Size() > maxAttachmentBytes {
		return attachmentUploadFile{}, clierrors.New(clierrors.KindValidation, "attachment file must not exceed 10,000,000 bytes")
	}
	file, err := os.Open(path)
	if err != nil {
		return attachmentUploadFile{}, clierrors.New(clierrors.KindValidation, "attachment file is missing or unreadable")
	}
	closeWithError := func(err error) (attachmentUploadFile, error) {
		_ = file.Close()
		return attachmentUploadFile{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return closeWithError(clierrors.New(clierrors.KindInternal, "inspect attachment file"))
	}
	if !info.Mode().IsRegular() {
		return closeWithError(clierrors.New(clierrors.KindValidation, "attachment must be a regular file"))
	}
	if info.Size() == 0 {
		return closeWithError(clierrors.New(clierrors.KindValidation, "attachment file must not be empty"))
	}
	if info.Size() > maxAttachmentBytes {
		return closeWithError(clierrors.New(clierrors.KindValidation, "attachment file must not exceed 10,000,000 bytes"))
	}

	fileName := filepath.Base(path)
	if overrideNameSet {
		fileName = overrideName
	}
	if err := validateAttachmentFileName(fileName); err != nil {
		return closeWithError(err)
	}
	contentType, err := resolveAttachmentContentType(file, fileName, contentTypeOverride, contentTypeSet)
	if err != nil {
		return closeWithError(err)
	}
	return attachmentUploadFile{file: file, fileName: fileName, contentType: contentType, size: info.Size()}, nil
}

func validateAttachmentFileName(fileName string) error {
	trimmed := strings.TrimSpace(fileName)
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return clierrors.New(clierrors.KindValidation, "attachment filename must be a non-empty basename other than . or ..")
	}
	if strings.ContainsAny(fileName, `/\\`) || filepath.Base(fileName) != fileName {
		return clierrors.New(clierrors.KindValidation, "attachment filename must be a basename without path separators")
	}
	for _, character := range fileName {
		if unicode.IsControl(character) || strings.ContainsRune(`<>:"/\\|?*+`, character) {
			return clierrors.New(clierrors.KindValidation, "attachment filename contains a control character or a Xero-prohibited character")
		}
	}
	return nil
}

func resolveAttachmentContentType(file *os.File, fileName, override string, overrideSet bool) (string, error) {
	if overrideSet {
		mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(override))
		if err != nil || strings.Count(mediaType, "/") != 1 || strings.HasPrefix(mediaType, "/") || strings.HasSuffix(mediaType, "/") {
			return "", clierrors.New(clierrors.KindValidation, "--content-type must be a valid MIME media type")
		}
		formatted := mime.FormatMediaType(mediaType, parameters)
		if formatted == "" {
			return "", clierrors.New(clierrors.KindValidation, "--content-type must be a valid MIME media type")
		}
		return formatted, nil
	}

	var sniff [512]byte
	n, err := io.ReadFull(file, sniff[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", clierrors.New(clierrors.KindInternal, "read attachment for MIME detection")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", clierrors.New(clierrors.KindInternal, "reset attachment after MIME detection")
	}
	detected := http.DetectContentType(sniff[:n])
	if detected != "application/octet-stream" {
		return detected, nil
	}
	if byExtension := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName))); byExtension != "" {
		return byExtension, nil
	}
	return "application/octet-stream", nil
}

func validateAttachmentInvoicePreflight(invoice xeroapi.Invoice, invoiceID string, config invoiceCommandConfig) error {
	if strings.TrimSpace(invoice.InvoiceID) == "" || !strings.EqualFold(invoice.InvoiceID, invoiceID) {
		return clierrors.New(clierrors.KindXeroRequest, "Xero invoice preflight did not match the requested invoice ID")
	}
	if !strings.EqualFold(invoice.Type, config.Type) {
		return clierrors.New(clierrors.KindValidation, fmt.Sprintf("invoice %s has Type %s and cannot receive an attachment through `xero %s`; expected %s", invoiceID, invoice.Type, config.Namespace, config.Type))
	}
	return nil
}

func attachmentNameCollision(attachments []xeroapi.InvoiceAttachment, fileName string) bool {
	for _, attachment := range attachments {
		if strings.EqualFold(attachment.FileName, fileName) {
			return true
		}
	}
	return false
}

func resetAndRevalidateAttachmentUpload(upload attachmentUploadFile) error {
	info, err := upload.file.Stat()
	if err != nil {
		return clierrors.New(clierrors.KindInternal, "reinspect attachment file before upload")
	}
	if !info.Mode().IsRegular() || info.Size() != upload.size || info.Size() <= 0 || info.Size() > maxAttachmentBytes {
		return clierrors.New(clierrors.KindValidation, "attachment file changed after validation; rerun the command")
	}
	if _, err := upload.file.Seek(0, io.SeekStart); err != nil {
		return clierrors.New(clierrors.KindInternal, "reset attachment before upload")
	}
	return nil
}

type exactSizeReader struct {
	reader    io.Reader
	remaining int64
	verified  bool
}

func (reader *exactSizeReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		if reader.verified {
			return 0, io.EOF
		}
		reader.verified = true
		var extra [1]byte
		n, err := reader.reader.Read(extra[:])
		if n != 0 {
			return 0, errors.New("attachment file grew after validation")
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		return 0, io.EOF
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	n, err := reader.reader.Read(buffer)
	reader.remaining -= int64(n)
	if err != nil {
		if errors.Is(err, io.EOF) && reader.remaining != 0 {
			return n, io.ErrUnexpectedEOF
		}
		return n, err
	}
	if n == 0 {
		return 0, io.ErrNoProgress
	}
	if reader.remaining == 0 {
		reader.verified = true
		var extra [1]byte
		extraN, extraErr := reader.reader.Read(extra[:])
		if extraN != 0 {
			return n, errors.New("attachment file grew after validation")
		}
		if extraErr != nil && !errors.Is(extraErr, io.EOF) {
			return n, extraErr
		}
	}
	return n, nil
}
