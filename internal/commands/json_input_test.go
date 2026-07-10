package commands

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

func TestDecodeJSONInputPreservesPresenceAndNumbers(t *testing.T) {
	var input invoiceUpdateInput
	err := decodeJSONInput("-", strings.NewReader(`{"sentToContact":false,"currencyRate":1.2300,"reference":""}`), false, &input)
	if err != nil {
		t.Fatalf("decode input: %v", err)
	}
	if input.SentToContact == nil || *input.SentToContact {
		t.Fatalf("expected explicit false to remain present: %+v", input.SentToContact)
	}
	if input.CurrencyRate == nil || input.CurrencyRate.String() != "1.2300" {
		t.Fatalf("expected lossless number, got %v", input.CurrencyRate)
	}
	if input.Reference == nil || *input.Reference != "" {
		t.Fatalf("expected explicit empty string to remain present: %+v", input.Reference)
	}
	if input.Date != nil {
		t.Fatalf("expected omitted date to remain absent: %+v", input.Date)
	}
}

func TestDecodeJSONInputRejectsInvalidDocuments(t *testing.T) {
	tests := []struct {
		name    string
		content string
		match   string
	}{
		{name: "empty", content: "", match: "one object"},
		{name: "array", content: `[]`, match: "one object"},
		{name: "bom", content: "\xef\xbb\xbf{}", match: "BOM"},
		{name: "invalid utf8", content: string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}), match: "UTF-8"},
		{name: "trailing", content: `{} {}`, match: "trailing"},
		{name: "duplicate top level", content: `{"reference":"a","reference":"b"}`, match: "duplicate key"},
		{name: "duplicate nested", content: `{"lineItems":[{"description":"a","description":"b"}]}`, match: "duplicate key"},
		{name: "null", content: `{"reference":null}`, match: "null"},
		{name: "nested null", content: `{"lineItems":[{"description":"a","tracking":[null]}]}`, match: "null"},
		{name: "unknown", content: `{"type":"ACCREC"}`, match: "unknown field"},
		{name: "wrong key casing", content: `{"ContactID":"` + testUUID + `"}`, match: "unknown field"},
		{name: "wrong nested key casing", content: `{"lineItems":[{"Description":"a"}]}`, match: "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input invoiceUpdateInput
			err := decodeJSONInput("-", strings.NewReader(tt.content), false, &input)
			if clierrors.KindOf(err) != clierrors.KindValidation {
				t.Fatalf("expected validation error, got %v", err)
			}
			if err == nil || !strings.Contains(err.Error(), tt.match) {
				t.Fatalf("expected error containing %q, got %v", tt.match, err)
			}
		})
	}
}

func TestDecodeJSONInputEnforcesStdinAndSizeLimits(t *testing.T) {
	var input invoiceUpdateInput
	if err := decodeJSONInput("-", strings.NewReader(`{}`), true, &input); clierrors.KindOf(err) != clierrors.KindValidation || !strings.Contains(err.Error(), "interactive stdin") {
		t.Fatalf("expected interactive stdin rejection, got %v", err)
	}

	tooLarge := strings.NewReader(strings.Repeat(" ", int(maxJSONInputBytes)+1))
	if err := decodeJSONInput("-", tooLarge, false, &input); clierrors.KindOf(err) != clierrors.KindValidation || !strings.Contains(err.Error(), "1 MiB") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestDecodeJSONInputRequiresRegularPathAndAllowsSymlinkToFile(t *testing.T) {
	tempDir := t.TempDir()
	var input invoiceUpdateInput
	missing := filepath.Join(tempDir, "private-input.json")
	if err := decodeJSONInput(missing, nil, false, &input); clierrors.KindOf(err) != clierrors.KindValidation || strings.Contains(err.Error(), missing) {
		t.Fatalf("expected sanitized missing-file validation error, got %v", err)
	}
	if err := decodeJSONInput(tempDir, nil, false, &input); clierrors.KindOf(err) != clierrors.KindValidation || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected directory rejection, got %v", err)
	}

	path := filepath.Join(tempDir, "input.json")
	if err := os.WriteFile(path, []byte(`{"reference":"PO-1"}`), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	link := filepath.Join(tempDir, "input-link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := decodeJSONInput(link, nil, false, &input); err != nil {
		t.Fatalf("decode symlink target: %v", err)
	}
	if input.Reference == nil || *input.Reference != "PO-1" {
		t.Fatalf("unexpected decoded input: %+v", input)
	}
}

func TestDecodeJSONInputClassifiesReadFailureAsInternal(t *testing.T) {
	var input invoiceUpdateInput
	boom := errors.New("boom")
	err := decodeJSONInput("-", failingReader{err: boom}, false, &input)
	if clierrors.KindOf(err) != clierrors.KindInternal || !errors.Is(err, boom) {
		t.Fatalf("expected internal read error, got %v", err)
	}
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

var _ io.Reader = failingReader{}
