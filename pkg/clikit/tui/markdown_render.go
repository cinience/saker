package tui

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var mdParser parser.Parser

func init() {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(),
			extension.Strikethrough,
			extension.TaskList,
		),
	)
	mdParser = md.Parser()
}

// issueRefRE matches owner/repo#NNN GitHub issue/PR references.
var issueRefRE = regexp.MustCompile(`(^|[^\w./-])([A-Za-z0-9][\w-]*/[A-Za-z0-9][\w.-]*)#(\d+)\b`)

func mdRender(source string, width int, theme Theme) string {
	src := []byte(source)
	doc := mdParser.Parse(text.NewReader(src))
	r := &mdRenderer{
		src:   src,
		width: width,
		theme: theme,
	}
	_ = gast.Walk(doc, r.walk)
	return strings.TrimRight(r.buf.String(), "\n")
}

type styleFrame struct {
	bold          bool
	italic        bool
	underline     bool
	strikethrough bool
	fgColor       color.Color
}

type tableState struct {
	alignments []east.Alignment
	header     []string
	rows       [][]string
	curRow     []string
	cellBuf    strings.Builder
	inHeader   bool
}

type mdRenderer struct {
	src   []byte
	width int
	theme Theme
	buf   strings.Builder

	styleStack []styleFrame

	listDepth    int
	listCounters []int

	bqDepth int

	table *tableState

	linkBuf *strings.Builder
}

func (r *mdRenderer) pushStyle(f styleFrame) {
	r.styleStack = append(r.styleStack, f)
}

func (r *mdRenderer) popStyle() {
	if len(r.styleStack) > 0 {
		r.styleStack = r.styleStack[:len(r.styleStack)-1]
	}
}

func (r *mdRenderer) applyStyle(s string) string {
	if len(r.styleStack) == 0 {
		return s
	}
	style := lipgloss.NewStyle()
	var fg color.Color
	for _, f := range r.styleStack {
		if f.bold {
			style = style.Bold(true)
		}
		if f.italic {
			style = style.Italic(true)
		}
		if f.underline {
			style = style.Underline(true)
		}
		if f.strikethrough {
			style = style.Strikethrough(true)
		}
		if f.fgColor != nil {
			fg = f.fgColor
		}
	}
	if fg != nil {
		style = style.Foreground(fg)
	}
	return style.Render(s)
}

func (r *mdRenderer) writeString(s string) {
	if r.table != nil {
		r.table.cellBuf.WriteString(s)
		return
	}
	if r.linkBuf != nil {
		r.linkBuf.WriteString(s)
		return
	}
	r.buf.WriteString(s)
}

func (r *mdRenderer) writeStyledText(s string) {
	r.writeString(r.applyStyle(s))
}

func (r *mdRenderer) writeBqPrefix() {
	if r.bqDepth <= 0 {
		return
	}
	barStyle := lipgloss.NewStyle().Foreground(r.theme.FgDim)
	prefix := barStyle.Render(IconBlockquoteBar) + " "
	for i := 0; i < r.bqDepth; i++ {
		r.buf.WriteString(prefix)
	}
}

func (r *mdRenderer) effectiveWidth() int {
	w := r.width - r.bqDepth*3 - r.listDepth*2
	if w < 20 {
		w = 20
	}
	return w
}

// writeWrapped word-wraps raw text and applies the current style stack
// to each wrapped line independently, so continuation lines retain styling.
func (r *mdRenderer) writeWrapped(s string) {
	if r.table != nil || r.linkBuf != nil {
		r.writeString(r.applyStyle(s))
		return
	}

	w := r.effectiveWidth()
	wrapped := ansi.Wordwrap(s, w, "")
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		if i > 0 {
			r.buf.WriteString("\n")
			r.writeBqPrefix()
			r.writeListIndent()
		}
		r.buf.WriteString(r.applyStyle(line))
	}
}

func (r *mdRenderer) writeListIndent() {
	if r.listDepth > 0 {
		r.buf.WriteString(strings.Repeat("  ", r.listDepth))
	}
}

// writeBlockSeparator writes a blank line between block elements.
// In blockquote context, the blank line gets the ▎ prefix so the
// visual bar is continuous.
func (r *mdRenderer) writeBlockSeparator() {
	r.buf.WriteString("\n")
	r.writeBqPrefix()
	r.buf.WriteString("\n")
}

