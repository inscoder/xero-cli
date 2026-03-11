package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/cesar/xero-cli/internal/auth"
	appconfig "github.com/cesar/xero-cli/internal/config"
	clierrors "github.com/cesar/xero-cli/internal/errors"
	"github.com/cesar/xero-cli/internal/output"
	"github.com/cesar/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

type Authenticator interface {
	Login(context.Context) (auth.LoginResult, error)
	EnsureFreshToken(context.Context, auth.TokenSet, bool) (auth.TokenSet, bool, error)
}

type Dependencies struct {
	Version          string
	IO               IOStreams
	NewViper         func() *viper.Viper
	NewTokenStore    func(appconfig.Settings) auth.TokenStore
	NewSessionStore  func(string) *auth.SessionStore
	NewInvoiceClient func(appconfig.Settings) xeroapi.InvoiceLister
	NewBrowserAuth   func(appconfig.Settings, auth.TokenStore, *auth.TenantStore, io.Reader, io.Writer) Authenticator
	IsTerminal       func(fd int) bool
	LookPath         func(string) error
	Now              func() time.Time
	ContextFactory   func() (context.Context, context.CancelFunc)
	PostRefreshState func(*Runtime, auth.TokenSet, bool) error
}

type Runtime struct {
	Settings     appconfig.Settings
	Config       *appconfig.Manager
	Tokens       auth.TokenStore
	Session      *auth.SessionStore
	Tenants      *auth.TenantStore
	Auth         Authenticator
	Xero         xeroapi.InvoiceLister
	SessionMeta  auth.SessionMetadata
	IO           IOStreams
	LookPath     func(string) error
	Now          func() time.Time
	Version      string
	contextMaker func() (context.Context, context.CancelFunc)
	postRefresh  func(*Runtime, auth.TokenSet, bool) error
}

func Execute(version string) error {
	deps := defaultDependencies(version)
	root := NewRootCommand(deps)
	if err := root.Execute(); err != nil {
		handleExecuteError(root, deps, err)
		os.Exit(clierrors.ExitCode(err))
	}
	return nil
}

func handleExecuteError(root *cobra.Command, deps Dependencies, err error) {
	if structured, quiet := wantsStructuredErrors(root, deps); structured {
		if writeErr := output.WriteErrorJSON(deps.IO.Out, err, quiet); writeErr == nil {
			return
		}
	}
	fmt.Fprintln(deps.IO.ErrOut, err)
}

func wantsStructuredErrors(root *cobra.Command, deps Dependencies) (bool, bool) {
	quiet, _ := root.PersistentFlags().GetBool("quiet")
	if quiet {
		return true, true
	}

	outputJSON, _ := root.PersistentFlags().GetBool("json")
	if outputJSON {
		return true, false
	}

	settings, err := loadErrorOutputSettings(root, deps)
	if err != nil {
		return false, false
	}
	if settings.Quiet {
		return true, true
	}
	return settings.OutputJSON, false
}

