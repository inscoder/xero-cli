package auth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

type Tenant struct {
	ID   string `json:"tenantId"`
	Name string `json:"tenantName"`
	Type string `json:"tenantType,omitempty"`
}

type SessionMetadata struct {
	Authenticated    bool      `json:"authenticated"`
	AuthMode         string    `json:"authMode,omitempty"`
	GeneratedAt      time.Time `json:"generatedAt,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
	LastRefreshAt    time.Time `json:"lastRefreshAt,omitempty"`
	KnownTenants     []Tenant  `json:"knownTenants,omitempty"`
	StorageMode      string    `json:"storageMode,omitempty"`
	LastError        string    `json:"lastError,omitempty"`
	FallbackFilePath string    `json:"fallbackFilePath,omitempty"`
}

type SessionStore struct {
	path string
}

type TenantStore struct {
	config  *appconfig.Manager
	session *SessionStore
	in      io.Reader
	out     io.Writer
}

func NewSessionStore(path string) *SessionStore {
	return &SessionStore{path: path}
}

func (s *SessionStore) Load() (SessionMetadata, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionMetadata{}, nil
		}
		return SessionMetadata{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "read session metadata", err)
	}
	var meta SessionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return SessionMetadata{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "parse session metadata", err)
	}
	return meta, nil
}

func (s *SessionStore) Save(meta SessionMetadata) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "create session metadata directory", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.KindInternal, "marshal session metadata", err)
	}
	data = append(data, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "write session metadata", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "replace session metadata", err)
	}
	return nil
}

func (s *SessionStore) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "remove session metadata", err)
	}
	return nil
}

func NewTenantStore(cfg *appconfig.Manager, session *SessionStore, in io.Reader, out io.Writer) *TenantStore {
	return &TenantStore{config: cfg, session: session, in: in, out: out}
}

func (s *TenantStore) Resolve(override string, known []Tenant) (Tenant, error) {
	selected := strings.TrimSpace(override)
	if selected == "" {
		selected = strings.TrimSpace(s.config.LoadedConfig().DefaultTenantID)
	}
	if selected == "" {
		return Tenant{}, clierrors.New(clierrors.KindTenantSelectionRequired, "no default tenant selected; run `xero auth login` or pass --tenant")
	}
	for _, tenant := range known {
		if tenant.ID == selected {
			return tenant, nil
		}
	}
	if len(known) > 0 {
		return Tenant{}, clierrors.New(clierrors.KindTenantSelectionRequired, "saved tenant is no longer available; run `xero auth login` to choose a new default tenant")
	}
	return Tenant{ID: selected, Name: selected}, nil
}

func (s *TenantStore) ChooseDefault(interactive bool, tenants []Tenant) (Tenant, error) {
	if len(tenants) == 0 {
		return Tenant{}, clierrors.New(clierrors.KindTenantSelectionRequired, "no Xero tenants were returned for this login")
	}
	if len(tenants) == 1 {
		return tenants[0], nil
	}
	if !interactive {
		return Tenant{}, clierrors.New(clierrors.KindTenantSelectionRequired, "multiple Xero tenants are available; re-run interactively to choose a default tenant")
	}

	reader := bufio.NewReader(s.in)
	for {
		fmt.Fprintln(s.out, "Multiple Xero tenants are available. Choose a default:")
		for index, tenant := range tenants {
			fmt.Fprintf(s.out, "  %d. %s (%s)\n", index+1, tenant.Name, tenant.ID)
		}
		fmt.Fprint(s.out, "Selection: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return Tenant{}, clierrors.Wrap(clierrors.KindValidation, "read tenant selection", err)
		}
		line = strings.TrimSpace(line)
		for index, tenant := range tenants {
			if line == fmt.Sprintf("%d", index+1) || strings.EqualFold(line, tenant.ID) {
				return tenant, nil
			}
		}
		fmt.Fprintln(s.out, "Please choose one of the listed tenant numbers or IDs.")
	}
}

func (s *TenantStore) PersistDefault(tenant Tenant) error {
	return s.config.UpdateDefaultTenant(tenant.ID, tenant.Name)
}

func (s *TenantStore) SaveSession(token TokenSet, tenants []Tenant, storageMode, fallbackPath string) error {
	meta := SessionMetadata{
		Authenticated:    true,
		AuthMode:         token.AuthMode,
		GeneratedAt:      token.GeneratedAt,
		ExpiresAt:        token.ExpiresAt,
		KnownTenants:     tenants,
		StorageMode:      storageMode,
		FallbackFilePath: fallbackPath,
	}
	return s.session.Save(meta)
}

func (s *TenantStore) UpdateRefreshState(token TokenSet, known []Tenant, storageMode, fallbackPath string) error {
	meta, err := s.session.Load()
	if err != nil {
		return err
	}
	meta.Authenticated = true
	meta.AuthMode = token.AuthMode
	meta.GeneratedAt = token.GeneratedAt
	meta.ExpiresAt = token.ExpiresAt
	meta.LastRefreshAt = time.Now().UTC()
	meta.StorageMode = storageMode
	meta.FallbackFilePath = fallbackPath
	if len(known) > 0 {
		meta.KnownTenants = known
	}
	meta.LastError = ""
	return s.session.Save(meta)
}

func (s *TenantStore) MarkError(message string) error {
	meta, err := s.session.Load()
	if err != nil {
		return err
	}
	meta.LastError = message
	return s.session.Save(meta)
}