func (r *mdRenderer) codeBlockText(n gast.Node) string {
	var sb strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		sb.Write(seg.Value(r.src))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (r *mdRenderer) collectText(n gast.Node) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *gast.Text:
			sb.Write(v.Segment.Value(r.src))
		case *gast.String:
			sb.Write(v.Value)
		default:
			sb.WriteString(r.collectText(c))
		}
	}
	return sb.String()
}

func (r *mdRenderer) renderHyperlink(displayText, url string) string {
	linkStyle := lipgloss.NewStyle().Foreground(r.theme.Link)
	styled := linkStyle.Render(displayText)
	return ansi.SetHyperlink(url) + styled + ansi.ResetHyperlink()
}

func (r *mdRenderer) linkifyIssueReferences(s string) string {
	return issueRefRE.ReplaceAllStringFunc(s, func(match string) string {
		parts := issueRefRE.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		prefix := parts[1]
		repo := parts[2]
		num := parts[3]
		url := "https://github.com/" + repo + "/issues/" + num
		return prefix + r.renderHyperlink(repo+"#"+num, url)
	})
}

func numberToLetter(n int) string {
	var result []byte
	for n > 0 {
		n--
		result = append([]byte{byte('a' + n%26)}, result...)
		n /= 26
	}
	return string(result)
}

