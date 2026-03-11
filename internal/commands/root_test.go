package commands

import (
	"bytes"
	"testing"

	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/spf13/viper"
)

func TestHandleExecuteErrorWritesJSONWhenRequested(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Dependencies{
		Version:   "test",
		IO:        IOStreams{Out: stdout, ErrOut: stderr},
		NewViper:  viper.New,
		IsTerminal: func(int) bool { return false },
	}

	root := NewRootCommand(deps)
	if err := root.PersistentFlags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	handleExecuteError(root, deps, clierrors.New(clierrors.KindXeroAPI, "A validation exception occurred"))

	expected := "{\n  \"ok\": false,\n  \"error\": {\n    \"kind\": \"XeroApiError\",\n    \"message\": \"A validation exception occurred\",\n    \"exitCode\": 14\n  }\n}\n"
	if stdout.String() != expected {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
