package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// SkillEntry is the subset of a skill exposed to the tool layer.
type SkillEntry struct {
	Slug        string
	Name        string
	Description string
	Content     string
	References  map[string]string
	Scripts     []string
	Dir         string
}

// SkillProvider gives the tool access to loaded skills without depending on the agent package.
type SkillProvider interface {
	GetSkill(slug string) (*SkillEntry, bool)
}

// LoadSkillTool lets an agent load an on-demand skill's full content and references.
type LoadSkillTool struct {
	Provider SkillProvider
}

func (t *LoadSkillTool) Name() string { return "load_skill" }
func (t *LoadSkillTool) Description() string {
	return "Load a skill by slug to get its full instructions and reference documents."
}
func (t *LoadSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"slug"},
		"properties": map[string]any{
			"slug": map[string]any{
				"type":        "string",
				"description": "The skill slug (shown in Available Skills list).",
			},
		},
	}
}

func (t *LoadSkillTool) Execute(_ context.Context, params map[string]any) (string, error) {
	slug, _ := params["slug"].(string)
	if slug == "" {
		return "", fmt.Errorf("slug is required")
	}

	entry, ok := t.Provider.GetSkill(slug)
	if !ok {
		return fmt.Sprintf("Skill %q not found.", slug), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s", entry.Name, entry.Content)

	for name, content := range entry.References {
		fmt.Fprintf(&b, "\n\n---\n\n## Reference: %s\n\n%s", name, content)
	}

	if len(entry.Scripts) > 0 {
		b.WriteString("\n\n---\n\n## Scripts\n\n")
		for _, name := range entry.Scripts {
			fmt.Fprintf(&b, "- `%s`\n", filepath.Join(entry.Dir, "scripts", name))
		}
	}

	return b.String(), nil
}
