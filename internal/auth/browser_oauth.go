package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/pkg/browser"
)

const (
	authorizeEndpoint = "https://login.xero.com/identity/connect/authorize"
	tokenEndpoint     = "https://identity.xero.com/connect/token"
	connectionsURL    = "https://api.xero.com/connections"
	redirectURL       = "http://localhost:3000/callback"
	listenAddress     = "localhost:3000"
)

type BrowserAuth struct {
	settings       appconfig.Settings
	httpClient     *http.Client
	openBrowser    func(string) error
	now            func() time.Time
	authorizeURL   string
	tokenURL       string
	connectionsURL string
	redirectURL    string
	listenAddress  string
	errOut         io.Writer
	in             io.Reader
	tenantStore    *TenantStore
	store          TokenStore
}

type BrowserAuthOptions struct {
	HTTPClient     *http.Client
	OpenBrowser    func(string) error
	Now            func() time.Time
	AuthorizeURL   string
	TokenURL       string
	ConnectionsURL string
	RedirectURL    string
	ListenAddress  string
}

type LoginResult struct {
	Token   TokenSet
	Tenants []Tenant
	Default Tenant
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

type connectionResponse struct {
	TenantID   string `json:"tenantId"`
	TenantName string `json:"tenantName"`
	TenantType string `json:"tenantType"`
}

func NewBrowserAuth(settings appconfig.Settings, store TokenStore, tenantStore *TenantStore, in io.Reader, errOut io.Writer) *BrowserAuth {
	return NewBrowserAuthWithOptions(settings, store, tenantStore, in, errOut, BrowserAuthOptions{})
}

func NewBrowserAuthWithOptions(settings appconfig.Settings, store TokenStore, tenantStore *TenantStore, in io.Reader, errOut io.Writer, options BrowserAuthOptions) *BrowserAuth {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	open := options.OpenBrowser
	if open == nil {
		open = openBrowser(settings.OpenCommand)
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &BrowserAuth{
		settings:       settings,
		httpClient:     client,
		openBrowser:    open,
		now:            now,
		authorizeURL:   firstNonEmpty(options.AuthorizeURL, authorizeEndpoint),
		tokenURL:       firstNonEmpty(options.TokenURL, tokenEndpoint),
		connectionsURL: firstNonEmpty(options.ConnectionsURL, connectionsURL),
		redirectURL:    firstNonEmpty(options.RedirectURL, redirectURL),
		listenAddress:  firstNonEmpty(options.ListenAddress, listenAddress),
		errOut:         errOut,
		in:             in,
		tenantStore:    tenantStore,
		store:          store,
	}
}

func openBrowser(command string) func(string) error {
	if strings.TrimSpace(command) == "" {
		return browser.OpenURL
	}
	return func(target string) error {
		cmd := exec.Command(command, target)
		return cmd.Start()
	}
}

func (a *BrowserAuth) Login(ctx context.Context) (LoginResult, error) {
	if err := appconfig.ValidateLoginConfig(a.settings); err != nil {
		return LoginResult{}, err
	}

	codeVerifier, err := randomToken(48)
	if err != nil {
		return LoginResult{}, clierrors.Wrap(clierrors.KindInternal, "generate PKCE verifier", err)
	}
	state, err := randomToken(32)
	if err != nil {
		return LoginResult{}, clierrors.Wrap(clierrors.KindInternal, "generate OAuth state", err)
	}

	callbackURL, err := url.Parse(a.redirectURL)
	if err != nil {
		return LoginResult{}, clierrors.Wrap(clierrors.KindValidation, "parse configured OAuth redirect URL", err)
	}
	listeners, err := listenLoopback(a.listenAddress)
	if err != nil {
		return LoginResult{}, clierrors.Wrap(clierrors.KindNetwork, fmt.Sprintf("start OAuth callback listener on %s", a.listenAddress), err)
	}
	defer closeListeners(listeners)

	authURL := buildAuthorizeURL(a.authorizeURL, a.settings.ClientID, a.redirectURL, a.settings.XeroScopes, state, codeVerifier)

	ctx, cancel := context.WithTimeout(ctx, a.settings.CallbackTimeout)
	defer cancel()

	resultCh := make(chan url.Values, 1)
	errCh := make(chan error, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != callbackURL.Path {
			http.NotFound(w, r)
			return
		}
		values := r.URL.Query()
		if values.Get("state") != state {
			http.Error(w, "OAuth state mismatch", http.StatusBadRequest)
			errCh <- clierrors.New(clierrors.KindAuthRequired, "received OAuth callback with invalid state")
			return
		}
		_, _ = w.Write([]byte("Xero CLI login complete. You can return to the terminal."))
		resultCh <- values
	})}

	for _, listener := range listeners {
		go func(listener net.Listener) {
			if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
				errCh <- clierrors.Wrap(clierrors.KindNetwork, "serve OAuth callback", serveErr)
			}
		}(listener)
	}
	defer server.Shutdown(context.Background())

	if err := a.openBrowser(authURL); err != nil {
		fmt.Fprintf(a.errOut, "Open this URL to authorize Xero CLI:\n%s\n", authURL)
		fmt.Fprintln(a.errOut, "If the browser cannot redirect back automatically, paste the final redirected URL here:")
		pasted, parseErr := readManualRedirect(ctx, a.in)
		if parseErr != nil {
			return LoginResult{}, parseErr
		}
		if pasted.Get("state") != state {
			return LoginResult{}, clierrors.New(clierrors.KindAuthRequired, "pasted redirect URL had an invalid OAuth state")
		}
		return a.finishLogin(ctx, a.redirectURL, codeVerifier, pasted.Get("code"))
	}

	fmt.Fprintln(a.errOut, "Waiting for browser authentication callback...")
	select {
	case <-ctx.Done():
		return LoginResult{}, clierrors.Wrap(clierrors.KindAuthRequired, "timed out waiting for Xero browser login", ctx.Err())
	case err := <-errCh:
		return LoginResult{}, err
	case values := <-resultCh:
		return a.finishLogin(ctx, a.redirectURL, codeVerifier, values.Get("code"))
	}
}

