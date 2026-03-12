package auth

import (
	"errors"
	"net"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

func TestOpenBrowserUsesConfiguredCommand(t *testing.T) {
	originalStart := startBrowserProcess
	startBrowserProcess = func(command string, args ...string) error {
		if command != "xdg-open" {
			t.Fatalf("expected configured command, got %q", command)
		}
		if len(args) != 1 || args[0] != "http://localhost:3000/callback" {
			t.Fatalf("unexpected args: %#v", args)
		}
		return nil
	}
	t.Cleanup(func() { startBrowserProcess = originalStart })

	if err := openBrowser(" xdg-open ")("http://localhost:3000/callback"); err != nil {
		t.Fatalf("open browser with configured command: %v", err)
	}
}

func TestOpenBrowserWithProvidersStartsFirstAvailableCommand(t *testing.T) {
	originalLookPath := browserCommandLookPath
	originalStart := startBrowserProcess
	browserCommandLookPath = func(command string) (string, error) {
		switch command {
		case "xdg-open":
			return "/usr/bin/xdg-open", nil
		default:
			return "", exec.ErrNotFound
		}
	}
	startBrowserProcess = func(command string, args ...string) error {
		if command != "xdg-open" {
			t.Fatalf("expected xdg-open, got %q", command)
		}
		if len(args) != 1 || args[0] != "https://example.com" {
			t.Fatalf("unexpected args: %#v", args)
		}
		return nil
	}
	t.Cleanup(func() {
		browserCommandLookPath = originalLookPath
		startBrowserProcess = originalStart
	})

	if err := openBrowserWithProviders("https://example.com", linuxBrowserCommands); err != nil {
		t.Fatalf("open browser with default providers: %v", err)
	}
}

func TestOpenBrowserWithProvidersReturnsNotFoundWhenNoCommandExists(t *testing.T) {
	originalLookPath := browserCommandLookPath
	browserCommandLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { browserCommandLookPath = originalLookPath })

	err := openBrowserWithProviders("https://example.com", linuxBrowserCommands)
	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected exec error, got %v", err)
	}
	if execErr.Err != exec.ErrNotFound {
		t.Fatalf("expected command not found, got %v", execErr.Err)
	}
}

func TestListenLoopbackReturnsErrorWhenLoopbackPortAlreadyInUse(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ipv4 loopback: %v", err)
	}
	defer busy.Close()

	port := busy.Addr().(*net.TCPAddr).Port
	_, err = listenLoopback(net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err == nil {
		t.Fatal("expected localhost loopback bind to fail when one family is already in use")
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("expected address in use error, got %v", err)
	}
}

func TestListenLoopbackBindsAvailableLoopbackListeners(t *testing.T) {
	port := freeLoopbackPort(t)
	listeners, err := listenLoopback(net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("listen on localhost loopback: %v", err)
	}
	defer closeListeners(listeners)
	if len(listeners) == 0 {
		t.Fatal("expected at least one loopback listener")
	}
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free loopback port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
