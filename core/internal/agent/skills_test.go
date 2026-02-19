package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupSkillsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")

	// Skill 1: always loaded
	s1Dir := filepath.Join(skillsDir, "writing-style")
	os.MkdirAll(s1Dir, 0o755)
	os.WriteFile(filepath.Join(s1Dir, "SKILL.md"), []byte(`# Writing Style
<!-- always_load: true -->

Use concise, professional language. Avoid jargon.

## Guidelines
- Keep sentences short
- Use active voice
`), 0o644)

	// Skill 2: on-demand
	s2Dir := filepath.Join(skillsDir, "linear-api")
	os.MkdirAll(s2Dir, 0o755)
	os.WriteFile(filepath.Join(s2Dir, "SKILL.md"), []byte(`# Linear API
Interact with Linear project management tool.

## Usage
Use the linear tools to create and manage issues.
`), 0o644)
	os.WriteFile(filepath.Join(s2Dir, "config.json"), []byte(`{"api_key": "lin_test"}`), 0o644)

	// Not a skill: regular file in skills dir
	os.WriteFile(filepath.Join(skillsDir, "readme.txt"), []byte("ignore me"), 0o644)

	return dir
}

func TestLoadSkills(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	if len(loader.All()) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(loader.All()))
	}
}

func TestLoadSkills_EmptyDir(t *testing.T) {
	loader := LoadSkills(t.TempDir())
	if len(loader.All()) != 0 {
		t.Errorf("expected 0 skills, got %d", len(loader.All()))
	}
}

func TestAlwaysLoaded(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	always := loader.AlwaysLoaded()
	if len(always) != 1 {
		t.Fatalf("expected 1 always_load skill, got %d", len(always))
	}
	if always[0].Slug != "writing-style" {
		t.Errorf("slug = %q", always[0].Slug)
	}
	if always[0].Name != "Writing Style" {
		t.Errorf("name = %q", always[0].Name)
	}
}

func TestSkillGet(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	s, ok := loader.Get("linear-api")
	if !ok {
		t.Fatal("linear-api not found")
	}
	if s.Name != "Linear API" {
		t.Errorf("name = %q", s.Name)
	}
	if s.AlwaysLoad {
		t.Error("linear-api should not be always_load")
	}
}

func TestSkillConfig(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	s, _ := loader.Get("linear-api")
	if s.Config["api_key"] != "lin_test" {
		t.Errorf("config api_key = %q", s.Config["api_key"])
	}
}

func TestBuildSkillsSummary(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	summary := loader.BuildSkillsSummary()
	if !strings.Contains(summary, "Writing Style") {
		t.Errorf("summary missing Writing Style: %q", summary)
	}
	if !strings.Contains(summary, "Linear API") {
		t.Errorf("summary missing Linear API: %q", summary)
	}
	if !strings.Contains(summary, "[always loaded]") {
		t.Errorf("summary missing [always loaded]: %q", summary)
	}
}

func TestBuildAlwaysLoadedContext(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	ctx := loader.BuildAlwaysLoadedContext()
	if !strings.Contains(ctx, "Writing Style") {
		t.Errorf("context missing Writing Style: %q", ctx)
	}
	if strings.Contains(ctx, "Linear API") {
		t.Errorf("context should not contain on-demand skills: %q", ctx)
	}
}

func TestExtractDescription(t *testing.T) {
	got := extractDescription("# Title\n\nFirst paragraph of description here.\n\nSecond paragraph.")
	if got != "First paragraph of description here." {
		t.Errorf("description = %q", got)
	}
}
