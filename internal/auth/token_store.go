package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appconfig "github.com/inscoder/xero-cli/internal/config"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"golang.org/x/crypto/scrypt"
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
	TenantID     string    `json:"tenantId,omitempty"`
	TenantName   string    `json:"tenantName,omitempty"`
	TenantType   string    `json:"tenantType,omitempty"`
}

type tokenCache map[string]encryptedTokenSet

type encryptedTokenSet struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	TokenType    string    `json:"tokenType,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
	GeneratedAt  time.Time `json:"generatedAt"`
	AuthMode     string    `json:"authMode"`
	TenantID     string    `json:"tenantId,omitempty"`
	TenantName   string    `json:"tenantName,omitempty"`
	TenantType   string    `json:"tenantType,omitempty"`
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
	cache, err := decodeTokenCache(data)
	if err != nil {
		return TokenSet{}, err
	}
	profileName := strings.TrimSpace(s.settings.ProfileName)
	if profileName == "" {
		return TokenSet{}, ErrTokenNotFound
	}
	entry, ok := cache[profileName]
	if !ok {
		return TokenSet{}, ErrTokenNotFound
	}
	key, err := s.loadEncryptionKey()
	if err != nil {
		return TokenSet{}, err
	}
	return decryptTokenSet(entry, key)
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
	if token.ExpiresAt.IsZero() {
		return clierrors.New(clierrors.KindValidation, "cannot save token without an expiry time")
	}

	profileName := strings.TrimSpace(s.settings.ProfileName)
	if profileName == "" {
		return clierrors.New(clierrors.KindValidation, "cannot save token without an active profile")
	}
	key, err := s.loadEncryptionKey()
	if err != nil {
		return err
	}
	cache, err := s.readCache()
	if err != nil {
		return err
	}
	entry, err := encryptTokenSet(token, key)
	if err != nil {
		return err
	}
	cache[profileName] = entry

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.KindInternal, "marshal token state", err)
	}
	data = append(data, '\n')

	return s.writeFallbackFile(data)
}

func (s *PersistentTokenStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profileName := strings.TrimSpace(s.settings.ProfileName)
	if profileName == "" {
		return nil
	}
	cache, err := s.readCache()
	if err != nil && !errors.Is(err, ErrTokenNotFound) {
		return err
	}
	if len(cache) == 0 {
		return nil
	}
	delete(cache, profileName)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.KindInternal, "marshal token state", err)
	}
	data = append(data, '\n')
	return s.writeFallbackFile(data)
}

func (s *PersistentTokenStore) StorageMode() string {
	return fmt.Sprintf("encrypted-file:%s", s.settings.TokenFallbackPath)
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

func (s *PersistentTokenStore) readCache() (tokenCache, error) {
	data, err := os.ReadFile(s.settings.TokenFallbackPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tokenCache{}, nil
		}
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "read token fallback file", err)
	}
	return decodeTokenCache(data)
}

func decodeTokenCache(data []byte) (tokenCache, error) {
	var cache tokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "parse token state", err)
	}
	if cache == nil {
		cache = tokenCache{}
	}
	return cache, nil
}

func (s *PersistentTokenStore) loadEncryptionKey() ([]byte, error) {
	if passphrase := strings.TrimSpace(os.Getenv("XERO_TOKEN_PASSPHRASE")); passphrase != "" {
		return s.keyFromPassphrase(passphrase)
	}
	storage := strings.ToLower(strings.TrimSpace(os.Getenv("XERO_KEY_STORAGE")))
	if storage == "" || storage == "auto" || storage == "file" {
		return s.keyFromFile()
	}
	if storage == "keyring" {
		return nil, clierrors.New(clierrors.KindConfigCorrupted, "XERO_KEY_STORAGE=keyring is not supported in this build; use XERO_TOKEN_PASSPHRASE or XERO_KEY_STORAGE=file")
	}
	return nil, clierrors.New(clierrors.KindValidation, "XERO_KEY_STORAGE must be auto, file, or keyring")
}

func (s *PersistentTokenStore) keyFromPassphrase(passphrase string) ([]byte, error) {
	salt, err := readOrCreateRandomFile(s.settings.TokenSaltPath, 16)
	if err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindInternal, "derive token encryption key", err)
	}
	return key, nil
}

func (s *PersistentTokenStore) keyFromFile() ([]byte, error) {
	data, err := os.ReadFile(s.settings.TokenKeyPath)
	if err == nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if decodeErr != nil || len(decoded) != 32 {
			return nil, clierrors.New(clierrors.KindConfigCorrupted, "stored token encryption key is invalid")
		}
		return decoded, nil
	}
	if !os.IsNotExist(err) {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "read token encryption key", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, clierrors.Wrap(clierrors.KindInternal, "generate token encryption key", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.settings.TokenKeyPath), 0o700); err != nil {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "create token key directory", err)
	}
	encoded := []byte(base64.StdEncoding.EncodeToString(key) + "\n")
	if err := os.WriteFile(s.settings.TokenKeyPath, encoded, 0o600); err != nil {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "write token encryption key", err)
	}
	return key, nil
}

func readOrCreateRandomFile(path string, size int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != size {
			return nil, clierrors.New(clierrors.KindConfigCorrupted, "stored token salt is invalid")
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "read token salt", err)
	}
	data = make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return nil, clierrors.Wrap(clierrors.KindInternal, "generate token salt", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "create token salt directory", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, clierrors.Wrap(clierrors.KindConfigCorrupted, "write token salt", err)
	}
	return data, nil
}

func encryptTokenSet(token TokenSet, key []byte) (encryptedTokenSet, error) {
	accessToken, err := encryptString(token.AccessToken, key)
	if err != nil {
		return encryptedTokenSet{}, err
	}
	refreshToken := ""
	if token.RefreshToken != "" {
		refreshToken, err = encryptString(token.RefreshToken, key)
		if err != nil {
			return encryptedTokenSet{}, err
		}
	}
	return encryptedTokenSet{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    token.TokenType,
		Scope:        token.Scope,
		ExpiresAt:    token.ExpiresAt,
		GeneratedAt:  token.GeneratedAt,
		AuthMode:     token.AuthMode,
		TenantID:     token.TenantID,
		TenantName:   token.TenantName,
		TenantType:   token.TenantType,
	}, nil
}

func decryptTokenSet(entry encryptedTokenSet, key []byte) (TokenSet, error) {
	accessToken, err := decryptString(entry.AccessToken, key)
	if err != nil {
		return TokenSet{}, err
	}
	refreshToken := ""
	if entry.RefreshToken != "" {
		refreshToken, err = decryptString(entry.RefreshToken, key)
		if err != nil {
			return TokenSet{}, err
		}
	}
	token := TokenSet{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    entry.TokenType,
		Scope:        entry.Scope,
		ExpiresAt:    entry.ExpiresAt,
		GeneratedAt:  entry.GeneratedAt,
		AuthMode:     entry.AuthMode,
		TenantID:     entry.TenantID,
		TenantName:   entry.TenantName,
		TenantType:   entry.TenantType,
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return TokenSet{}, clierrors.New(clierrors.KindConfigCorrupted, "stored token state is missing an access token")
	}
	if token.GeneratedAt.IsZero() {
		return TokenSet{}, clierrors.New(clierrors.KindConfigCorrupted, "stored token state is missing generatedAt")
	}
	if token.ExpiresAt.IsZero() {
		return TokenSet{}, clierrors.New(clierrors.KindConfigCorrupted, "stored token state is missing expiresAt")
	}
	return token, nil
}

func encryptString(value string, key []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", clierrors.Wrap(clierrors.KindInternal, "generate token nonce", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	packed := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(packed), nil
}

func decryptString(value string, key []byte) (string, error) {
	packed, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", clierrors.Wrap(clierrors.KindConfigCorrupted, "decode encrypted token", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(packed) < gcm.NonceSize() {
		return "", clierrors.New(clierrors.KindConfigCorrupted, "encrypted token payload is invalid")
	}
	nonce := packed[:gcm.NonceSize()]
	ciphertext := packed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", clierrors.Wrap(clierrors.KindConfigCorrupted, "decrypt token", err)
	}
	return string(plaintext), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		hashed := sha256.Sum256(key)
		key = hashed[:]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindInternal, "create token cipher", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindInternal, "create token cipher mode", err)
	}
	return gcm, nil
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
