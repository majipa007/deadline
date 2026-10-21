package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// forceColor enables styling for one test; without a TTY lipgloss renders
// plain text.
func forceColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// mdPlain renders src and strips the styling.
func mdPlain(t *testing.T, src string, width int) string {
	t.Helper()
	return stripANSI(renderMarkdown(src, width))
}

func TestRenderMarkdownPlainTextWraps(t *testing.T) {
	out := mdPlain(t, "one two three four five", 13)
	rows := strings.Split(out, "\n")
	if len(rows) != 2 || rows[0] != "one two three" || rows[1] != "four five" {
		t.Errorf("wrapped = %q, want two tidy rows", rows)
	}
}

func TestRenderMarkdownHeadings(t *testing.T) {
	forceColor(t)
	out := mdPlain(t, "## Real fix\n### detail", 40)
	if strings.Contains(out, "#") {
		t.Errorf("heading markers survived:\n%s", out)
	}
	for _, want := range []string{"Real fix", "detail"} {
		if !strings.Contains(out, want) {
			t.Errorf("heading text %q missing:\n%s", want, out)
		}
	}
	// Styled, not just stripped: the raw render must carry escapes.
	if renderMarkdown("## Real fix", 40) == "Real fix" {
		t.Error("heading rendered with no styling at all")
	}
}

func TestRenderMarkdownInlineSpans(t *testing.T) {
	forceColor(t)
	out := mdPlain(t, "The **bold** and *italic* and `code` done", 60)
	for _, want := range []string{"The bold and italic and code done"} {
		if out != want {
			t.Errorf("inline = %q, want %q", out, want)
		}
	}
	raw := renderMarkdown("**bold**", 60)
	if raw == "bold" || !strings.Contains(stripANSI(raw), "bold") {
		t.Errorf("bold has no styling or lost text: %q", raw)
	}
}

func TestRenderMarkdownLeavesNonMarkupAlone(t *testing.T) {
	for _, src := range []string{
		"5 * 3 = 15",
		"upload_original_pdf swallows it",
		"unclosed **bold and _under and `tick",
		"[brackets] without a target",
		"a*b is not emphasis",
	} {
		if got := mdPlain(t, src, 60); got != src {
			t.Errorf("renderMarkdown(%q) = %q, want it literal", src, got)
		}
	}
}

func TestRenderMarkdownListsHangTheirContinuations(t *testing.T) {
	out := mdPlain(t, "- short\n- second item wrapping far enough to continue", 30)
	rows := strings.Split(out, "\n")
	if rows[0] != "• short" {
		t.Errorf("first row = %q, want the professional bullet marker", rows[0])
	}
	if !strings.HasPrefix(rows[2], "  ") || strings.TrimSpace(rows[2]) == "" {
		t.Errorf("continuation is not indented:\n%s", out)
	}
	num := mdPlain(t, "1. first step", 30)
	if num != "1. first step" {
		t.Errorf("numbered item = %q, want the marker kept", num)
	}
}

// Hard-wrapped list items are soft wraps: continuations keep the hanging
// indent.
func TestRenderMarkdownHardWrappedListKeepsIndent(t *testing.T) {
	src := "(1) First line from a pasted report that keeps going\n" +
		"onto a source-wrapped continuation and then\n" +
		"one final fragment before the blank.\n\nNext paragraph."
	rows := strings.Split(mdPlain(t, src, 46), "\n")
	if !strings.HasPrefix(rows[0], "1. ") {
		t.Fatalf("first row lost ordered marker: %q", rows[0])
	}
	for i := 1; i < len(rows) && rows[i] != ""; i++ {
		if !strings.HasPrefix(rows[i], "   ") {
			t.Errorf("continuation row %d is flush-left: %q", i, rows[i])
		}
	}
	if !strings.Contains(strings.Join(rows, " "), "final fragment") {
		t.Errorf("hard-wrap normalization dropped text:\n%s", strings.Join(rows, "\n"))
	}
}

func TestRenderMarkdownFenceTruncatesInsteadOfWrapping(t *testing.T) {
	out := mdPlain(t, "```\nabcdefghijklmnopqrstuvwxyz0123456789abcd\n```", 20)
	rows := strings.Split(out, "\n")
	if len(rows) != 1 {
		t.Fatalf("fence produced %d rows, want 1 truncated row", len(rows))
	}
	if !strings.HasSuffix(rows[0], "…") {
		t.Errorf("fence row = %q, want an ellipsis cut", rows[0])
	}
	if strings.Contains(out, "```") {
		t.Errorf("fence markers survived:\n%s", out)
	}
}

func TestRenderMarkdownLinksAndRules(t *testing.T) {
	out := mdPlain(t, "[docs](https://example.com/x)", 60)
	if out != "docs (https://example.com/x)" {
		t.Errorf("link = %q, want text plus muted url", out)
	}
	rule := mdPlain(t, "---", 10)
	if strings.Count(rule, "─") != 10 {
		t.Errorf("rule = %q, want a full-width divider", rule)
	}
}

// Every row fits width, measured with styling escapes in place.
func TestRenderMarkdownRowsNeverExceedWidth(t *testing.T) {
	srcs := []string{
		"## Heading with **bold** and *italic* inside",
		"The **AccountId-drift** card breaks *replace* when snake_case stays.",
		"- list item with `code` and [a link](https://example.com/very/long/path/here)",
		"```\n" + strings.Repeat("x", 200) + "\n```",
		"session cbea686c-5da4-4ea9-984f-d4de600c0098 keeps wrapping",
		"1. numbered with a very long line that must wrap onto continuations",
		"---",
		"",
	}
	for _, width := range []int{10, 20, 40, 80} {
		for _, src := range srcs {
			for i, row := range strings.Split(renderMarkdown(src, width), "\n") {
				if got := lipgloss.Width(row); got > width {
					t.Errorf("width=%d row %d is %d cols, over budget: %q", width, i, got, row)
				}
			}
		}
	}
}

func TestStructureProseSplitsEnumerations(t *testing.T) {
	src := "Blah blah. (1) Five things happened here today. Done first. (2) The rest follows."
	out := mdPlain(t, src, 60)
	for _, want := range []string{"\n1. Five things happened here today.", "\n2. The rest follows."} {
		if !strings.Contains(out, want) {
			t.Errorf("enumeration never became its own block:\n%s", out)
		}
	}
}

func TestStructureProseLeavesNonPointsAlone(t *testing.T) {
	for _, src := range []string{
		"run 18/09/2026 (session cbea686c-5da4) went fine",
		"options (1) and (2) stay on their line",
		"a quick fix: later has no sentence before it",
		"something end. fix: lower stays put",
		"(see fig. 3) for the diagram",
		"already split\n(1) Five things",
	} {
		out := mdPlain(t, src, 60)
		if strings.Count(out, "\n\n") > strings.Count(src, "\n\n") {
			t.Errorf("structureProse split %q, want it untouched:\n%s", src, out)
		}
	}
}

func TestStructureProseSplitsAdmonitions(t *testing.T) {
	out := mdPlain(t, "own statement. Fix: let it rest here.", 60)
	if !strings.Contains(out, "\nFix: let it rest here.") {
		t.Errorf("Fix: never became its own paragraph:\n%s", out)
	}
	forceColor(t)
	raw := renderMarkdown("own statement. Fix: let it rest.", 60)
	head := strings.Split(stripANSI(raw), "\n")
	found := false
	for _, row := range head {
		if strings.HasPrefix(row, "Fix:") {
			found = true
		}
	}
	if !found {
		t.Errorf("Fix: label is not row-leading:\n%s", raw)
	}
	_ = head
}
