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

	// Skill 1: always loaded, with references and scripts
	s1Dir := filepath.Join(skillsDir, "writing-style")
	os.MkdirAll(filepath.Join(s1Dir, "references"), 0o755)
	os.MkdirAll(filepath.Join(s1Dir, "scripts"), 0o755)
	os.WriteFile(filepath.Join(s1Dir, "SKILL.md"), []byte(`---
name: Writing Style
description: Concise professional writing guidelines
always_load: true
---

Use concise, professional language. Avoid jargon.

## Guidelines
- Keep sentences short
- Use active voice
`), 0o644)
	os.WriteFile(filepath.Join(s1Dir, "references", "tone-guide.md"), []byte("# Tone Guide\nBe direct and clear."), 0o644)
	os.WriteFile(filepath.Join(s1Dir, "scripts", "lint.sh"), []byte("#!/bin/sh\necho lint"), 0o644)

	// Skill 2: on-demand, no references/scripts
	s2Dir := filepath.Join(skillsDir, "linear-api")
	os.MkdirAll(s2Dir, 0o755)
	os.WriteFile(filepath.Join(s2Dir, "SKILL.md"), []byte(`---
name: Linear API
---

Interact with Linear project management tool.

## Usage
Use the linear tools to create and manage issues.
`), 0o644)

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

func TestSkillFrontmatter(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	s, _ := loader.Get("writing-style")
	if s.Description != "Concise professional writing guidelines" {
		t.Errorf("description = %q", s.Description)
	}
	// Content should not contain frontmatter
	if strings.Contains(s.Content, "---") {
		t.Errorf("content should not contain frontmatter delimiters: %q", s.Content[:80])
	}
	if !strings.HasPrefix(s.Content, "Use concise") {
		t.Errorf("content should start with body, got: %q", s.Content[:40])
	}
}

func TestSkillReferences(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	s, _ := loader.Get("writing-style")
	if len(s.References) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(s.References))
	}
	content, ok := s.References["tone-guide.md"]
	if !ok {
		t.Fatal("tone-guide.md reference not found")
	}
	if !strings.Contains(content, "Tone Guide") {
		t.Errorf("reference content = %q", content)
	}
}

func TestSkillScripts(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	s, _ := loader.Get("writing-style")
	if len(s.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(s.Scripts))
	}
	if s.Scripts[0] != "lint.sh" {
		t.Errorf("script = %q", s.Scripts[0])
	}
}

func TestSkillNoReferencesOrScripts(t *testing.T) {
	dir := setupSkillsDir(t)
	loader := LoadSkills(dir)

	s, _ := loader.Get("linear-api")
	if len(s.References) != 0 {
		t.Errorf("expected 0 references, got %d", len(s.References))
	}
	if len(s.Scripts) != 0 {
		t.Errorf("expected 0 scripts, got %d", len(s.Scripts))
	}
}

func TestLoadSkills_MultiDir(t *testing.T) {
	// dir1: shared skills
	dir1 := t.TempDir()
	s1Dir := filepath.Join(dir1, "skills", "shared-skill")
	os.MkdirAll(s1Dir, 0o755)
	os.WriteFile(filepath.Join(s1Dir, "SKILL.md"), []byte(`---
name: Shared Skill
description: From shared dir
---

Shared instructions.
`), 0o644)

	// Also add writing-style in dir1 (will be overridden)
	wsDir1 := filepath.Join(dir1, "skills", "writing-style")
	os.MkdirAll(wsDir1, 0o755)
	os.WriteFile(filepath.Join(wsDir1, "SKILL.md"), []byte(`---
name: Writing Style OLD
---

Old instructions.
`), 0o644)

	// dir2: agent-specific skills, overrides writing-style
	dir2 := t.TempDir()
	wsDir2 := filepath.Join(dir2, "skills", "writing-style")
	os.MkdirAll(wsDir2, 0o755)
	os.WriteFile(filepath.Join(wsDir2, "SKILL.md"), []byte(`---
name: Writing Style NEW
---

New instructions.
`), 0o644)

	loader := LoadSkills(dir1, dir2)

	if len(loader.All()) != 2 {
		t.Fatalf("expected 2 skills (shared + overridden), got %d", len(loader.All()))
	}

	// shared-skill should exist
	if _, ok := loader.Get("shared-skill"); !ok {
		t.Error("shared-skill not found")
	}

	// writing-style should be the dir2 version
	ws, ok := loader.Get("writing-style")
	if !ok {
		t.Fatal("writing-style not found")
	}
	if ws.Name != "Writing Style NEW" {
		t.Errorf("expected overridden name, got %q", ws.Name)
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
	if !strings.Contains(summary, "[refs:") {
		t.Errorf("summary missing refs: %q", summary)
	}
	if !strings.Contains(summary, "[scripts:") {
		t.Errorf("summary missing scripts: %q", summary)
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
	// Should include reference content
	if !strings.Contains(ctx, "Tone Guide") {
		t.Errorf("context should include reference content: %q", ctx)
	}
}

func TestExtractDescription(t *testing.T) {
	got := extractDescription("# Title\n\nFirst paragraph of description here.\n\nSecond paragraph.")
	if got != "First paragraph of description here." {
		t.Errorf("description = %q", got)
	}
}

func TestExtractDescription_NoHeading(t *testing.T) {
	got := extractDescription("First line is the description.\n\nMore text.")
	if got != "First line is the description." {
		t.Errorf("description = %q", got)
	}
}