func numberToRoman(n int) string {
	vals := []struct {
		v int
		s string
	}{
		{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"},
		{100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"},
		{10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"},
	}
	var sb strings.Builder
	for _, p := range vals {
		for n >= p.v {
			sb.WriteString(p.s)
			n -= p.v
		}
	}
	return sb.String()
}

func (r *mdRenderer) listBullet() string {
	if len(r.listCounters) == 0 {
		return "- "
	}
	counter := r.listCounters[len(r.listCounters)-1]
	if counter == 0 {
		return "- "
	}
	depth := r.listDepth - 1
	switch {
	case depth <= 1:
		return fmt.Sprintf("%d. ", counter)
	case depth == 2:
		return numberToLetter(counter) + ". "
	default:
		return numberToRoman(counter) + ". "
	}
}

func (r *mdRenderer) renderTable() {
	t := r.table
	if t == nil {
		return
	}

	allRows := make([][]string, 0, 1+len(t.rows))
	allRows = append(allRows, t.header)
	allRows = append(allRows, t.rows...)

	numCols := len(t.header)
	if numCols == 0 {
		return
	}

	colWidths := make([]int, numCols)
	for _, row := range allRows {
		for i, cell := range row {
			if i < numCols {
				w := ansi.StringWidth(cell)
				if w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	padCell := func(content string, col int, center bool) string {
		w := ansi.StringWidth(content)
		padding := colWidths[col] - w
		if padding < 0 {
			padding = 0
		}
		if center {
			left := padding / 2
			return strings.Repeat(" ", left) + content + strings.Repeat(" ", padding-left)
		}
		align := east.AlignNone
		if col < len(t.alignments) {
			align = t.alignments[col]
		}
		switch align {
		case east.AlignCenter:
			left := padding / 2
			return strings.Repeat(" ", left) + content + strings.Repeat(" ", padding-left)
		case east.AlignRight:
			return strings.Repeat(" ", padding) + content
		default:
			return content + strings.Repeat(" ", padding)
		}
	}

	borderLine := func(left, horiz, cross, right string) {
		r.writeBqPrefix()
		r.buf.WriteString(left)
		for i, w := range colWidths {
			r.buf.WriteString(strings.Repeat(horiz, w+2))
			if i < numCols-1 {
				r.buf.WriteString(cross)
			} else {
				r.buf.WriteString(right)
			}
		}
		r.buf.WriteString("\n")
	}

	// Top border
	borderLine("┌", "─", "┬", "┐")

	// Header row (centered)
	r.writeBqPrefix()
	r.buf.WriteString("│")
	for i, cell := range t.header {
		r.buf.WriteString(" " + padCell(cell, i, true) + " │")
	}
	r.buf.WriteString("\n")

	// Header separator
	borderLine("├", "─", "┼", "┤")

	// Data rows with separators between them
	for ri, row := range t.rows {
		r.writeBqPrefix()
		r.buf.WriteString("│")
		for i := 0; i < numCols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			r.buf.WriteString(" " + padCell(cell, i, false) + " │")
		}
		r.buf.WriteString("\n")
		if ri < len(t.rows)-1 {
			borderLine("├", "─", "┼", "┤")
		}
	}

	// Bottom border
	borderLine("└", "─", "┴", "┘")
}

func (r *mdRenderer) walk(n gast.Node, entering bool) (gast.WalkStatus, error) {
	kind := n.Kind()

	switch {
	case kind == gast.KindDocument:
		return gast.WalkContinue, nil

	case kind == gast.KindParagraph:
		if !entering {
			r.writeString("\n")
			if !r.isInTightList(n) && n.NextSibling() != nil {
				r.writeBqPrefix()
				r.writeString("\n")
			}
		}
		return gast.WalkContinue, nil

	case kind == gast.KindTextBlock:
		if !entering {
			r.writeString("\n")
		}
		return gast.WalkContinue, nil

	case kind == gast.KindHeading:
		h := n.(*gast.Heading)
		if entering {
			r.writeBqPrefix()
			if h.Level == 1 {
				r.pushStyle(styleFrame{bold: true, italic: true, underline: true})
			} else {
				r.pushStyle(styleFrame{bold: true})
			}
		} else {
			r.popStyle()
			r.writeBlockSeparator()
		}
		return gast.WalkContinue, nil

	case kind == gast.KindThematicBreak:
		if entering {
			r.writeBqPrefix()
			r.writeString("---\n")
		}
		return gast.WalkContinue, nil

	case kind == gast.KindBlockquote:
		if entering {
			r.bqDepth++
		} else {
			r.bqDepth--
		}
		return gast.WalkContinue, nil

	case kind == gast.KindList:
		if entering {
			list := n.(*gast.List)
			r.listDepth++
			if list.IsOrdered() {
				r.listCounters = append(r.listCounters, list.Start)
			} else {
				r.listCounters = append(r.listCounters, 0)
			}
		} else {
			r.listDepth--
			if len(r.listCounters) > 0 {
				r.listCounters = r.listCounters[:len(r.listCounters)-1]
			}
		}
		return gast.WalkContinue, nil

	case kind == gast.KindListItem:
		if entering {
			r.writeBqPrefix()
			indent := strings.Repeat("  ", r.listDepth-1)
			bullet := r.listBullet()
			r.buf.WriteString(indent)
			r.buf.WriteString(bullet)
			if len(r.listCounters) > 0 && r.listCounters[len(r.listCounters)-1] > 0 {
				r.listCounters[len(r.listCounters)-1]++
			}
		} else {
			if !r.isInTightList(n) && n.NextSibling() != nil {
				r.writeBqPrefix()
				r.buf.WriteString("\n")
			}
		}
		return gast.WalkContinue, nil

	case kind == gast.KindFencedCodeBlock:
		if entering {
			fcb := n.(*gast.FencedCodeBlock)
			lang := ""
			if fcb.Language(r.src) != nil {
				lang = string(fcb.Language(r.src))
			}
			code := r.codeBlockText(n)
			highlighted := highlightCode(code, lang)

			if lang != "" {
				r.writeBqPrefix()
				langStyle := lipgloss.NewStyle().Foreground(r.theme.FgDim).Italic(true)
				r.buf.WriteString("  " + langStyle.Render(lang) + "\n")
			}

			lines := strings.Split(highlighted, "\n")
			for _, line := range lines {
				r.writeBqPrefix()
				r.buf.WriteString("  ")
				r.buf.WriteString(line)
				r.buf.WriteString("\n")
			}
			r.buf.WriteString("\n")
		}
		return gast.WalkSkipChildren, nil

	case kind == gast.KindCodeBlock:
		if entering {
			code := r.codeBlockText(n)
			highlighted := highlightCode(code, "")
			lines := strings.Split(highlighted, "\n")
			for _, line := range lines {
				r.writeBqPrefix()
				r.buf.WriteString("  ")
				r.buf.WriteString(line)
				r.buf.WriteString("\n")
			}
			r.buf.WriteString("\n")
		}
		return gast.WalkSkipChildren, nil

	case kind == gast.KindCodeSpan:
		if entering {
			t := r.collectText(n)
			codeStyle := lipgloss.NewStyle().Foreground(r.theme.Permission)
			r.writeString(codeStyle.Render(t))
		}
		return gast.WalkSkipChildren, nil

	case kind == gast.KindEmphasis:
		em := n.(*gast.Emphasis)
		if entering {
			if em.Level == 2 {
				r.pushStyle(styleFrame{bold: true})
			} else {
				r.pushStyle(styleFrame{italic: true})
			}
		} else {
			r.popStyle()
		}
		return gast.WalkContinue, nil

	case kind == gast.KindLink:
		if entering {
			r.linkBuf = &strings.Builder{}
		} else {
			link := n.(*gast.Link)
			url := string(link.Destination)
			linkText := r.linkBuf.String()
			r.linkBuf = nil
			if strings.HasPrefix(url, "mailto:") {
				r.writeString(strings.TrimPrefix(url, "mailto:"))
			} else if linkText != "" && linkText != url {
				r.writeString(r.renderHyperlink(linkText, url))
			} else {
				r.writeString(r.renderHyperlink(url, url))
			}
		}
		return gast.WalkContinue, nil

	case kind == gast.KindAutoLink:
		if entering {
			al := n.(*gast.AutoLink)
			url := string(al.URL(r.src))
			label := string(al.Label(r.src))
			if al.AutoLinkType == gast.AutoLinkEmail {
				r.writeString(label)
			} else {
				r.writeString(r.renderHyperlink(label, url))
			}
		}
		return gast.WalkSkipChildren, nil

	case kind == gast.KindImage:
		if entering {
			img := n.(*gast.Image)
			r.writeString(string(img.Destination))
		}
		return gast.WalkSkipChildren, nil

	case kind == gast.KindText:
		if entering {
			t := n.(*gast.Text)
			value := string(t.Segment.Value(r.src))

			if r.table != nil || r.linkBuf != nil {
				r.writeStyledText(value)
			} else if r.isInListItem(n) {
				value = r.linkifyIssueReferences(value)
				r.writeStyledText(value)
			} else {
				value = r.linkifyIssueReferences(value)
				r.writeBqPrefix()
				r.writeWrapped(value)
			}

			if t.SoftLineBreak() {
				if r.table == nil && r.linkBuf == nil {
					r.writeString("\n")
					r.writeBqPrefix()
					r.writeListIndent()
				} else {
					r.writeString(" ")
				}
			}
			if t.HardLineBreak() {
				r.writeString("\n")
				if r.table == nil && r.linkBuf == nil {
					r.writeBqPrefix()
					r.writeListIndent()
				}
			}
		}
		return gast.WalkContinue, nil

	case kind == gast.KindString:
		if entering {
			s := n.(*gast.String)
			r.writeStyledText(string(s.Value))
		}
		return gast.WalkContinue, nil

	case kind == gast.KindRawHTML || kind == gast.KindHTMLBlock:
		return gast.WalkSkipChildren, nil

	case kind == east.KindTable:
		if entering {
			tbl := n.(*east.Table)
			r.table = &tableState{
				alignments: tbl.Alignments,
			}
		} else {
			r.renderTable()
			r.table = nil
			r.writeString("\n")
		}
		return gast.WalkContinue, nil

	case kind == east.KindTableHeader:
		if entering {
			if r.table != nil {
				r.table.inHeader = true
				r.table.curRow = nil
			}
		} else {
			if r.table != nil {
				r.table.header = r.table.curRow
				r.table.curRow = nil
				r.table.inHeader = false
			}
		}
		return gast.WalkContinue, nil

	case kind == east.KindTableRow:
		if entering {
			if r.table != nil {
				r.table.curRow = nil
			}
		} else {
			if r.table != nil {
				r.table.rows = append(r.table.rows, r.table.curRow)
				r.table.curRow = nil
			}
		}
		return gast.WalkContinue, nil

	case kind == east.KindTableCell:
		if entering {
			if r.table != nil {
				r.table.cellBuf.Reset()
			}
		} else {
			if r.table != nil {
				r.table.curRow = append(r.table.curRow, r.table.cellBuf.String())
				r.table.cellBuf.Reset()
			}
		}
		return gast.WalkContinue, nil

	case kind == east.KindStrikethrough:
		if entering {
			r.pushStyle(styleFrame{strikethrough: true})
		} else {
			r.popStyle()
		}
		return gast.WalkContinue, nil

	case kind == east.KindTaskCheckBox:
		if entering {
			cb := n.(*east.TaskCheckBox)
			if cb.IsChecked {
				r.writeString("[x] ")
			} else {
				r.writeString("[ ] ")
			}
		}
		return gast.WalkContinue, nil
	}

	return gast.WalkContinue, nil
}

func (r *mdRenderer) isInListItem(n gast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == gast.KindListItem {
			return true
		}
	}
	return false
}

func (r *mdRenderer) isInTightList(n gast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if list, ok := p.(*gast.List); ok {
			return list.IsTight
		}
	}
	return false
}
