package auth

import (
	"errors"
	"net"
	"strconv"
	"syscall"
	"testing"
)

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
