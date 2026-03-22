package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// APIKey represents a stored API key with metadata.
type APIKey struct {
	Name      string   `yaml:"name"`
	Hash      string   `yaml:"hash"`                 // SHA-256 hex of the plaintext key
	Projects  []string `yaml:"projects"`             // empty = all projects
	Scope     string   `yaml:"scope,omitempty"`      // "admin", "worker", or "" (default = admin)
	WorkerID  string   `yaml:"worker_id,omitempty"`  // required when scope=worker
	CreatedAt string   `yaml:"created_at"`           // RFC3339
}

// keyFile is the on-disk YAML structure.
type keyFile struct {
	Keys []APIKey `yaml:"keys"`
}

// KeyStore manages API keys stored in a YAML file.
type KeyStore struct {
	mu   sync.RWMutex
	path string
	keys []APIKey
}

// NewKeyStore loads a KeyStore from the given YAML file path.
// If the file does not exist, it is created with an empty key list.
func NewKeyStore(path string) (*KeyStore, error) {
	ks := &KeyStore{path: path}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted config
	if os.IsNotExist(err) {
		// Ensure parent directory exists, then write an empty file.
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
			return nil, fmt.Errorf("create key store directory: %w", mkErr)
		}
		if writeErr := ks.flush(); writeErr != nil {
			return nil, fmt.Errorf("create key store file: %w", writeErr)
		}
		return ks, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read key store: %w", err)
	}

	var f keyFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse key store: %w", err)
	}
	ks.keys = f.Keys
	return ks, nil
}

// Create generates a new API key, stores its hash, and returns the plaintext.
// The plaintext is only available at creation time.
func (ks *KeyStore) Create(name string, projects []string) (string, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Reject duplicate names.
	for i := range ks.keys {
		if ks.keys[i].Name == name {
			return "", fmt.Errorf("key with name %q already exists", name)
		}
	}

	// Generate 32 random bytes -> 64 hex chars.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random key: %w", err)
	}
	plaintext := "sk_" + hex.EncodeToString(raw)

	hash := sha256.Sum256([]byte(plaintext))
	hashHex := hex.EncodeToString(hash[:])

	if projects == nil {
		projects = []string{}
	}

	key := APIKey{
		Name:      name,
		Hash:      hashHex,
		Projects:  projects,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	ks.keys = append(ks.keys, key)

	if err := ks.flush(); err != nil {
		// Roll back the append on write failure.
		ks.keys = ks.keys[:len(ks.keys)-1]
		return "", fmt.Errorf("save key store: %w", err)
	}
	return plaintext, nil
}

// CreateScoped generates a new API key with scope and optional worker ID.
// Scope must be "admin", "worker", or "" (treated as admin).
// When scope is "worker", workerID is required.
func (ks *KeyStore) CreateScoped(name string, projects []string, scope, workerID string) (plaintext string, err error) {
	if scope == "worker" && workerID == "" {
		return "", fmt.Errorf("worker_id is required when scope is %q", scope)
	}
	if scope != "" && scope != "admin" && scope != "worker" {
		return "", fmt.Errorf("invalid scope %q: must be admin, worker, or empty", scope)
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Reject duplicate names.
	for i := range ks.keys {
		if ks.keys[i].Name == name {
			return "", fmt.Errorf("key with name %q already exists", name)
		}
	}

	// Generate 32 random bytes -> 64 hex chars.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random key: %w", err)
	}
	plaintext = "sk_" + hex.EncodeToString(raw)

	hash := sha256.Sum256([]byte(plaintext))
	hashHex := hex.EncodeToString(hash[:])

	if projects == nil {
		projects = []string{}
	}

	key := APIKey{
		Name:      name,
		Hash:      hashHex,
		Projects:  projects,
		Scope:     scope,
		WorkerID:  workerID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	ks.keys = append(ks.keys, key)

	if err := ks.flush(); err != nil {
		// Roll back the append on write failure.
		ks.keys = ks.keys[:len(ks.keys)-1]
		return "", fmt.Errorf("save key store: %w", err)
	}
	return plaintext, nil
}

// ValidateWorker checks a token, verifies it is authorized for worker
// operations, and that the workerID matches the key's WorkerID field.
// Admin-scoped keys (scope="" or scope="admin") pass unconditionally.
func (ks *KeyStore) ValidateWorker(token, workerID string) bool {
	key, ok := ks.Validate(token)
	if !ok {
		return false
	}

	// Admin scope keys can do anything.
	if key.Scope == "" || key.Scope == "admin" {
		return true
	}

	// Worker scope keys must match worker ID.
	return key.Scope == "worker" && key.WorkerID == workerID
}

// List returns all stored keys. Hashes are included for display purposes.
func (ks *KeyStore) List() []APIKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	result := make([]APIKey, len(ks.keys))
	copy(result, ks.keys)
	return result
}

// Revoke removes a key by name.
func (ks *KeyStore) Revoke(name string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	idx := -1
	for i := range ks.keys {
		if ks.keys[i].Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("key %q not found", name)
	}

	ks.keys = append(ks.keys[:idx], ks.keys[idx+1:]...)

	if err := ks.flush(); err != nil {
		return fmt.Errorf("save key store: %w", err)
	}
	return nil
}

// Validate checks a plaintext token against all stored key hashes.
// Returns the matching APIKey and true if found, or nil and false otherwise.
// Uses constant-time comparison to prevent timing attacks.
func (ks *KeyStore) Validate(token string) (*APIKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	hash := sha256.Sum256([]byte(token)) //nolint:gosec // G401: SHA-256 is appropriate for constant-time API token hash comparison
	tokenHash := hex.EncodeToString(hash[:])

	for i := range ks.keys {
		if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(ks.keys[i].Hash)) == 1 {
			key := ks.keys[i]
			return &key, true
		}
	}
	return nil, false
}

// ValidateForProject checks a plaintext token and verifies it has access
// to the given project. Unscoped keys (empty Projects) have access to all
// projects.
func (ks *KeyStore) ValidateForProject(token, project string) bool {
	key, ok := ks.Validate(token)
	if !ok {
		return false
	}

	// Unscoped key: access to all projects.
	if len(key.Projects) == 0 {
		return true
	}

	for _, p := range key.Projects {
		if p == project {
			return true
		}
	}
	return false
}

// flush writes the current key list to disk. Caller must hold ks.mu.
func (ks *KeyStore) flush() error {
	f := keyFile{Keys: ks.keys}
	if f.Keys == nil {
		f.Keys = []APIKey{}
	}

	data, err := yaml.Marshal(&f)
	if err != nil {
		return fmt.Errorf("marshal key store: %w", err)
	}
	return os.WriteFile(ks.path, data, 0o600)
}