func (a *BrowserAuth) EnsureFreshToken(ctx context.Context, token TokenSet, interactive bool) (TokenSet, bool, error) {
	if !ShouldRefresh(token, a.now(), a.settings.RefreshAfter) {
		return token, false, nil
	}
	refreshed, err := a.refreshToken(ctx, token)
	if err == nil {
		if saveErr := a.store.Save(refreshed); saveErr != nil {
			return TokenSet{}, false, saveErr
		}
		return refreshed, true, nil
	}
	if interactive && !a.settings.NoBrowser {
		login, loginErr := a.Login(ctx)
		if loginErr != nil {
			return TokenSet{}, false, clierrors.Wrap(clierrors.KindTokenRefreshFailed, "token refresh failed and browser re-authentication also failed", loginErr)
		}
		return login.Token, true, nil
	}
	return TokenSet{}, false, clierrors.Wrap(clierrors.KindTokenRefreshFailed, "token refresh failed", err)
}

func ShouldRefresh(token TokenSet, now time.Time, threshold time.Duration) bool {
	if token.GeneratedAt.IsZero() {
		return true
	}
	return now.Sub(token.GeneratedAt) > threshold
}

func buildAuthorizeURL(baseURL, clientID, redirectURI string, scopes []string, state, verifier string) string {
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", strings.Join(scopes, " "))
	values.Set("state", state)
	values.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	values.Set("code_challenge_method", "S256")
	return baseURL + "?" + values.Encode()
}

func readManualRedirect(ctx context.Context, in io.Reader) (url.Values, error) {
	type response struct {
		values url.Values
		err    error
	}
	resultCh := make(chan response, 1)
	go func() {
		var line string
		_, err := fmt.Fscanln(in, &line)
		if err != nil {
			resultCh <- response{err: clierrors.Wrap(clierrors.KindValidation, "read pasted redirect URL", err)}
			return
		}
		parsed, parseErr := url.Parse(strings.TrimSpace(line))
		if parseErr != nil {
			resultCh <- response{err: clierrors.Wrap(clierrors.KindValidation, "parse pasted redirect URL", parseErr)}
			return
		}
		resultCh <- response{values: parsed.Query()}
	}()

	select {
	case <-ctx.Done():
		return nil, clierrors.Wrap(clierrors.KindAuthRequired, "timed out waiting for pasted redirect URL", ctx.Err())
	case result := <-resultCh:
		return result.values, result.err
	}
}

