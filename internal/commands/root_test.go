package commands

import (
	"bytes"
	"testing"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	appversion "github.com/inscoder/xero-cli/internal/version"
	"github.com/spf13/viper"
)

func TestHandleExecuteErrorWritesJSONWhenRequested(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Dependencies{
		Version:    "test",
		IO:         IOStreams{Out: stdout, ErrOut: stderr},
		NewViper:   viper.New,
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

func TestRootCommandVersionFlagWritesVersion(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Dependencies{
		Version:    "0.1.0",
		IO:         IOStreams{Out: stdout, ErrOut: stderr},
		NewViper:   viper.New,
		IsTerminal: func(int) bool { return false },
	}

	cmd := NewRootCommand(deps)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version flag: %v", err)
	}
	if stdout.String() != "xero 0.1.0\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestVersionCommandWritesJSON(t *testing.T) {
	originalCommit := appversion.Commit
	originalDate := appversion.Date
	appversion.Commit = "abc1234"
	appversion.Date = "2026-03-11T12:00:00Z"
	t.Cleanup(func() {
		appversion.Commit = originalCommit
		appversion.Date = originalDate
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deps := Dependencies{
		Version:    "0.1.0",
		IO:         IOStreams{Out: stdout, ErrOut: stderr},
		NewViper:   viper.New,
		IsTerminal: func(int) bool { return false },
	}

	cmd := NewRootCommand(deps)
	cmd.SetArgs([]string{"version", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	expected := "{\n  \"ok\": true,\n  \"data\": {\n    \"version\": \"0.1.0\",\n    \"commit\": \"abc1234\",\n    \"date\": \"2026-03-11T12:00:00Z\"\n  },\n  \"summary\": \"Version information\"\n}\n"
	if stdout.String() != expected {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
