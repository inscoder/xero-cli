package auth

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestDefaultBrowserCommands(t *testing.T) {
	commands := DefaultBrowserCommands()
	switch runtime.GOOS {
	case "linux":
		expected := []string{"xdg-open", "x-www-browser", "www-browser"}
		if len(commands) != len(expected) {
			t.Fatalf("expected %d commands, got %d", len(expected), len(commands))
		}
		for index, command := range expected {
			if commands[index] != command {
				t.Fatalf("expected command %d to be %q, got %q", index, command, commands[index])
			}
		}
	case "darwin":
		if len(commands) != 1 || commands[0] != "open" {
			t.Fatalf("expected macOS opener, got %#v", commands)
		}
	default:
		if len(commands) != 0 {
			t.Fatalf("expected no default commands on %s, got %#v", runtime.GOOS, commands)
		}
	}
}

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

func TestStartBrowserCommandReturnsQuickExitError(t *testing.T) {
	originalNewCommand := newBrowserCommand
	originalTimeout := browserStartTimeout
	newBrowserCommand = helperBrowserCommand("fail")
	browserStartTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		newBrowserCommand = originalNewCommand
		browserStartTimeout = originalTimeout
	})

	err := startBrowserCommand("helper")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}
}

func TestStartBrowserCommandReturnsAfterStartupTimeout(t *testing.T) {
	originalNewCommand := newBrowserCommand
	originalTimeout := browserStartTimeout
	newBrowserCommand = helperBrowserCommand("sleep")
	browserStartTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		newBrowserCommand = originalNewCommand
		browserStartTimeout = originalTimeout
	})

	started := time.Now()
	if err := startBrowserCommand("helper"); err != nil {
		t.Fatalf("expected delayed browser command to be treated as started, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("expected browser start to return quickly, took %s", elapsed)
	}
}

func helperBrowserCommand(mode string) func(string, ...string) *exec.Cmd {
	return func(_ string, _ ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperBrowserProcess", "--", mode)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
}

func TestHelperBrowserProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	mode := ""
	for index, arg := range os.Args {
		if arg == "--" && index+1 < len(os.Args) {
			mode = os.Args[index+1]
			break
		}
	}
	switch mode {
	case "fail":
		os.Exit(1)
	case "sleep":
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	default:
		os.Exit(0)
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
