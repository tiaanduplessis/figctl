package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

const (
	keyringService = "figctl"
	// StoreEnv selects the credential store: "keyring", "file", or empty
	// for keyring with a file fallback.
	StoreEnv = "FIGCTL_CREDENTIAL_STORE"
)

// ErrNotFound is returned when a profile has no stored token.
var ErrNotFound = errors.New("credential not found")

// Store keeps tokens keyed by profile name.
type Store interface {
	// Get returns the token for a profile, or ErrNotFound.
	Get(profile string) (string, error)
	// Set stores the token for a profile.
	Set(profile, token string) error
	// Delete removes the token for a profile, or returns ErrNotFound.
	Delete(profile string) error
	// Kind names the backend for status output.
	Kind() string
}

// OpenStore returns the credential store selected by FIGCTL_CREDENTIAL_STORE.
// By default it uses the OS keychain and falls back to the credentials file
// when the keychain is unavailable, calling warn when that happens.
func OpenStore(warn func(format string, args ...any)) (Store, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	file := NewFileStore(filepath.Join(dir, credentialFile))
	switch strings.ToLower(os.Getenv(StoreEnv)) {
	case "file":
		return file, nil
	case "keyring":
		return KeyringStore{}, nil
	case "":
		return &fallbackStore{primary: KeyringStore{}, fallback: file, warn: warn}, nil
	default:
		return nil, figctl.Newf(figctl.CodeUsage, "invalid %s value %q", StoreEnv, os.Getenv(StoreEnv)).
			WithHint("Use keyring or file.")
	}
}

// KeyringStore stores tokens in the OS keychain.
type KeyringStore struct{}

// Get implements Store.
func (KeyringStore) Get(profile string) (string, error) {
	token, err := keyring.Get(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring: %w", err)
	}
	return token, nil
}

// Set implements Store.
func (KeyringStore) Set(profile, token string) error {
	if err := keyring.Set(keyringService, profile, token); err != nil {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}

// Delete implements Store.
func (KeyringStore) Delete(profile string) error {
	err := keyring.Delete(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}

// Kind implements Store.
func (KeyringStore) Kind() string { return "keyring" }

// FileStore stores tokens in a 0600 YAML file mapping profile to token.
type FileStore struct {
	path string
}

// NewFileStore creates a file store at path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Path returns the credentials file path.
func (s *FileStore) Path() string { return s.path }

// Get implements Store.
func (s *FileStore) Get(profile string) (string, error) {
	entries, err := s.read()
	if err != nil {
		return "", err
	}
	token, ok := entries[profile]
	if !ok || token == "" {
		return "", ErrNotFound
	}
	return token, nil
}

// Set implements Store.
func (s *FileStore) Set(profile, token string) error {
	entries, err := s.read()
	if err != nil {
		return err
	}
	entries[profile] = token
	return s.write(entries)
}

// Delete implements Store.
func (s *FileStore) Delete(profile string) error {
	entries, err := s.read()
	if err != nil {
		return err
	}
	if _, ok := entries[profile]; !ok {
		return ErrNotFound
	}
	delete(entries, profile)
	return s.write(entries)
}

// Kind implements Store.
func (s *FileStore) Kind() string { return "file" }

func (s *FileStore) read() (map[string]string, error) {
	entries := map[string]string{}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading credentials %s: %w", s.path, err)
	}
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parsing credentials %s: %w", s.path, err)
	}
	return entries, nil
}

func (s *FileStore) write(entries map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}
	raw, err := yaml.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("writing credentials %s: %w", s.path, err)
	}
	return os.Chmod(s.path, 0o600)
}

// fallbackStore prefers the primary store and uses the fallback when the
// primary fails for any reason other than a missing entry.
type fallbackStore struct {
	primary  Store
	fallback Store
	warn     func(format string, args ...any)
	warned   bool
}

func (s *fallbackStore) warnOnce(err error) {
	if s.warned || s.warn == nil {
		return
	}
	s.warned = true
	s.warn("OS keychain unavailable (%v); using credentials file %s", err, describe(s.fallback))
}

func describe(store Store) string {
	if f, ok := store.(*FileStore); ok {
		return f.Path()
	}
	return store.Kind()
}

func (s *fallbackStore) Get(profile string) (string, error) {
	token, err := s.primary.Get(profile)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, ErrNotFound) {
		s.warnOnce(err)
	}
	return s.fallback.Get(profile)
}

func (s *fallbackStore) Set(profile, token string) error {
	err := s.primary.Set(profile, token)
	if err == nil {
		return nil
	}
	s.warnOnce(err)
	return s.fallback.Set(profile, token)
}

func (s *fallbackStore) Delete(profile string) error {
	primaryErr := s.primary.Delete(profile)
	if primaryErr != nil && !errors.Is(primaryErr, ErrNotFound) {
		s.warnOnce(primaryErr)
	}
	fallbackErr := s.fallback.Delete(profile)
	if primaryErr == nil || fallbackErr == nil {
		return nil
	}
	if errors.Is(primaryErr, ErrNotFound) && errors.Is(fallbackErr, ErrNotFound) {
		return ErrNotFound
	}
	return fallbackErr
}

func (s *fallbackStore) Kind() string {
	if s.warned {
		return s.fallback.Kind()
	}
	return s.primary.Kind()
}
