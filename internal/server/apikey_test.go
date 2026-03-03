package server_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/herbhall/samverk/internal/server"
)

func tempKeyStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "auth.yaml")
}

func TestKeyStore_CreateAndValidate(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	plaintext, err := ks.Create("test-key", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plaintext == "" {
		t.Fatal("Create returned empty plaintext")
	}
	if len(plaintext) < 64 {
		t.Errorf("plaintext too short: got %d chars", len(plaintext))
	}

	key, ok := ks.Validate(plaintext)
	if !ok {
		t.Fatal("Validate returned false for valid key")
	}
	if key.Name != "test-key" {
		t.Errorf("key name = %q, want %q", key.Name, "test-key")
	}

	// Wrong token should fail.
	_, ok = ks.Validate("sk_wrong")
	if ok {
		t.Fatal("Validate returned true for invalid key")
	}
}

func TestKeyStore_Create_DuplicateName(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	if _, err := ks.Create("dupe", nil); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = ks.Create("dupe", nil)
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestKeyStore_Revoke(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	plaintext, err := ks.Create("revoke-me", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ks.Revoke("revoke-me"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, ok := ks.Validate(plaintext)
	if ok {
		t.Fatal("Validate returned true after revocation")
	}
}

func TestKeyStore_Revoke_NotFound(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	err = ks.Revoke("nonexistent")
	if err == nil {
		t.Fatal("expected error for revoking nonexistent key, got nil")
	}
}

func TestKeyStore_ValidateForProject(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	plaintext, err := ks.Create("scoped-key", []string{"project-a", "project-b"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !ks.ValidateForProject(plaintext, "project-a") {
		t.Error("ValidateForProject returned false for allowed project-a")
	}
	if !ks.ValidateForProject(plaintext, "project-b") {
		t.Error("ValidateForProject returned false for allowed project-b")
	}
}

func TestKeyStore_ValidateForProject_Denied(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	plaintext, err := ks.Create("scoped-key", []string{"project-a"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if ks.ValidateForProject(plaintext, "project-c") {
		t.Error("ValidateForProject returned true for disallowed project-c")
	}
}

func TestKeyStore_ValidateForProject_Unscoped(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	plaintext, err := ks.Create("unscoped-key", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !ks.ValidateForProject(plaintext, "any-project") {
		t.Error("ValidateForProject returned false for unscoped key")
	}
	if !ks.ValidateForProject(plaintext, "another-project") {
		t.Error("ValidateForProject returned false for unscoped key on another project")
	}
}

func TestKeyStore_Persistence(t *testing.T) {
	path := tempKeyStorePath(t)

	ks1, err := server.NewKeyStore(path)
	if err != nil {
		t.Fatalf("NewKeyStore (first): %v", err)
	}

	plaintext, err := ks1.Create("persist-key", []string{"proj"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Reload from the same file.
	ks2, err := server.NewKeyStore(path)
	if err != nil {
		t.Fatalf("NewKeyStore (reload): %v", err)
	}

	key, ok := ks2.Validate(plaintext)
	if !ok {
		t.Fatal("Validate failed after reload")
	}
	if key.Name != "persist-key" {
		t.Errorf("key name = %q, want %q", key.Name, "persist-key")
	}
	if len(key.Projects) != 1 || key.Projects[0] != "proj" {
		t.Errorf("key projects = %v, want [proj]", key.Projects)
	}
}

func TestKeyStore_List(t *testing.T) {
	ks, err := server.NewKeyStore(tempKeyStorePath(t))
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	if _, err := ks.Create("key-a", nil); err != nil {
		t.Fatalf("Create key-a: %v", err)
	}
	if _, err := ks.Create("key-b", []string{"proj-1"}); err != nil {
		t.Fatalf("Create key-b: %v", err)
	}

	keys := ks.List()
	if len(keys) != 2 {
		t.Fatalf("List returned %d keys, want 2", len(keys))
	}

	if keys[0].Name != "key-a" {
		t.Errorf("keys[0].Name = %q, want %q", keys[0].Name, "key-a")
	}
	if keys[1].Name != "key-b" {
		t.Errorf("keys[1].Name = %q, want %q", keys[1].Name, "key-b")
	}
	if len(keys[1].Projects) != 1 || keys[1].Projects[0] != "proj-1" {
		t.Errorf("keys[1].Projects = %v, want [proj-1]", keys[1].Projects)
	}
	if keys[0].CreatedAt == "" {
		t.Error("keys[0].CreatedAt is empty")
	}
}

func TestKeyStore_NewKeyStore_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "auth.yaml")

	ks, err := server.NewKeyStore(path)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}
	_ = ks

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected key store file to be created")
	}
}
