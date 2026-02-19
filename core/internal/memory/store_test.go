package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if len(s.List()) != 0 {
		t.Errorf("expected empty store, got %d scopes", len(s.List()))
	}
}

func TestSetAndGet(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := s.Set("project", "# My Project\nSome notes."); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := s.Get("project")
	if got != "# My Project\nSome notes." {
		t.Errorf("Get returned %q", got)
	}

	// Verify file was persisted
	data, err := os.ReadFile(filepath.Join(dir, "memory", "project.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "# My Project\nSome notes." {
		t.Errorf("file content = %q", string(data))
	}
}

func TestGetMissing(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if got := s.Get("nonexistent"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Set("project", "project notes")
	s.Set("preferences", "user prefs")
	s.Set("team", "team info")

	scopes := s.List()
	if len(scopes) != 3 {
		t.Fatalf("expected 3 scopes, got %d", len(scopes))
	}
	if scopes["project"] != "project notes" {
		t.Errorf("project = %q", scopes["project"])
	}
	if scopes["preferences"] != "user prefs" {
		t.Errorf("preferences = %q", scopes["preferences"])
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Set("temp", "temporary data")
	if err := s.Delete("temp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := s.Get("temp"); got != "" {
		t.Errorf("expected empty after delete, got %q", got)
	}

	// File should be gone
	path := filepath.Join(dir, "memory", "temp.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, got err=%v", err)
	}
}

func TestDeleteMissing(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Deleting a nonexistent scope should not error
	if err := s.Delete("ghost"); err != nil {
		t.Errorf("Delete nonexistent: %v", err)
	}
}

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-populate memory directory
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0o755)
	os.WriteFile(filepath.Join(memDir, "project.md"), []byte("existing project notes"), 0o644)
	os.WriteFile(filepath.Join(memDir, "preferences.md"), []byte("existing prefs"), 0o644)
	// Non-.md file should be ignored
	os.WriteFile(filepath.Join(memDir, "notes.txt"), []byte("ignored"), 0o644)

	s := NewStore(dir)
	scopes := s.List()

	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d: %v", len(scopes), scopes)
	}
	if scopes["project"] != "existing project notes" {
		t.Errorf("project = %q", scopes["project"])
	}
	if scopes["preferences"] != "existing prefs" {
		t.Errorf("preferences = %q", scopes["preferences"])
	}
}

func TestOverwrite(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Set("project", "version 1")
	s.Set("project", "version 2")

	if got := s.Get("project"); got != "version 2" {
		t.Errorf("expected overwritten content, got %q", got)
	}
}

func TestListReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.Set("a", "value")

	list := s.List()
	list["a"] = "modified"

	// Original should be unchanged
	if got := s.Get("a"); got != "value" {
		t.Errorf("List returned reference instead of copy, Get = %q", got)
	}
}
