package commands

import (
	"errors"
	"testing"

	"github.com/inscoder/xero-cli/internal/auth"
)

func TestCheckBrowserCommandUsesConfiguredCommand(t *testing.T) {
	check := checkBrowserCommand(func(command string) error {
		if command != "custom-open" {
			t.Fatalf("expected configured command, got %q", command)
		}
		return nil
	}, " custom-open ")

	if check.Status != "ok" || check.Detail != "custom-open" {
		t.Fatalf("unexpected check: %+v", check)
	}
}

func TestCheckBrowserCommandAcceptsFallbackBrowserCandidates(t *testing.T) {
	commands := auth.DefaultBrowserCommands()
	if len(commands) == 0 {
		t.Skip("no default browser candidates on this platform")
	}
	target := commands[len(commands)-1]

	check := checkBrowserCommand(func(command string) error {
		if command == target {
			return nil
		}
		return errors.New("not found")
	}, "")

	if check.Status != "ok" || check.Detail != target {
		t.Fatalf("unexpected check: %+v", check)
	}
}
