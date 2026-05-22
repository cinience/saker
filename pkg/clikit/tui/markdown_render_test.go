package tui

import (
	"strings"
	"testing"
)

func init() {
	SetMarkdownTheme(DefaultTheme())
}

func stripANSI(s string) string {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip CSI sequences: ESC [ ... final_byte
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && s[i] < 0x40 {
					i++
				}
				if i < len(s) {
					i++ // skip final byte
				}
				continue
			}
			// Skip OSC sequences: ESC ] ... BEL
			if i+1 < len(s) && s[i+1] == ']' {
				i += 2
				for i < len(s) && s[i] != '\x07' {
					i++
				}
				if i < len(s) {
					i++ // skip BEL
				}
				continue
			}
			i++
			continue
		}
		buf.WriteByte(s[i])
		i++
	}
	return buf.String()
}

func TestPlainText(t *testing.T) {
	result := mdRender("hello world", 80, DefaultTheme())
	if result != "hello world" {
		t.Errorf("plain text should pass through unchanged, got %q", result)
	}
}

func TestHeadingH1(t *testing.T) {
	result := mdRender("# Title", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "Title") {
		t.Errorf("H1 should contain title text, got %q", plain)
	}
	if !strings.Contains(result, "\x1b[") {
		t.Errorf("H1 should contain ANSI codes for bold/italic/underline")
	}
}

func TestHeadingH2(t *testing.T) {
	result := mdRender("## Section", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "Section") {
		t.Errorf("H2 should contain text, got %q", plain)
	}
	if !strings.Contains(result, "\x1b[1m") {
		t.Errorf("H2 should be bold")
	}
}

func TestBold(t *testing.T) {
	result := mdRender("some **bold** text", 80, DefaultTheme())
	if !strings.Contains(result, "\x1b[1m") {
		t.Errorf("bold should produce ANSI bold code, got %q", result)
	}
	plain := stripANSI(result)
	if !strings.Contains(plain, "bold") {
		t.Errorf("should contain bold text, got %q", plain)
	}
}

func TestItalic(t *testing.T) {
	result := mdRender("some *italic* text", 80, DefaultTheme())
	if !strings.Contains(result, "\x1b[3m") {
		t.Errorf("italic should produce ANSI italic code, got %q", result)
	}
}

func TestInlineCode(t *testing.T) {
	result := mdRender("use `fmt.Println`", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "fmt.Println") {
		t.Errorf("inline code should contain text, got %q", plain)
	}
	if !strings.Contains(result, "\x1b[") {
		t.Errorf("inline code should have ANSI color")
	}
}

func TestFencedCodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "func main()") {
		t.Errorf("code block should contain code, got %q", plain)
	}
}

func TestFencedCodeBlockLanguageLabel(t *testing.T) {
	input := "```python\nprint('hello')\n```"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "python") {
		t.Errorf("code block should show language label, got %q", plain)
	}
}

func TestBlockquote(t *testing.T) {
	result := mdRender("> quoted text", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, IconBlockquoteBar) {
		t.Errorf("blockquote should have ▎ prefix, got %q", plain)
	}
	if !strings.Contains(plain, "quoted text") {
		t.Errorf("blockquote should contain text, got %q", plain)
	}
}

func TestBlockquoteSeparatorLine(t *testing.T) {
	input := "> para 1\n>\n> para 2"
	result := mdRender(input, 80, DefaultTheme())
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		plain := stripANSI(line)
		trimmed := strings.TrimSpace(plain)
		if trimmed == "" {
			continue
		}
		if !strings.Contains(plain, IconBlockquoteBar) {
			t.Errorf("all non-empty blockquote lines should have ▎ prefix, line without: %q", plain)
		}
	}
}

func TestUnorderedList(t *testing.T) {
	input := "- item 1\n- item 2\n- item 3"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "- item 1") {
		t.Errorf("unordered list should have dash bullets, got %q", plain)
	}
	if strings.Count(plain, "- ") < 3 {
		t.Errorf("should have 3 bullet items, got %q", plain)
	}
}

func TestOrderedList(t *testing.T) {
	input := "1. first\n2. second\n3. third"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "1. first") {
		t.Errorf("ordered list should have numbered items, got %q", plain)
	}
	if !strings.Contains(plain, "2. second") {
		t.Errorf("should have item 2, got %q", plain)
	}
}

func TestTightListNoExtraBlankLine(t *testing.T) {
	input := "- a\n- b\n- c"
	result := mdRender(input, 80, DefaultTheme())
	if strings.Contains(result, "\n\n") {
		t.Errorf("tight list should not have blank lines between items, got %q", stripANSI(result))
	}
}

func TestLooseListHasBlankLines(t *testing.T) {
	input := "- a\n\n- b\n\n- c"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "\n\n") {
		t.Errorf("loose list should have blank lines between items, got %q", plain)
	}
}

