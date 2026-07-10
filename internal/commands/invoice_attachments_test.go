package commands

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

func TestOpenAttachmentUploadFileValidatesAndDetectsContentType(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "receipt.pdf")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	upload, err := openAttachmentUploadFile(path, "remote receipt.pdf", true, "", false)
	if err != nil {
		t.Fatalf("open attachment: %v", err)
	}
	defer upload.file.Close()
	if upload.fileName != "remote receipt.pdf" || upload.contentType != "application/pdf" || upload.size != 4 {
		t.Fatalf("unexpected upload metadata: %+v", upload)
	}

	override, err := openAttachmentUploadFile(path, "", false, "application/pdf; profile=test", true)
	if err != nil {
		t.Fatalf("open attachment with override: %v", err)
	}
	defer override.file.Close()
	if override.contentType != "application/pdf; profile=test" {
		t.Fatalf("unexpected override: %q", override.contentType)
	}
}

func TestOpenAttachmentUploadFileRejectsUnsafeSources(t *testing.T) {
	tempDir := t.TempDir()
	empty := filepath.Join(tempDir, "empty.pdf")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	large := filepath.Join(tempDir, "large.pdf")
	file, err := os.Create(large)
	if err != nil {
		t.Fatalf("create large file: %v", err)
	}
	if err := file.Truncate(maxAttachmentBytes + 1); err != nil {
		t.Fatalf("truncate large file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close large file: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "stdin", path: "-", want: "not supported"},
		{name: "missing", path: filepath.Join(tempDir, "missing.pdf"), want: "missing or unreadable"},
		{name: "directory", path: tempDir, want: "regular file"},
		{name: "empty", path: empty, want: "must not be empty"},
		{name: "oversize", path: large, want: "10,000,000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := openAttachmentUploadFile(tt.path, "", false, "", false)
			if clierrors.KindOf(err) != clierrors.KindValidation || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected validation error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateAttachmentFileName(t *testing.T) {
	for _, valid := range []string{"receipt.pdf", "résumé #1%[final].pdf", "two words.png"} {
		if err := validateAttachmentFileName(valid); err != nil {
			t.Fatalf("expected %q to be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", ".", "..", "../receipt.pdf", "a/b.pdf", "a\\b.pdf", "bad+name.pdf", "bad:name.pdf", "bad\x00name.pdf"} {
		if err := validateAttachmentFileName(invalid); clierrors.KindOf(err) != clierrors.KindValidation {
			t.Fatalf("expected %q to be invalid, got %v", invalid, err)
		}
	}
}

func TestResolveAttachmentContentTypeRejectsInvalidOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer file.Close()
	if _, err := resolveAttachmentContentType(file, "file.bin", "not-a-type", true); clierrors.KindOf(err) != clierrors.KindValidation {
		t.Fatalf("expected invalid content type, got %v", err)
	}
}

func TestExactSizeReaderDetectsGrowthAndTruncation(t *testing.T) {
	reader := &exactSizeReader{reader: bytes.NewBufferString("abc"), remaining: 3}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "abc" {
		t.Fatalf("read exact content: data=%q err=%v", data, err)
	}

	grown := &exactSizeReader{reader: bytes.NewBufferString("abcd"), remaining: 3}
	if _, err := io.ReadAll(grown); err == nil || !strings.Contains(err.Error(), "grew") {
		t.Fatalf("expected growth error, got %v", err)
	}

	short := &exactSizeReader{reader: bytes.NewBufferString("ab"), remaining: 3}
	if _, err := io.ReadAll(short); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected truncation error, got %v", err)
	}
}
