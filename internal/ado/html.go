package ado

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// spaceRuns matches the whitespace HTML collapses. Non-breaking spaces
// (&nbsp;) aren't included: they're how editors keep indentation and
// repeated spaces, so they survive as regular spaces.
var spaceRuns = regexp.MustCompile(`[ \t\n\r\f]+`)

const nbsp = "\u00a0"

// HTMLToText converts the HTML stored in rich text fields (descriptions,
// acceptance criteria, comments) to readable plain text.
func HTMLToText(s string) string {
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(html.UnescapeString(s))
	}

	var b strings.Builder
	newline := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
	}
	pre, skip := 0, 0
	// lists tracks open lists: 0 for <ul>, otherwise the next <ol> number.
	var lists []int

	z := xhtml.NewTokenizer(strings.NewReader(s))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		name, _ := z.TagName()
		tag := atom.Lookup(name)

		switch tt {
		case xhtml.TextToken:
			if skip > 0 {
				continue
			}
			text := string(z.Text())
			if pre == 0 {
				text = spaceRuns.ReplaceAllString(text, " ")
				if b.Len() == 0 || strings.HasSuffix(b.String(), "\n") {
					text = strings.TrimLeft(text, " ")
				}
			}
			b.WriteString(strings.ReplaceAll(text, nbsp, " "))

		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			switch tag {
			case atom.Script, atom.Style:
				if tt == xhtml.StartTagToken {
					skip++
				}
			case atom.Br:
				b.WriteByte('\n')
			case atom.Ul:
				newline()
				lists = append(lists, 0)
			case atom.Ol:
				newline()
				lists = append(lists, 1)
			case atom.Li:
				newline()
				depth := max(len(lists)-1, 0)
				b.WriteString(strings.Repeat("  ", depth))
				if n := len(lists); n > 0 && lists[n-1] > 0 {
					b.WriteString(strconv.Itoa(lists[n-1]) + ". ")
					lists[n-1]++
				} else {
					b.WriteString("• ")
				}
			case atom.Pre:
				newline()
				pre++
			case atom.Img:
				b.WriteString("[image]")
			case atom.Td, atom.Th:
				b.WriteString("  ")
			case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
				atom.Tr, atom.Table, atom.Blockquote, atom.Hr:
				newline()
			}

		case xhtml.EndTagToken:
			switch tag {
			case atom.Script, atom.Style:
				skip = max(0, skip-1)
			case atom.Pre:
				pre = max(0, pre-1)
				newline()
			case atom.Ul, atom.Ol:
				if len(lists) > 0 {
					lists = lists[:len(lists)-1]
				}
				newline()
			case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
				atom.Li, atom.Tr, atom.Table, atom.Blockquote:
				newline()
			}
		}
	}

	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// TextToHTML converts plain text to the simple HTML the Azure DevOps editor
// produces: one <div> per line, with &nbsp; keeping indentation and
// repeated spaces. HTMLToText(TextToHTML(s)) returns s, apart from trailing
// spaces and leading or trailing blank lines.
func TextToHTML(text string) string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n ")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString("<div><br></div>")
			continue
		}
		b.WriteString("<div>")
		b.WriteString(keepSpaces(html.EscapeString(line)))
		b.WriteString("</div>")
	}
	return b.String()
}

// keepSpaces turns spaces HTML would collapse (leading ones and runs of
// more than one) into &nbsp;.
func keepSpaces(line string) string {
	var b strings.Builder
	prevSpace := true // a space at the start of the line would collapse
	for _, r := range line {
		if r == ' ' && prevSpace {
			b.WriteString("&nbsp;")
			continue
		}
		prevSpace = r == ' '
		b.WriteRune(r)
	}
	return b.String()
}

// plainTags survive a round trip through HTMLToText and TextToHTML, as long
// as they have no attributes (like style).
var plainTags = map[atom.Atom]bool{atom.Div: true, atom.Br: true, atom.P: true}

// HasRichFormatting reports whether s uses HTML that would be lost by
// converting it to text and back: anything beyond plain lines of text.
func HasRichFormatting(s string) bool {
	z := xhtml.NewTokenizer(strings.NewReader(s))
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return false
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if !plainTags[atom.Lookup(name)] || hasAttr {
				return true
			}
		}
	}
}
