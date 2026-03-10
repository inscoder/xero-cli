package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appconfig "github.com/cesar/xero-cli/internal/config"
	clierrors "github.com/cesar/xero-cli/internal/errors"
)

var ErrTokenNotFound = errors.New("token not found")

type TokenSet struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	TokenType    string    `json:"tokenType,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
	GeneratedAt  time.Time `json:"generatedAt"`
	AuthMode     string    `json:"authMode"`
}

type TokenStore interface {
	Load() (TokenSet, error)
	Save(TokenSet) error
	Clear() error
	StorageMode() string
	FallbackPath() string
}

type PersistentTokenStore struct {
	settings appconfig.Settings
	mu       sync.Mutex
}

func NewTokenStore(settings appconfig.Settings) *PersistentTokenStore {
	return &PersistentTokenStore{settings: settings}
}

func (s *PersistentTokenStore) Load() (TokenSet, error) {
	data, err := os.ReadFile(s.settings.TokenFallbackPath)
	if err != nil {
		if os.IsNotExist(err) {
			return TokenSet{}, ErrTokenNotFound
		}
		return TokenSet{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "read token fallback file", err)
	}
	return decodeTokenSet(data)
}

func (s *PersistentTokenStore) Save(token TokenSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(token.AccessToken) == "" {
		return clierrors.New(clierrors.KindValidation, "cannot save empty access token")
	}
	if token.GeneratedAt.IsZero() {
		token.GeneratedAt = time.Now().UTC()
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.KindInternal, "marshal token state", err)
	}
	data = append(data, '\n')

	return s.writeFallbackFile(data)
}

func (s *PersistentTokenStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.settings.TokenFallbackPath); err != nil && !os.IsNotExist(err) {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "remove token file", err)
	}
	return nil
}

func (s *PersistentTokenStore) StorageMode() string {
	return fmt.Sprintf("file:%s", s.settings.TokenFallbackPath)
}

func (s *PersistentTokenStore) FallbackPath() string {
	return s.settings.TokenFallbackPath
}

func (s *PersistentTokenStore) writeFallbackFile(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.settings.TokenFallbackPath), 0o700); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "create token directory", err)
	}
	lock, err := acquireLockFile(s.settings.TokenLockPath, 5*time.Second)
	if err != nil {
		return err
	}
	defer lock.release()

	tmp := s.settings.TokenFallbackPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "write token fallback file", err)
	}
	if err := os.Rename(tmp, s.settings.TokenFallbackPath); err != nil {
		return clierrors.Wrap(clierrors.KindConfigCorrupted, "replace token fallback file", err)
	}
	return nil
}

func decodeTokenSet(data []byte) (TokenSet, error) {
	var token TokenSet
	if err := json.Unmarshal(data, &token); err != nil {
		return TokenSet{}, clierrors.Wrap(clierrors.KindConfigCorrupted, "parse token state", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return TokenSet{}, clierrors.New(clierrors.KindConfigCorrupted, "stored token state is missing an access token")
	}
	if token.GeneratedAt.IsZero() {
		return TokenSet{}, clierrors.New(clierrors.KindConfigCorrupted, "stored token state is missing generatedAt")
	}
	return token, nil
}

type fileLock struct {
	path string
	file *os.File
}

func acquireLockFile(path string, timeout time.Duration) (*fileLock, error) {
	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return &fileLock{path: path, file: file}, nil
		}
		if !os.IsExist(err) {
			return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "acquire token write lock", err)
		}
		if time.Now().After(deadline) {
			return nil, clierrors.New(clierrors.KindConfigCorrupted, "timed out waiting for token storage lock")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *fileLock) release() {
	if l == nil {
		return
	}
	_ = l.file.Close()
	_ = os.Remove(l.path)
}
