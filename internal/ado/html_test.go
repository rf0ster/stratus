package ado

import "testing"

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain text", "just text &amp; more", "just text & more"},
		{"divs", "<div>one</div><div>two</div>", "one\ntwo"},
		{"blank line", "<div>one</div><div><br></div><div>two</div>", "one\n\ntwo"},
		{"paragraphs", "<p>one</p>\n  <p>two</p>", "one\ntwo"},
		{"br", "one<br>two<br/>three", "one\ntwo\nthree"},
		{"list", "<div>Steps:</div><ul><li>first</li><li>second</li></ul>", "Steps:\n• first\n• second"},
		{"ordered list", "<ol><li>one</li><li>two</li></ol>", "1. one\n2. two"},
		{"nested list", "<ul><li>a<ol><li>x</li><li>y</li></ol></li><li>b</li></ul>", "• a\n  1. x\n  2. y\n• b"},
		{"inline", "<div>a <b>bold</b> and <code>code</code></div>", "a bold and code"},
		{"entities", "<div>&lt;tag&gt; &amp; x</div>", "<tag> & x"},
		{"nbsp keeps spaces", "<div>&nbsp;&nbsp;indented &nbsp;twice</div>", "  indented  twice"},
		{"nbsp blank line", "<div>a</div><div>&nbsp;</div><div>b</div>", "a\n\nb"},
		{"whitespace", "<div>\n   lots   of\n  space  </div>", "lots of space"},
		{"script", "<div>a</div><script>alert(1)</script><div>b</div>", "a\nb"},
		{"pre", "<pre>  indented\n    more</pre>", "  indented\n    more"},
		{"blank lines kept", "<div>a</div><div><br></div><div><br></div><div>b</div>", "a\n\n\nb"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTMLToText(tt.in); got != tt.want {
				t.Errorf("HTMLToText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTextToHTMLRoundTrip(t *testing.T) {
	for _, text := range []string{
		"single line",
		"one\ntwo",
		"one\n\ntwo",
		"special <chars> & \"quotes\"",
		"• bullet\n• another",
		"Steps:\n  - indented item\n      code  with  spaces\n\n\nend",
		"  leading spaces",
	} {
		if got := HTMLToText(TextToHTML(text)); got != text {
			t.Errorf("round trip of %q = %q", text, got)
		}
	}
	if got := TextToHTML("  \n "); got != "" {
		t.Errorf("TextToHTML(blank) = %q, want empty", got)
	}
}

func TestSplitTags(t *testing.T) {
	got := JoinTags(SplitTags(" api;security , API;; ux "))
	if want := "api; security; ux"; got != want {
		t.Errorf("SplitTags = %q, want %q", got, want)
	}
}

func TestQueryWIQL(t *testing.T) {
	tests := []struct {
		name string
		q    Query
		want string
	}{
		{
			"everyone",
			Query{},
			"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project ORDER BY [Microsoft.VSTS.Common.Priority] ASC, [System.Id] ASC",
		},
		{
			"me, open, area",
			Query{AssignedTo: "@me", HideCompleted: true, AreaPath: `Proj\Team`},
			"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.AssignedTo] = @Me AND [System.State] NOT IN ('Closed', 'Done', 'Removed') AND [System.AreaPath] UNDER 'Proj\\Team' ORDER BY [Microsoft.VSTS.Common.Priority] ASC, [System.Id] ASC",
		},
		{
			"quoted name",
			Query{AssignedTo: "Pat O'Brien", IterationPath: `Proj\Sprint 1`},
			"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.AssignedTo] = 'Pat O''Brien' AND [System.IterationPath] UNDER 'Proj\\Sprint 1' ORDER BY [Microsoft.VSTS.Common.Priority] ASC, [System.Id] ASC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.WIQL(); got != tt.want {
				t.Errorf("WIQL() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestHasRichFormatting(t *testing.T) {
	for in, want := range map[string]bool{
		"":                                    false,
		"plain text":                          false,
		TextToHTML("one\n\ntwo <three>"):      false,
		"<p>para</p><div>x</div>":             false,
		"<span>x</span>":                      true,
		`<div style="color:red">x</div>`:      true,
		"<div>a <b>bold</b></div>":            true,
		"<ul><li>item</li></ul>":              true,
		`<div><img src="x.png"></div>`:        true,
		`<a href="https://example.com">x</a>`: true,
	} {
		if got := HasRichFormatting(in); got != want {
			t.Errorf("HasRichFormatting(%q) = %v, want %v", in, got, want)
		}
	}
}
