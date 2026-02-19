package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded skill definition.
type Skill struct {
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	AlwaysLoad  bool              `json:"always_load"`
	Content     string            `json:"-"` // full SKILL.md content
	Config      map[string]string `json:"config,omitempty"`
	Dir         string            `json:"-"` // path to skill directory
}

// SkillsLoader loads and manages skill definitions from disk.
type SkillsLoader struct {
	skills []*Skill
}

// LoadSkills scans the {agentDir}/skills/ directory and loads all skill definitions.
func LoadSkills(agentDir string) *SkillsLoader {
	loader := &SkillsLoader{}
	skillsDir := filepath.Join(agentDir, "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return loader // no skills directory — that's fine
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		skillDir := filepath.Join(skillsDir, slug)
		skill := loadSkill(slug, skillDir)
		if skill != nil {
			loader.skills = append(loader.skills, skill)
		}
	}

	return loader
}

func loadSkill(slug, dir string) *Skill {
	skillPath := filepath.Join(dir, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return nil // no SKILL.md — skip
	}

	skill := &Skill{
		Slug:    slug,
		Content: string(content),
		Dir:     dir,
	}

	// Parse metadata from SKILL.md frontmatter-style comments
	// Look for: # Name, <!-- always_load: true -->
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") && skill.Name == "" {
			skill.Name = strings.TrimPrefix(line, "# ")
		}
		if strings.Contains(line, "always_load: true") {
			skill.AlwaysLoad = true
		}
	}

	if skill.Name == "" {
		skill.Name = slug
	}

	// Extract first paragraph as description
	skill.Description = extractDescription(string(content))

	// Load config.json if present
	configPath := filepath.Join(dir, "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &skill.Config)
	}

	return skill
}

func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	var desc []string
	inBody := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip heading and metadata
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "<!--") {
			inBody = true
			continue
		}
		if !inBody {
			continue
		}
		if trimmed == "" && len(desc) > 0 {
			break // end of first paragraph
		}
		if trimmed != "" {
			desc = append(desc, trimmed)
		}
	}

	return strings.Join(desc, " ")
}

// All returns all loaded skills.
func (l *SkillsLoader) All() []*Skill {
	return l.skills
}

// AlwaysLoaded returns skills marked with always_load.
func (l *SkillsLoader) AlwaysLoaded() []*Skill {
	var result []*Skill
	for _, s := range l.skills {
		if s.AlwaysLoad {
			result = append(result, s)
		}
	}
	return result
}

// Get returns a skill by slug.
func (l *SkillsLoader) Get(slug string) (*Skill, bool) {
	for _, s := range l.skills {
		if s.Slug == slug {
			return s, true
		}
	}
	return nil, false
}

// BuildSkillsSummary generates a text summary of available skills for the system prompt.
func (l *SkillsLoader) BuildSkillsSummary() string {
	if len(l.skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Skills\n\n")
	for _, s := range l.skills {
		marker := ""
		if s.AlwaysLoad {
			marker = " [always loaded]"
		}
		fmt.Fprintf(&b, "- **%s** (`%s`)%s: %s\n", s.Name, s.Slug, marker, s.Description)
	}
	return b.String()
}

// BuildAlwaysLoadedContext returns the combined content of all always_load skills.
func (l *SkillsLoader) BuildAlwaysLoadedContext() string {
	var parts []string
	for _, s := range l.skills {
		if s.AlwaysLoad {
			parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", s.Name, s.Content))
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}