func loadErrorOutputSettings(root *cobra.Command, deps Dependencies) (appconfig.Settings, error) {
	v := deps.NewViper()
	appconfig.ConfigureViper(v)
	if flag := root.PersistentFlags().Lookup("config"); flag != nil {
		v.Set("config", flag.Value.String())
	}
	manager, err := appconfig.NewManager(v)
	if err != nil {
		return appconfig.Settings{}, err
	}
	return manager.Load(deps.IsTerminal(0), deps.Version)
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	v := deps.NewViper()
	appconfig.ConfigureViper(v)

	root := &cobra.Command{
		Use:           "xero",
		Short:         "Terminal-first Xero CLI",
		Version:       deps.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(deps.IO.Out)
	root.SetErr(deps.IO.ErrOut)
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	root.PersistentFlags().String("config", "", "config file path")
	root.PersistentFlags().Bool("json", false, "emit JSON envelope on stdout")
	root.PersistentFlags().Bool("quiet", false, "emit raw data only on stdout")
	root.PersistentFlags().String("tenant", "", "override tenant ID")
	root.PersistentFlags().Bool("no-browser", false, "fail instead of opening a browser")
	root.PersistentFlags().String("client-id", "", "Xero OAuth client ID")
	root.PersistentFlags().String("client-secret", "", "Xero OAuth client secret")

	mustBind(v, "config", root.PersistentFlags().Lookup("config"))
	mustBind(v, "output.json", root.PersistentFlags().Lookup("json"))
	mustBind(v, "output.quiet", root.PersistentFlags().Lookup("quiet"))
	mustBind(v, "tenant", root.PersistentFlags().Lookup("tenant"))
	mustBind(v, "auth.no_browser", root.PersistentFlags().Lookup("no-browser"))
	mustBind(v, "auth.client_id", root.PersistentFlags().Lookup("client-id"))
	mustBind(v, "auth.client_secret", root.PersistentFlags().Lookup("client-secret"))

	root.AddCommand(newAuthCommand(deps, v))
	root.AddCommand(newInvoicesCommand(deps, v))
	root.AddCommand(newDoctorCommand(deps, v))
	root.AddCommand(newVersionCommand(deps))

	return root
}

func defaultDependencies(version string) Dependencies {
	return Dependencies{
		Version:         version,
		IO:              IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
		NewViper:        viper.New,
		NewTokenStore:   func(settings appconfig.Settings) auth.TokenStore { return auth.NewTokenStore(settings) },
		NewSessionStore: auth.NewSessionStore,
		NewInvoiceClient: func(settings appconfig.Settings) xeroapi.InvoiceLister {
			return xeroapi.NewClient(settings, xeroapi.ClientOptions{})
		},
		NewBrowserAuth: func(settings appconfig.Settings, store auth.TokenStore, tenants *auth.TenantStore, in io.Reader, errOut io.Writer) Authenticator {
			return auth.NewBrowserAuth(settings, store, tenants, in, errOut)
		},
		IsTerminal:     term.IsTerminal,
		LookPath:       defaultLookPath,
		Now:            func() time.Time { return time.Now().UTC() },
		ContextFactory: xeroapi.DefaultContext,
		PostRefreshState: func(rt *Runtime, token auth.TokenSet, refreshed bool) error {
			if !refreshed {
				return nil
			}
			return rt.Tenants.UpdateRefreshState(token, rt.SessionMeta.KnownTenants, rt.Tokens.StorageMode(), rt.Tokens.FallbackPath())
		},
	}
}

func defaultLookPath(name string) error {
	if name == "" {
		return errors.New("empty executable name")
	}
	_, err := execLookPath(name)
	return err
}

var execLookPath = func(name string) (string, error) {
	return exec.LookPath(name)
}

func mustBind(v *viper.Viper, key string, flag *pflag.Flag) {
	if flag == nil {
		return
	}
	_ = v.BindPFlag(key, flag)
}

func loadRuntime(deps Dependencies, v *viper.Viper) (*Runtime, error) {
	interactive := deps.IsTerminal(0)
	manager, err := appconfig.NewManager(v)
	if err != nil {
		return nil, err
	}
	settings, err := manager.Load(interactive, deps.Version)
	if err != nil {
		return nil, err
	}
	tokens := deps.NewTokenStore(settings)
	session := deps.NewSessionStore(settings.SessionFilePath)
	meta, err := session.Load()
	if err != nil {
		return nil, err
	}
	tenants := auth.NewTenantStore(manager, session, deps.IO.In, deps.IO.ErrOut)
	runtime := &Runtime{
		Settings:     settings,
		Config:       manager,
		Tokens:       tokens,
		Session:      session,
		Tenants:      tenants,
		Auth:         deps.NewBrowserAuth(settings, tokens, tenants, deps.IO.In, deps.IO.ErrOut),
		Xero:         deps.NewInvoiceClient(settings),
		SessionMeta:  meta,
		IO:           deps.IO,
		LookPath:     deps.LookPath,
		Now:          deps.Now,
		Version:      deps.Version,
		contextMaker: deps.ContextFactory,
		postRefresh:  deps.PostRefreshState,
	}
	return runtime, nil
}

func (rt *Runtime) Context() (context.Context, context.CancelFunc) {
	return rt.contextMaker()
}

func (rt *Runtime) WriteData(data any, summary string, breadcrumbs []output.Breadcrumb, human func(io.Writer) error) error {
	if rt.Settings.OutputJSON || rt.Settings.Quiet {
		return output.WriteJSON(rt.IO.Out, data, summary, breadcrumbs, rt.Settings.Quiet)
	}
	return human(rt.IO.Out)
}

func (rt *Runtime) LoadToken() (auth.TokenSet, error) {
	token, err := rt.Tokens.Load()
	if err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) {
			return auth.TokenSet{}, clierrors.New(clierrors.KindAuthRequired, "no saved Xero session; run `xero auth login`")
		}
		return auth.TokenSet{}, err
	}
	return token, nil
}

func (rt *Runtime) EnsureToken(token auth.TokenSet) (auth.TokenSet, error) {
	ctx, cancel := rt.Context()
	defer cancel()
	refreshedToken, refreshed, err := rt.Auth.EnsureFreshToken(ctx, token, rt.Settings.Interactive)
	if err != nil {
		_ = rt.Tenants.MarkError(err.Error())
		return auth.TokenSet{}, err
	}
	if err := rt.postRefresh(rt, refreshedToken, refreshed); err != nil {
		return auth.TokenSet{}, err
	}
	if refreshed {
		meta, _ := rt.Session.Load()
		rt.SessionMeta = meta
	}
	return refreshedToken, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
