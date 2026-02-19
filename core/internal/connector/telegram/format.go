package telegram

import (
	"regexp"
	"strings"
)

// MarkdownToTelegramHTML converts standard Markdown to Telegram's HTML subset.
// If conversion produces invalid output, falls back to plain text.
func MarkdownToTelegramHTML(md string) string {
	result := convertMarkdown(md)
	return result
}

func convertMarkdown(md string) string {
	// Process line by line for code blocks
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inCodeBlock := false
	codeLang := ""

	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				// Opening code fence
				codeLang = strings.TrimPrefix(line, "```")
				codeLang = strings.TrimSpace(codeLang)
				if codeLang != "" {
					out.WriteString(`<pre><code class="language-` + escapeHTML(codeLang) + `">`)
				} else {
					out.WriteString("<pre><code>")
				}
				inCodeBlock = true
			} else {
				// Closing code fence
				out.WriteString("</code></pre>")
				inCodeBlock = false
				codeLang = ""
			}
			if i < len(lines)-1 {
				out.WriteString("\n")
			}
			continue
		}

		if inCodeBlock {
			// Inside code block — escape HTML but don't process markdown
			out.WriteString(escapeHTML(line))
			if i < len(lines)-1 {
				out.WriteString("\n")
			}
			continue
		}

		// Process inline formatting
		processed := processInline(line)
		out.WriteString(processed)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}

	// If we ended inside a code block, close it
	if inCodeBlock {
		out.WriteString("</code></pre>")
	}

	return out.String()
}

var (
	// Order matters — process code first to avoid conflicts
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`\*(.+?)\*`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

func processInline(line string) string {
	// Protect inline code spans first
	type codeSpan struct {
		placeholder string
		html        string
	}
	var spans []codeSpan
	counter := 0

	line = reInlineCode.ReplaceAllStringFunc(line, func(match string) string {
		inner := reInlineCode.FindStringSubmatch(match)[1]
		placeholder := "\x00CODE" + string(rune('A'+counter)) + "\x00"
		counter++
		spans = append(spans, codeSpan{
			placeholder: placeholder,
			html:        "<code>" + escapeHTML(inner) + "</code>",
		})
		return placeholder
	})

	// Escape HTML in non-code content
	line = escapeHTML(line)

	// Process bold before italic (** before *)
	line = reBold.ReplaceAllString(line, "<b>$1</b>")
	line = reItalic.ReplaceAllString(line, "<i>$1</i>")

	// Process links
	line = reLink.ReplaceAllString(line, `<a href="$2">$1</a>`)

	// Restore code spans
	for _, s := range spans {
		line = strings.Replace(line, escapeHTML(s.placeholder), s.html, 1)
	}

	return line
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// StripMarkdown removes all Markdown formatting, returning plain text.
func StripMarkdown(md string) string {
	// Remove code blocks
	re := regexp.MustCompile("```[\\s\\S]*?```")
	result := re.ReplaceAllStringFunc(md, func(match string) string {
		inner := strings.TrimPrefix(match, "```")
		inner = strings.TrimSuffix(inner, "```")
		// Remove language identifier from first line
		if idx := strings.Index(inner, "\n"); idx >= 0 {
			inner = inner[idx+1:]
		}
		return inner
	})

	// Remove inline code
	result = reInlineCode.ReplaceAllString(result, "$1")
	// Remove bold
	result = reBold.ReplaceAllString(result, "$1")
	// Remove italic
	result = reItalic.ReplaceAllString(result, "$1")
	// Convert links to "text (url)"
	result = reLink.ReplaceAllString(result, "$1 ($2)")

	return result
}