func (a *BrowserAuth) finishLogin(ctx context.Context, redirectURL, verifier, code string) (LoginResult, error) {
	if strings.TrimSpace(code) == "" {
		return LoginResult{}, clierrors.New(clierrors.KindAuthRequired, "Xero did not return an authorization code")
	}
	token, err := a.exchangeCode(ctx, redirectURL, verifier, code)
	if err != nil {
		return LoginResult{}, err
	}
	if err := a.store.Save(token); err != nil {
		return LoginResult{}, err
	}
	tenants, err := a.fetchTenants(ctx, token.AccessToken)
	if err != nil {
		return LoginResult{}, err
	}
	selected, err := a.tenantStore.ChooseDefault(a.settings.Interactive, tenants)
	if err != nil {
		return LoginResult{}, err
	}
	if err := a.tenantStore.PersistDefault(selected); err != nil {
		return LoginResult{}, err
	}
	if err := a.tenantStore.SaveSession(token, tenants, a.store.StorageMode(), a.store.FallbackPath()); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, Tenants: tenants, Default: selected}, nil
}

func (a *BrowserAuth) exchangeCode(ctx context.Context, redirectURL, verifier, code string) (TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", a.settings.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)
	form.Set("code_verifier", verifier)
	if strings.TrimSpace(a.settings.ClientSecret) != "" {
		form.Set("client_secret", a.settings.ClientSecret)
	}
	resp, err := a.tokenRequest(ctx, form)
	if err != nil {
		return TokenSet{}, err
	}
	return a.tokenSetFromResponse(resp), nil
}

func (a *BrowserAuth) refreshToken(ctx context.Context, token TokenSet) (TokenSet, error) {
	if strings.TrimSpace(token.RefreshToken) == "" {
		return TokenSet{}, clierrors.New(clierrors.KindTokenRefreshFailed, "stored session has no refresh token")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", a.settings.ClientID)
	form.Set("refresh_token", token.RefreshToken)
	if strings.TrimSpace(a.settings.ClientSecret) != "" {
		form.Set("client_secret", a.settings.ClientSecret)
	}
	resp, err := a.tokenRequest(ctx, form)
	if err != nil {
		return TokenSet{}, err
	}
	refreshed := a.tokenSetFromResponse(resp)
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	return refreshed, nil
}

func (a *BrowserAuth) tokenRequest(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, clierrors.Wrap(clierrors.KindInternal, "build Xero token request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return tokenResponse{}, clierrors.Wrap(clierrors.KindNetwork, "send Xero token request", err)
	}
	defer resp.Body.Close()

	var payload tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return tokenResponse{}, clierrors.Wrap(clierrors.KindNetwork, "decode Xero token response", err)
	}
	if resp.StatusCode >= 400 {
		message := payload.Description
		if message == "" {
			message = payload.Error
		}
		if message == "" {
			message = resp.Status
		}
		return tokenResponse{}, clierrors.New(clierrors.KindXeroAPI, message)
	}
	return payload, nil
}

func (a *BrowserAuth) tokenSetFromResponse(resp tokenResponse) TokenSet {
	generatedAt := a.now()
	return TokenSet{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		Scope:        resp.Scope,
		GeneratedAt:  generatedAt,
		ExpiresAt:    generatedAt.Add(time.Duration(resp.ExpiresIn) * time.Second),
		AuthMode:     "browser_oauth",
	}
}

func (a *BrowserAuth) fetchTenants(ctx context.Context, accessToken string) ([]Tenant, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.connectionsURL, nil)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindInternal, "build Xero connections request", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindNetwork, "load Xero tenants", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, clierrors.New(clierrors.KindXeroAPI, "failed to discover Xero tenants")
	}

	var payload []connectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, clierrors.Wrap(clierrors.KindNetwork, "decode tenant discovery response", err)
	}
	tenants := make([]Tenant, 0, len(payload))
	for _, item := range payload {
		tenants = append(tenants, Tenant{ID: item.TenantID, Name: item.TenantName, Type: item.TenantType})
	}
	return tenants, nil
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func listenLoopback(address string) ([]net.Listener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(host, "localhost") {
		listener, listenErr := net.Listen("tcp", address)
		if listenErr != nil {
			return nil, listenErr
		}
		return []net.Listener{listener}, nil
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}

	addresses := []string{
		net.JoinHostPort("127.0.0.1", strconv.Itoa(portNumber)),
		net.JoinHostPort("::1", strconv.Itoa(portNumber)),
	}
	listeners := make([]net.Listener, 0, len(addresses))
	var errs []string
	for _, candidate := range addresses {
		listener, listenErr := net.Listen("tcp", candidate)
		if listenErr != nil {
			if errors.Is(listenErr, syscall.EADDRINUSE) {
				closeListeners(listeners)
				return nil, listenErr
			}
			errs = append(errs, listenErr.Error())
			continue
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return listeners, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}
