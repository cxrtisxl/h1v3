package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/h1v3-io/h1v3/internal/tool"
)

// Skill represents a loaded skill definition.
type Skill struct {
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	AlwaysLoad  bool              `json:"always_load"`
	Content     string            `json:"-"` // instruction body (frontmatter stripped)
	References  map[string]string `json:"-"` // filename → content from references/
	Scripts     []string          `json:"-"` // filenames from scripts/
	Dir         string            `json:"-"` // path to skill directory
}

// SkillsLoader loads and manages skill definitions from disk.
type SkillsLoader struct {
	skills []*Skill
}

// LoadSkills scans {dir}/skills/ for each provided directory and loads all
// skill definitions. Skills from later directories override earlier ones
// (matched by slug). Pass shared/bundled dirs first, agent-specific last.
func LoadSkills(dirs ...string) *SkillsLoader {
	loader := &SkillsLoader{}
	seen := map[string]int{} // slug → index in loader.skills

	for _, dir := range dirs {
		skillsDir := filepath.Join(dir, "skills")
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue // no skills directory — that's fine
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			slug := e.Name()
			skillDir := filepath.Join(skillsDir, slug)
			skill := loadSkill(slug, skillDir)
			if skill == nil {
				continue
			}
			if idx, ok := seen[slug]; ok {
				loader.skills[idx] = skill // override
			} else {
				seen[slug] = len(loader.skills)
				loader.skills = append(loader.skills, skill)
			}
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
		Slug: slug,
		Dir:  dir,
	}

	// Parse YAML frontmatter (between --- delimiters) and extract body
	body := string(content)
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---"); end >= 0 {
			frontmatter := body[4 : 4+end]
			body = strings.TrimLeft(body[4+end+4:], "\n")
			parseFrontmatter(skill, frontmatter)
		}
	}
	skill.Content = body

	if skill.Name == "" {
		skill.Name = slug
	}

	// Extract description from frontmatter first; fall back to first paragraph
	if skill.Description == "" {
		skill.Description = extractDescription(body)
	}

	// Load references/ directory
	refsDir := filepath.Join(dir, "references")
	if entries, err := os.ReadDir(refsDir); err == nil {
		skill.References = make(map[string]string)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(refsDir, e.Name()))
			if err == nil {
				skill.References[e.Name()] = string(data)
			}
		}
	}

	// List scripts/ directory
	scriptsDir := filepath.Join(dir, "scripts")
	if entries, err := os.ReadDir(scriptsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			skill.Scripts = append(skill.Scripts, e.Name())
		}
	}

	return skill
}

// parseFrontmatter extracts key: value pairs from YAML frontmatter.
// Handles simple scalar values only (no nested structures).
func parseFrontmatter(skill *Skill, fm string) {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "name":
			skill.Name = val
		case "description":
			skill.Description = val
		case "always_load":
			skill.AlwaysLoad = val == "true"
		}
	}
}

func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	var desc []string
	pastHeading := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			pastHeading = true
			continue
		}
		if !pastHeading && len(desc) == 0 {
			// If no heading, treat first non-empty line as start
			if trimmed != "" {
				desc = append(desc, trimmed)
			}
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

// GetSkill implements tool.SkillProvider.
func (l *SkillsLoader) GetSkill(slug string) (*tool.SkillEntry, bool) {
	s, ok := l.Get(slug)
	if !ok {
		return nil, false
	}
	return &tool.SkillEntry{
		Slug:        s.Slug,
		Name:        s.Name,
		Description: s.Description,
		Content:     s.Content,
		References:  s.References,
		Scripts:     s.Scripts,
		Dir:         s.Dir,
	}, true
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
		fmt.Fprintf(&b, "- **%s** (`%s`)%s: %s", s.Name, s.Slug, marker, s.Description)
		if len(s.References) > 0 {
			names := make([]string, 0, len(s.References))
			for name := range s.References {
				names = append(names, name)
			}
			fmt.Fprintf(&b, " [refs: %s]", strings.Join(names, ", "))
		}
		if len(s.Scripts) > 0 {
			fmt.Fprintf(&b, " [scripts: %s]", strings.Join(s.Scripts, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// BuildAlwaysLoadedContext returns the combined content of all always_load skills
// including their reference documents.
func (l *SkillsLoader) BuildAlwaysLoadedContext() string {
	var parts []string
	for _, s := range l.skills {
		if !s.AlwaysLoad {
			continue
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "### Skill: %s\n\n%s", s.Name, s.Content)
		for name, content := range s.References {
			fmt.Fprintf(&sb, "\n\n#### Reference: %s\n\n%s", name, content)
		}
		if len(s.Scripts) > 0 {
			sb.WriteString("\n\n#### Scripts\n\n")
			for _, name := range s.Scripts {
				fmt.Fprintf(&sb, "- `%s`\n", filepath.Join(s.Dir, "scripts", name))
			}
		}
		parts = append(parts, sb.String())
	}
	return strings.Join(parts, "\n\n---\n\n")
}