func TestTable(t *testing.T) {
	input := "| A | B |\n|---|---|\n| 1 | 2 |"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "┌") || !strings.Contains(plain, "┐") {
		t.Errorf("table should have box-drawing top border, got %q", plain)
	}
	if !strings.Contains(plain, "└") || !strings.Contains(plain, "┘") {
		t.Errorf("table should have box-drawing bottom border, got %q", plain)
	}
	if !strings.Contains(plain, "│") {
		t.Errorf("table should use │ cell borders, got %q", plain)
	}
	if !strings.Contains(plain, "A") || !strings.Contains(plain, "B") {
		t.Errorf("table should contain header cells, got %q", plain)
	}
}

func TestHorizontalRule(t *testing.T) {
	result := mdRender("---", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "---") {
		t.Errorf("horizontal rule should render as ---, got %q", plain)
	}
}

func TestLink(t *testing.T) {
	result := mdRender("[click](https://example.com)", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "click") {
		t.Errorf("link should show display text, got %q", plain)
	}
	if !strings.Contains(result, "\x1b]8;;https://example.com\x07") {
		t.Errorf("link should contain OSC 8 hyperlink sequence, got %q", result)
	}
}

func TestMailtoLink(t *testing.T) {
	result := mdRender("[email](mailto:user@example.com)", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "user@example.com") {
		t.Errorf("mailto link should show email, got %q", plain)
	}
	if strings.Contains(result, "\x1b]8;;") {
		t.Errorf("mailto link should NOT be rendered as hyperlink")
	}
}

func TestStrikethrough(t *testing.T) {
	result := mdRender("~~deleted~~", 80, DefaultTheme())
	if !strings.Contains(result, "\x1b[9m") {
		t.Errorf("strikethrough should produce ANSI strikethrough code, got %q", result)
	}
}

func TestTaskList(t *testing.T) {
	input := "- [x] done\n- [ ] todo"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "[x]") {
		t.Errorf("should contain checked box, got %q", plain)
	}
	if !strings.Contains(plain, "[ ]") {
		t.Errorf("should contain unchecked box, got %q", plain)
	}
}

func TestIssueReference(t *testing.T) {
	result := mdRender("see anthropics/claude-code#123 for details", 80, DefaultTheme())
	if !strings.Contains(result, "https://github.com/anthropics/claude-code/issues/123") {
		t.Errorf("issue reference should be linkified, got %q", result)
	}
}

func TestParagraphSeparation(t *testing.T) {
	input := "paragraph one\n\nparagraph two"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "\n\n") {
		t.Errorf("paragraphs should be separated by blank line, got %q", plain)
	}
}

func TestWordWrap(t *testing.T) {
	long := strings.Repeat("word ", 30)
	result := mdRender(long, 40, DefaultTheme())
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Errorf("long text should wrap to multiple lines at width 40, got %d lines", len(lines))
	}
}

func TestCodeBlockNoWrap(t *testing.T) {
	longCode := "```\n" + strings.Repeat("x", 200) + "\n```"
	result := mdRender(longCode, 40, DefaultTheme())
	plain := stripANSI(result)
	for _, line := range strings.Split(plain, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 200 {
			continue
		}
		if strings.Contains(trimmed, strings.Repeat("x", 50)) {
			return
		}
	}
	t.Errorf("code block should not word-wrap, got %q", plain)
}

func TestNestedBlockquoteList(t *testing.T) {
	input := "> - item in quote"
	result := mdRender(input, 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, IconBlockquoteBar) {
		t.Errorf("nested blockquote+list should have ▎, got %q", plain)
	}
	if !strings.Contains(plain, "- ") {
		t.Errorf("nested blockquote+list should have bullet, got %q", plain)
	}
}

func TestBoldItalicNested(t *testing.T) {
	result := mdRender("***bold italic***", 80, DefaultTheme())
	plain := stripANSI(result)
	if !strings.Contains(plain, "bold italic") {
		t.Errorf("should contain text, got %q", plain)
	}
	// lipgloss may combine bold+italic as \x1b[1;3m or separate sequences
	hasBoldItalic := strings.Contains(result, "\x1b[1;3m") ||
		(strings.Contains(result, "\x1b[1m") && strings.Contains(result, "\x1b[3m"))
	if !hasBoldItalic {
		t.Errorf("should contain bold+italic ANSI codes, got %q", result)
	}
}

func TestStreamingRenderer(t *testing.T) {
	sr := NewStreamingRenderer()
	r1 := sr.Render("hello **world**\n\nnew paragraph", 80)
	r2 := sr.Render("hello **world**\n\nnew paragraph more", 80)
	if r1 == "" || r2 == "" {
		t.Errorf("streaming renderer should produce output")
	}
	if !strings.Contains(stripANSI(r2), "more") {
		t.Errorf("updated render should contain new text")
	}
}

func TestHighlightCode(t *testing.T) {
	result := highlightCode("func main() {}", "go")
	if !strings.Contains(result, "\x1b[") {
		t.Errorf("syntax highlighting should produce ANSI codes, got %q", result)
	}
	plain := stripANSI(result)
	if !strings.Contains(plain, "func") {
		t.Errorf("highlighted code should contain source text, got %q", plain)
	}
}

func TestHighlightCodeUnknownLanguage(t *testing.T) {
	result := highlightCode("some code", "nonexistent_lang_xyz")
	plain := stripANSI(result)
	if !strings.Contains(plain, "some code") {
		t.Errorf("unknown language should fallback to plain text, got %q", plain)
	}
}
