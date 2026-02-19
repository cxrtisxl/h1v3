package telegram

import (
	"strings"
	"testing"
)

func TestBold(t *testing.T) {
	got := MarkdownToTelegramHTML("This is **bold** text")
	if !strings.Contains(got, "<b>bold</b>") {
		t.Errorf("expected bold tag, got %q", got)
	}
}

func TestItalic(t *testing.T) {
	got := MarkdownToTelegramHTML("This is *italic* text")
	if !strings.Contains(got, "<i>italic</i>") {
		t.Errorf("expected italic tag, got %q", got)
	}
}

func TestInlineCode(t *testing.T) {
	got := MarkdownToTelegramHTML("Use `fmt.Println` here")
	if !strings.Contains(got, "<code>fmt.Println</code>") {
		t.Errorf("expected code tag, got %q", got)
	}
}

func TestCodeBlock(t *testing.T) {
	md := "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	got := MarkdownToTelegramHTML(md)
	if !strings.Contains(got, `<pre><code class="language-go">`) {
		t.Errorf("expected pre/code with language, got %q", got)
	}
	if !strings.Contains(got, "</code></pre>") {
		t.Errorf("expected closing pre/code, got %q", got)
	}
	// Code content should be HTML-escaped
	if !strings.Contains(got, "fmt.Println") {
		t.Errorf("expected code content, got %q", got)
	}
}

func TestCodeBlockNoLang(t *testing.T) {
	md := "```\nhello\n```"
	got := MarkdownToTelegramHTML(md)
	if !strings.Contains(got, "<pre><code>") {
		t.Errorf("expected pre/code without language, got %q", got)
	}
}

func TestLink(t *testing.T) {
	got := MarkdownToTelegramHTML("Click [here](https://example.com)")
	if !strings.Contains(got, `<a href="https://example.com">here</a>`) {
		t.Errorf("expected link tag, got %q", got)
	}
}

func TestHTMLEscaping(t *testing.T) {
	got := MarkdownToTelegramHTML("Use <script> & tags")
	if strings.Contains(got, "<script>") {
		t.Errorf("expected HTML escaped, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped angle brackets, got %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("expected escaped ampersand, got %q", got)
	}
}

func TestBoldAndItalic(t *testing.T) {
	got := MarkdownToTelegramHTML("**bold** and *italic*")
	if !strings.Contains(got, "<b>bold</b>") {
		t.Errorf("expected bold, got %q", got)
	}
	if !strings.Contains(got, "<i>italic</i>") {
		t.Errorf("expected italic, got %q", got)
	}
}

func TestPlainText(t *testing.T) {
	input := "Just plain text, nothing special."
	got := MarkdownToTelegramHTML(input)
	if got != input {
		t.Errorf("expected unchanged text, got %q", got)
	}
}

func TestStripMarkdown(t *testing.T) {
	md := "**bold** and *italic* with `code` and [link](https://example.com)"
	got := StripMarkdown(md)
	if strings.Contains(got, "**") || strings.Contains(got, "*") || strings.Contains(got, "`") {
		t.Errorf("expected stripped markdown, got %q", got)
	}
	if !strings.Contains(got, "bold") || !strings.Contains(got, "italic") {
		t.Errorf("expected text content preserved, got %q", got)
	}
	if !strings.Contains(got, "link (https://example.com)") {
		t.Errorf("expected link converted, got %q", got)
	}
}

func TestCodeBlockHTMLEscaping(t *testing.T) {
	md := "```html\n<div>test</div>\n```"
	got := MarkdownToTelegramHTML(md)
	if strings.Contains(got, "<div>") {
		t.Errorf("expected HTML in code block to be escaped, got %q", got)
	}
	if !strings.Contains(got, "&lt;div&gt;") {
		t.Errorf("expected escaped HTML in code block, got %q", got)
	}
}
