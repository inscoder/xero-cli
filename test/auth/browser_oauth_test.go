package auth_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/inscoder/xero-cli/internal/auth"
	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/spf13/viper"
)

func TestBrowserAuthLoginPersistsSessionAndDiscoversTenant(t *testing.T) {
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	port := freePort(t)
	callbackURL := "http://localhost:" + strconv.Itoa(port) + "/callback"
	listenAddress := "localhost:" + strconv.Itoa(port)
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			redirectURI := r.URL.Query().Get("redirect_uri")
			if redirectURI != callbackURL {
				t.Fatalf("unexpected redirect URI: %q", redirectURI)
			}
			state := r.URL.Query().Get("state")
			http.Redirect(w, r, redirectURI+"?code=code-123&state="+state, http.StatusFound)
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("grant_type") != "authorization_code" {
				t.Fatalf("unexpected grant type: %s", r.Form.Get("grant_type"))
			}
			_, _ = io.WriteString(w, `{"access_token":"access-123","refresh_token":"refresh-123","token_type":"Bearer","expires_in":1800,"scope":"offline_access accounting.invoices.read"}`)
		case "/connections":
			if got := r.Header.Get("Authorization"); got != "Bearer access-123" {
				t.Fatalf("unexpected auth header: %q", got)
			}
			_, _ = io.WriteString(w, `[{"tenantId":"tenant-1","tenantName":"Acme","tenantType":"ORGANISATION"}]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer authServer.Close()

	tempDir := t.TempDir()
	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", tempDir+"/config.json")
	v.Set("client_id", "client-123")
	v.Set("auth.scopes", []string{"openid", "profile", "email"})
	v.Set("auth.callback_timeout", "10s")
	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	tokenStore := auth.NewTokenStore(settings)
	sessionStore := auth.NewSessionStore(settings.SessionFilePath)
	tenantStore := auth.NewTenantStore(manager, sessionStore, strings.NewReader(""), io.Discard)
	browserAuth := auth.NewBrowserAuthWithOptions(settings, tokenStore, tenantStore, strings.NewReader(""), io.Discard, auth.BrowserAuthOptions{
		HTTPClient:     authServer.Client(),
		AuthorizeURL:   authServer.URL + "/authorize",
		TokenURL:       authServer.URL + "/token",
		ConnectionsURL: authServer.URL + "/connections",
		RedirectURL:    callbackURL,
		ListenAddress:  listenAddress,
		Now:            func() time.Time { return fixedNow },
		UseManualFlow:  func() bool { return false },
		OpenBrowser: func(target string) error {
			resp, err := authServer.Client().Get(target)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return nil
		},
	})

	result, err := browserAuth.Login(context.Background())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Default.ID != "tenant-1" {
		t.Fatalf("expected default tenant, got %+v", result.Default)
	}
	token, err := tokenStore.Load()
	if err != nil {
		t.Fatalf("load saved token: %v", err)
	}
	if token.AccessToken != "access-123" || token.GeneratedAt != fixedNow || token.TenantID != "tenant-1" {
		t.Fatalf("unexpected saved token: %+v", token)
	}
	meta, err := sessionStore.Load()
	if err != nil {
		t.Fatalf("load session metadata: %v", err)
	}
	if len(meta.KnownTenants) != 1 || meta.KnownTenants[0].ID != "tenant-1" {
		t.Fatalf("unexpected session tenants: %+v", meta.KnownTenants)
	}
	if meta.DefaultTenant.ID != "tenant-1" {
		t.Fatalf("expected session tenant to persist, got %+v", meta.DefaultTenant)
	}
}

func TestBrowserAuthManualLoginPrintsAuthorizationURLAndAcceptsCallbackURL(t *testing.T) {
	fixedNow := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("code") != "manual-code" {
				t.Fatalf("unexpected authorization code: %q", r.Form.Get("code"))
			}
			_, _ = io.WriteString(w, `{"access_token":"manual-access","refresh_token":"manual-refresh","token_type":"Bearer","expires_in":1800,"scope":"offline_access"}`)
		case "/connections":
			_, _ = io.WriteString(w, `[{"tenantId":"tenant-1","tenantName":"Acme","tenantType":"ORGANISATION"}]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer authServer.Close()

	tempDir := t.TempDir()
	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", tempDir+"/config.json")
	v.Set("client_id", "client-123")
	v.Set("auth.callback_timeout", "10s")
	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	settings, err := manager.Load(false, "test")
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	callbackURL := "http://localhost:8742/callback"
	output := &authorizationURLCapture{authorizeBase: authServer.URL + "/authorize"}
	input := &manualCallbackReader{capture: output, callbackURL: callbackURL}
	tokenStore := auth.NewTokenStore(settings)
	sessionStore := auth.NewSessionStore(settings.SessionFilePath)
	tenantStore := auth.NewTenantStore(manager, sessionStore, strings.NewReader(""), io.Discard)
	openCalled := false
	browserAuth := auth.NewBrowserAuthWithOptions(settings, tokenStore, tenantStore, input, output, auth.BrowserAuthOptions{
		HTTPClient:     authServer.Client(),
		AuthorizeURL:   authServer.URL + "/authorize",
		TokenURL:       authServer.URL + "/token",
		ConnectionsURL: authServer.URL + "/connections",
		RedirectURL:    callbackURL,
		Now:            func() time.Time { return fixedNow },
		UseManualFlow:  func() bool { return true },
		OpenBrowser: func(string) error {
			openCalled = true
			return nil
		},
	})

	result, err := browserAuth.Login(context.Background())
	if err != nil {
		t.Fatalf("manual login: %v", err)
	}
	if openCalled {
		t.Fatal("expected manual login not to launch a browser")
	}
	if result.Token.AccessToken != "manual-access" || result.Default.ID != "tenant-1" {
		t.Fatalf("unexpected login result: %+v", result)
	}
	if !strings.Contains(output.String(), "Open this URL in your browser") || !strings.Contains(output.String(), authServer.URL+"/authorize?") {
		t.Fatalf("expected authorization instructions and URL, got %q", output.String())
	}
	if !strings.Contains(output.String(), "paste it here") {
		t.Fatalf("expected callback paste prompt, got %q", output.String())
	}
}

type authorizationURLCapture struct {
	strings.Builder
	authorizeBase string
	authorizeURL  string
}

func (w *authorizationURLCapture) Write(p []byte) (int, error) {
	n, err := w.Builder.Write(p)
	for _, line := range strings.Split(string(p), "\n") {
		if strings.HasPrefix(line, w.authorizeBase+"?") {
			w.authorizeURL = line
		}
	}
	return n, err
}

type manualCallbackReader struct {
	capture     *authorizationURLCapture
	callbackURL string
	reader      *strings.Reader
}

func (r *manualCallbackReader) Read(p []byte) (int, error) {
	if r.reader == nil {
		authorizeURL, err := url.Parse(r.capture.authorizeURL)
		if err != nil {
			return 0, err
		}
		state := authorizeURL.Query().Get("state")
		callback := r.callbackURL + "?code=manual-code&state=" + url.QueryEscape(state) + "\n"
		r.reader = strings.NewReader(callback)
	}
	return r.reader.Read(p)
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestShouldRefreshUsesExpiresAtBuffer(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	fresh := auth.TokenSet{ExpiresAt: now.Add(2 * time.Minute)}
	nearExpiry := auth.TokenSet{ExpiresAt: now.Add(30 * time.Second)}
	expired := auth.TokenSet{ExpiresAt: now.Add(-1 * time.Second)}

	if auth.ShouldRefresh(fresh, now, time.Minute) {
		t.Fatal("expected token outside refresh buffer to skip refresh")
	}
	if !auth.ShouldRefresh(nearExpiry, now, time.Minute) {
		t.Fatal("expected token inside refresh buffer to refresh")
	}
	if !auth.ShouldRefresh(expired, now, time.Minute) {
		t.Fatal("expected expired token to refresh")
	}
}

func TestSessionStoreRejectsCorruptJSON(t *testing.T) {
	tempDir := t.TempDir()
	store := auth.NewSessionStore(tempDir + "/session.json")
	if err := os.WriteFile(tempDir+"/session.json", []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt session: %v", err)
	}
	_, err := store.Load()
	if clierrors.KindOf(err) != clierrors.KindConfigCorrupted {
		t.Fatalf("expected config corrupted error, got %v", err)
	}
}

func TestTenantResolveTokenRequiresSelectedTenant(t *testing.T) {
	tempDir := t.TempDir()
	v := viper.New()
	appconfig.ConfigureViper(v)
	v.Set("config", tempDir+"/config.json")
	manager, err := appconfig.NewManager(v)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Load(false, "test"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	tenantStore := auth.NewTenantStore(manager, auth.NewSessionStore(tempDir+"/session.json"), strings.NewReader(""), io.Discard)
	_, err = tenantStore.ResolveTokenTenant(auth.TokenSet{})
	if clierrors.KindOf(err) != clierrors.KindTenantSelectionRequired {
		t.Fatalf("expected tenant selection required error, got %v", err)
	}
}
