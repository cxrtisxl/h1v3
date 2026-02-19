package slackconn

import "testing"

func TestMarkdownToMrkdwn_Bold(t *testing.T) {
	got := MarkdownToMrkdwn("This is **bold** text")
	want := "This is *bold* text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToMrkdwn_Italic(t *testing.T) {
	got := MarkdownToMrkdwn("This is *italic* text")
	want := "This is _italic_ text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToMrkdwn_BoldAndItalic(t *testing.T) {
	got := MarkdownToMrkdwn("**bold** and *italic*")
	want := "*bold* and _italic_"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToMrkdwn_Strikethrough(t *testing.T) {
	got := MarkdownToMrkdwn("~~deleted~~ text")
	want := "~deleted~ text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToMrkdwn_Links(t *testing.T) {
	got := MarkdownToMrkdwn("Click [here](https://example.com) now")
	want := "Click <https://example.com|here> now"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToMrkdwn_CodePreserved(t *testing.T) {
	got := MarkdownToMrkdwn("Use `*not bold*` in code")
	want := "Use `*not bold*` in code"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownToMrkdwn_CodeBlock(t *testing.T) {
	input := "```\ncode here\n```"
	got := MarkdownToMrkdwn(input)
	if got != input {
		t.Errorf("code block should be preserved: got %q", got)
	}
}

func TestMarkdownToMrkdwn_PlainText(t *testing.T) {
	input := "Just plain text with no formatting"
	got := MarkdownToMrkdwn(input)
	if got != input {
		t.Errorf("plain text should be unchanged: got %q", got)
	}
}

func TestStripMention(t *testing.T) {
	tests := []struct {
		input string
		botID string
		want  string
	}{
		{"<@U123> hello", "U123", "hello"},
		{"hey <@U123> there", "U123", "hey  there"},
		{"no mention here", "U123", "no mention here"},
		{"<@U999> hello", "U123", "<@U999> hello"},
	}

	for _, tt := range tests {
		got := StripMention(tt.input, tt.botID)
		if got != tt.want {
			t.Errorf("StripMention(%q, %q) = %q, want %q", tt.input, tt.botID, got, tt.want)
		}
	}
}

func TestIsAllowedChannel(t *testing.T) {
	c := &Connector{config: Config{Channels: []string{"C001", "C002"}}}

	if !c.isAllowedChannel("C001") {
		t.Error("C001 should be allowed")
	}
	if !c.isAllowedChannel("C002") {
		t.Error("C002 should be allowed")
	}
	if c.isAllowedChannel("C999") {
		t.Error("C999 should not be allowed")
	}
}

func TestIsAllowedChannel_Empty(t *testing.T) {
	c := &Connector{config: Config{}}

	if !c.isAllowedChannel("anything") {
		t.Error("empty channels list should allow all")
	}
}

func TestConvertLinks_Multiple(t *testing.T) {
	got := convertLinks("[a](http://a.com) and [b](http://b.com)")
	want := "<http://a.com|a> and <http://b.com|b>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertLinks_Incomplete(t *testing.T) {
	// Incomplete link syntax should be left as-is
	got := convertLinks("[no link here")
	want := "[no link here"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConnectorName(t *testing.T) {
	c := &Connector{}
	if c.Name() != "slack" {
		t.Errorf("Name() = %q", c.Name())
	}
}
