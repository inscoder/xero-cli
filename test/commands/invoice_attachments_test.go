package commands_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	"github.com/inscoder/xero-cli/internal/commands"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/xeroapi"
)

const attachmentInvoiceID = "220ddca8-3144-4085-9a88-2d72c5133734"

func TestInvoiceAttachmentUploadUsesValidatedFileAndNamespace(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))
	filePath := filepath.Join(tempDir, "local.pdf")
	content := []byte("%PDF-1.7\nreceipt\n")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	client := &fakeLister{
		invoices: []xeroapi.Invoice{{InvoiceID: attachmentInvoiceID, Type: "ACCREC", Attachments: []xeroapi.InvoiceAttachment{}}},
		uploadResult: xeroapi.InvoiceAttachmentMutationResult{
			Operation: "uploaded", Resource: "invoice", InvoiceID: attachmentInvoiceID, TenantID: "tenant-1", Type: "ACCREC",
			AttachmentID: "88192a99-cbc5-4a66-bf1a-2f9fea2d36d0", FileName: "résumé #1%.pdf", ContentType: "application/pdf",
			Bytes: int64(len(content)), IncludeOnline: boolResultPointer(true), IdempotencyKey: "attachment-key",
		},
	}
	deps, stdout, stderr := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token", GeneratedAt: time.Now().UTC()}}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{
		"--config", configPath, "invoices", "attachments", "upload",
		"--invoice-id", attachmentInvoiceID,
		"--file", filePath,
		"--filename", "résumé #1%.pdf",
		"--content-type", "application/pdf",
		"--include-online",
		"--idempotency-key", "attachment-key",
		"--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute upload: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if client.getRequest.InvoiceID != attachmentInvoiceID || client.uploadCalls != 1 {
		t.Fatalf("expected one preflight and upload: get=%+v uploads=%d", client.getRequest, client.uploadCalls)
	}
	if client.uploadReq.Resource != "invoice" || client.uploadReq.Type != "ACCREC" || client.uploadReq.Replace || !client.uploadReq.IncludeOnline {
		t.Fatalf("unexpected upload request: %+v", client.uploadReq)
	}
	if client.uploadReq.FileName != "résumé #1%.pdf" || client.uploadReq.ContentType != "application/pdf" || client.uploadReq.ContentLength != int64(len(content)) || client.uploadReq.IdempotencyKey != "attachment-key" {
		t.Fatalf("unexpected file metadata: %+v", client.uploadReq)
	}
	if string(client.uploadBody) != string(content) {
		t.Fatalf("unexpected upload body: %q", client.uploadBody)
	}
	if !strings.Contains(stdout.String(), "\"includeOnline\": true") || !strings.Contains(stdout.String(), "xero invoices --invoice-id "+attachmentInvoiceID+" --json") {
		t.Fatalf("unexpected JSON output: %s", stdout.String())
	}
}

func TestAttachmentUploadCollisionAndOverwriteRules(t *testing.T) {
	tests := []struct {
		name        string
		attachments []xeroapi.InvoiceAttachment
		overwrite   bool
		wantError   string
		wantUploads int
	}{
		{name: "collision rejected", attachments: []xeroapi.InvoiceAttachment{{FileName: "Receipt.PDF"}}, wantError: "already exists"},
		{name: "collision replaced", attachments: []xeroapi.InvoiceAttachment{{FileName: "Receipt.PDF"}}, overwrite: true, wantUploads: 1},
		{name: "overwrite target missing", overwrite: true, wantError: "does not exist"},
		{name: "eleventh rejected", attachments: tenAttachments(), wantError: "already has 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.json")
			prepareConfig(t, configPath)
			prepareSession(t, filepath.Join(tempDir, "session.json"))
			filePath := filepath.Join(tempDir, "receipt.pdf")
			if err := os.WriteFile(filePath, []byte("receipt"), 0o600); err != nil {
				t.Fatalf("write attachment: %v", err)
			}
			client := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: attachmentInvoiceID, Type: "ACCPAY", Attachments: tt.attachments}}}
			deps, stdout, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, client, false)
			args := []string{"--config", configPath, "bills", "attachments", "upload", "--invoice-id", attachmentInvoiceID, "--file", filePath, "--idempotency-key", "key"}
			if tt.overwrite {
				args = append(args, "--overwrite")
			}
			cmd := commands.NewRootCommand(deps)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if tt.wantError != "" {
				if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Fatalf("execute upload: %v", err)
			}
			if client.uploadCalls != tt.wantUploads {
				t.Fatalf("unexpected uploads: got %d want %d", client.uploadCalls, tt.wantUploads)
			}
			if tt.wantUploads == 1 {
				if !client.uploadReq.Replace || !strings.Contains(stdout.String(), "Replaced attachment") || !strings.Contains(stdout.String(), "Idempotency key: key") {
					t.Fatalf("expected replacement request/output: request=%+v output=%q", client.uploadReq, stdout.String())
				}
			}
		})
	}
}

func TestAttachmentUploadRejectsWrongNamespaceBeforeUpload(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	prepareConfig(t, configPath)
	prepareSession(t, filepath.Join(tempDir, "session.json"))
	filePath := filepath.Join(tempDir, "receipt.pdf")
	if err := os.WriteFile(filePath, []byte("receipt"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	client := &fakeLister{invoices: []xeroapi.Invoice{{InvoiceID: attachmentInvoiceID, Type: "ACCREC"}}}
	deps, _, _ := testDependencies(configPath, &fakeStore{token: auth.TokenSet{AccessToken: "token"}}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", configPath, "bills", "attachments", "upload", "--invoice-id", attachmentInvoiceID, "--file", filePath})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), "ACCPAY") {
		t.Fatalf("expected namespace error, got %v", err)
	}
	if client.uploadCalls != 0 {
		t.Fatalf("expected no upload, got %d", client.uploadCalls)
	}
}

func TestBillAttachmentCommandDoesNotExposeIncludeOnline(t *testing.T) {
	deps, _, _ := testDependencies(filepath.Join(t.TempDir(), "config.json"), &fakeStore{}, &fakeLister{}, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"bills", "attachments", "upload", "--include-online"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --include-online") {
		t.Fatalf("expected unknown include-online flag, got %v", err)
	}
}

func TestAttachmentFileValidationRunsBeforeRuntime(t *testing.T) {
	tempDir := t.TempDir()
	client := &fakeLister{}
	deps, _, _ := testDependencies(filepath.Join(tempDir, "missing-config.json"), &fakeStore{}, client, false)
	cmd := commands.NewRootCommand(deps)
	cmd.SetArgs([]string{"--config", filepath.Join(tempDir, "missing-config.json"), "invoices", "attachments", "upload", "--invoice-id", attachmentInvoiceID, "--file", "-"})
	err := cmd.Execute()
	if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected local validation error, got %v", err)
	}
	if client.getRequest.InvoiceID != "" || client.uploadCalls != 0 {
		t.Fatalf("expected no client calls: get=%+v uploads=%d", client.getRequest, client.uploadCalls)
	}
}

func tenAttachments() []xeroapi.InvoiceAttachment {
	attachments := make([]xeroapi.InvoiceAttachment, 10)
	for index := range attachments {
		attachments[index].FileName = "existing-" + string(rune('a'+index)) + ".pdf"
	}
	return attachments
}

func boolResultPointer(value bool) *bool {
	return &value
}
