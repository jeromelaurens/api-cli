package runner

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

type CredentialStore interface {
	Set(key string, value string) error
	Get(key string) (string, error)
	Delete(key string) error
}

type MemoryCredentialStore struct {
	mu     sync.Mutex
	values map[string]string
}

func NewMemoryCredentialStore() *MemoryCredentialStore {
	return &MemoryCredentialStore{values: make(map[string]string)}
}

func (s *MemoryCredentialStore) Set(key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *MemoryCredentialStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", codedError(CodeCredentialMissing, "credential %s is missing", key)
	}
	return value, nil
}

func (s *MemoryCredentialStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; !ok {
		return codedError(CodeCredentialMissing, "credential %s is missing", key)
	}
	delete(s.values, key)
	return nil
}

type KeyringCredentialStore struct{}

func (s KeyringCredentialStore) Set(key string, value string) error {
	if err := keyring.Set(keyringService, key, value); err != nil {
		return wrapError(CodeKeyringUnavailable, err, "cannot store credential %s in keyring", key)
	}
	return nil
}

func (s KeyringCredentialStore) Get(key string) (string, error) {
	value, err := keyring.Get(keyringService, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", codedError(CodeCredentialMissing, "credential %s is missing", key)
		}
		return "", wrapError(CodeKeyringUnavailable, err, "cannot read credential %s from keyring", key)
	}
	return value, nil
}

func (s KeyringCredentialStore) Delete(key string) error {
	if err := keyring.Delete(keyringService, key); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return codedError(CodeCredentialMissing, "credential %s is missing", key)
		}
		return wrapError(CodeKeyringUnavailable, err, "cannot delete credential %s from keyring", key)
	}
	return nil
}
