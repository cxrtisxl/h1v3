package memory

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store provides scoped persistent memory backed by .md files.
// Each scope maps to a file at {dir}/memory/{scope}.md.
type Store struct {
	dir    string // base agent directory (memory files live in {dir}/memory/)
	mu     sync.RWMutex
	scopes map[string]string // scope_name → content
}

// NewStore creates a memory store and loads all existing .md files from {dir}/memory/.
// If the directory doesn't exist yet, it will be created on the first Set call.
func NewStore(dir string) *Store {
	s := &Store{
		dir:    dir,
		scopes: make(map[string]string),
	}
	s.load()
	return s
}

// Get returns the content of a scope, or empty string if it doesn't exist.
func (s *Store) Get(scope string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scopes[scope]
}

// Set writes content to a scope and persists it to disk.
func (s *Store) Set(scope, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	memDir := filepath.Join(s.dir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(memDir, scope+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	s.scopes[scope] = content
	return nil
}

// List returns a copy of all scopes and their content.
func (s *Store) List() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]string, len(s.scopes))
	for k, v := range s.scopes {
		out[k] = v
	}
	return out
}

// Delete removes a scope from memory and disk.
func (s *Store) Delete(scope string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, "memory", scope+".md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	delete(s.scopes, scope)
	return nil
}

// load reads all .md files from the memory directory into the scopes map.
func (s *Store) load() {
	memDir := filepath.Join(s.dir, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return // directory doesn't exist yet — that's fine
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		scope := strings.TrimSuffix(e.Name(), ".md")
		s.scopes[scope] = string(data)
	}
}
