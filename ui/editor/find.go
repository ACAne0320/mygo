package editor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// finder is an editor's find bar and its matches, as Zed's buffer
// search: a query, case sensitive or not, whole words, a regular
// expression, in the selections or everywhere, and its replacement.
type finder struct {
	open        bool
	replacing   bool
	query       string
	replace     string
	caseSens    bool
	wholeWord   bool
	regex       bool
	inSelection bool
	scope       []sel
	focusQuery  bool
	// The matches of the last search, and the text and options it was of.
	matches  []diffRange
	current  int
	stale    bool
	version  uint64
	searched string
	options  [4]bool
	err      error
	lower    string // the text lowered, for searching without case
	lowerVer uint64
	scrollTo bool
}

// maxMatches bounds the matches a search finds.
const maxMatches = 100000

func (f *finder) show(ed *editor, replace bool) {
	f.open = true
	f.replacing = f.replacing || replace
	if replace {
		f.replacing = true
	}
	// The selection, or the word at the caret, seeds the query.
	p := ed.primary()
	if !p.empty() && ed.view.text.LineOf(p.start()) == ed.view.text.LineOf(p.end()) {
		f.query = ed.slice(p.start(), p.end())
	} else if p.empty() {
		if a, z := wordAt(ed.view.text, p.head, ed.wordChars()); z > a && f.query == "" {
			f.query = ed.slice(a, z)
		}
	}
	if f.inSelection {
		f.scope = nil
	}
	f.focusQuery = true
	f.stale = true
	f.scrollTo = true
	ed.needsBuild = true
}

// search finds the matches of the query in the text again when either
// changed.
func (f *finder) search(ed *editor) {
	opts := [4]bool{f.caseSens, f.wholeWord, f.regex, f.inSelection}
	if !f.stale && f.version == ed.view.version && f.searched == f.query && f.options == opts {
		return
	}
	f.stale = false
	f.version, f.searched, f.options = ed.view.version, f.query, opts
	f.matches = f.matches[:0]
	f.err = nil
	if f.query == "" {
		return
	}
	r := &ed.view.text
	if f.inSelection && f.scope == nil {
		for _, s := range ed.sels {
			if !s.empty() {
				f.scope = append(f.scope, s)
			}
		}
	}
	text := r.String()
	extra := ed.wordChars()
	add := func(a, b int) bool {
		if f.wholeWord && !isWholeWord(text, a, b, extra) {
			return true
		}
		if f.inSelection && len(f.scope) > 0 {
			in := false
			for _, s := range f.scope {
				if a >= s.start() && b <= s.end() {
					in = true
					break
				}
			}
			if !in {
				return true
			}
		}
		f.matches = append(f.matches, diffRange{a, b})
		return len(f.matches) < maxMatches
	}
	switch {
	case f.regex:
		pat := f.query
		if !f.caseSens {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile("(?m)" + pat)
		if err != nil {
			f.err = err
			return
		}
		for _, m := range re.FindAllStringIndex(text, maxMatches) {
			if m[1] == m[0] {
				continue
			}
			if !add(m[0], m[1]) {
				break
			}
		}
	case f.caseSens:
		for at := 0; ; {
			i := strings.Index(text[at:], f.query)
			if i < 0 || !add(at+i, at+i+len(f.query)) {
				break
			}
			at += i + len(f.query)
		}
	case isASCII(f.query):
		if f.lowerVer != ed.view.version || f.lower == "" && text != "" {
			f.lower, f.lowerVer = asciiLower(text), ed.view.version
		}
		q := asciiLower(f.query)
		for at := 0; ; {
			i := strings.Index(f.lower[at:], q)
			if i < 0 || !add(at+i, at+i+len(q)) {
				break
			}
			at += i + len(q)
		}
	default:
		re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(f.query))
		for _, m := range re.FindAllStringIndex(text, maxMatches) {
			if !add(m[0], m[1]) {
				break
			}
		}
	}
	f.current = min(f.current, max(0, len(f.matches)-1))
}

// asciiLower lowers the ASCII letters of s, keeping its length.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// nearest returns the match at or after off, round to the first.
func (f *finder) nearest(off int) int {
	i := sort.Search(len(f.matches), func(i int) bool { return f.matches[i].start >= off })
	if i == len(f.matches) {
		return 0
	}
	return i
}

// step selects the next match (or previous) after the newest caret.
func (f *finder) step(ed *editor, dir int) bool {
	f.search(ed)
	if len(f.matches) == 0 {
		return false
	}
	p := ed.primary()
	var i int
	if dir > 0 {
		i = f.nearest(p.end())
		if f.matches[i].start == p.start() && f.matches[i].end == p.end() && i+1 <= len(f.matches) {
			i = (i + 1) % len(f.matches)
		}
	} else {
		i = f.nearest(p.start()) - 1
		if i < 0 {
			i = len(f.matches) - 1
		}
	}
	f.current = i
	m := f.matches[i]
	ed.setSels([]sel{ed.newSel(m.start, m.end)}, revealCenter)
	return true
}

// replaceOne replaces the current match when selected, and goes to the
// next.
func (f *finder) replaceOne(ed *editor) {
	f.search(ed)
	p := ed.primary()
	for i, m := range f.matches {
		if m.start == p.start() && m.end == p.end() {
			with := f.replacement(ed, m)
			ed.apply([]Edit{{Start: m.start, End: m.end, Text: with}}, editOther, func(move func(int, bias) int) []sel {
				at := move(m.start, biasLeft) + len(with)
				return []sel{ed.newSel(at, at)}
			})
			f.search(ed)
			f.current = i
			f.step(ed, 1)
			return
		}
	}
	f.step(ed, 1)
}

// replaceAll replaces every match, as one step of undo.
func (f *finder) replaceAll(ed *editor) {
	f.search(ed)
	if len(f.matches) == 0 {
		return
	}
	edits := make([]Edit, len(f.matches))
	for i, m := range f.matches {
		edits[i] = Edit{Start: m.start, End: m.end, Text: f.replacement(ed, m)}
	}
	ed.apply(edits, editOther, nil)
}

// replacement returns the replacement of a match: the replace text, with
// $1 and ${name} expanded for a regular expression.
func (f *finder) replacement(ed *editor, m diffRange) string {
	if !f.regex {
		return f.replace
	}
	pat := f.query
	if !f.caseSens {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile("(?m)" + pat)
	if err != nil {
		return f.replace
	}
	src := ed.slice(m.start, m.end)
	sub := re.FindStringSubmatchIndex(src)
	if sub == nil {
		return f.replace
	}
	return string(re.ExpandString(nil, f.replace, src, sub))
}

// status is what the bar says of the matches.
func (f *finder) status() string {
	switch {
	case f.err != nil:
		return "Invalid"
	case f.query == "":
		return ""
	case len(f.matches) == 0:
		return "No results"
	case len(f.matches) >= maxMatches:
		return fmt.Sprintf("%d of %d+", f.current+1, len(f.matches))
	}
	return fmt.Sprintf("%d of %d", f.current+1, len(f.matches))
}

// build builds the find bar over the editor's top right corner.
func (f *finder) build(c *ui.Context, ed *editor) {
	t := c.Theme()
	f.search(ed)
	if f.scrollTo && len(f.matches) > 0 {
		// The match at or after the caret is selected as the query is
		// typed, as Zed's search does.
		f.scrollTo = false
		f.current = f.nearest(ed.primary().start())
		m := f.matches[f.current]
		ed.setSels([]sel{ed.newSel(m.start, m.end)}, revealNone)
		if cr, ok := ed.caretRect(m.start); ok && (float64(cr.Y) < ed.scrollY || float64(cr.Y) > ed.scrollY+float64(ed.viewH)-float64(ed.view.lineHeight)) {
			ed.setScroll(float64(cr.Y) - float64(ed.viewH)/3)
		}
	}
	bar := ui.Column(c).Key("code find").Absolute().Top(t.Space(1)).Right(t.Space(4)).Gap(t.Space(1)).Padding(t.Space(1.5)).
		Background(t.Surface).Border(1, t.Border).Radius(t.Radius).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.18)).
		Role(ui.RoleToolbar).Label("Find").Cursor(ui.CursorDefault)
	bar.HandleInput(shield)
	closeBar := func() {
		f.open = false
		ed.refocus = true
	}
	field := func(value *string, placeholder string) ui.Element {
		return ui.TextInputBase(c, value).Width(t.Space(50)).Placeholder(placeholder).Padding(t.Space(1), t.Space(2)).
			Background(t.Background).Border(1, t.Border).Radius(t.Radius - 2).FontSize(t.FontSize - 1).Label(placeholder)
	}
	button := func(label, tip string) ui.Element {
		b := ui.ButtonBase(c).Padding(t.Space(0.5), t.Space(1.5)).Radius(t.Radius - 2).Label(tip).Tooltip(tip).KeepFocus()
		b.Children(func() { ui.Text(c, label).FontSize(t.FontSize - 1) })
		return b
	}
	bar.Children(func() {
		ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
			in := field(&f.query, "Find")
			if f.focusQuery {
				f.focusQuery = false
				in.Focus().SetTextSelection(0, utf8.RuneCountInString(f.query))
			}
			if in.Changed() {
				f.stale = true
				f.scrollTo = true
			}
			switch {
			case in.Submitted() && c.Modifiers()&ui.Shift != 0:
				f.step(ed, -1)
			case in.Submitted() && c.Modifiers()&ui.Alt != 0:
				f.selectAll(ed)
			case in.Submitted():
				f.step(ed, 1)
			case in.Shortcut(0, ui.KeyEscape):
				closeBar()
			}
			toggle := func(on *bool, label, tip string) {
				b := button(label, tip)
				if *on {
					b.Background(t.Accent.Alpha(0.2)).TextColor(t.Accent)
				}
				if b.Clicked() {
					*on = !*on
					f.stale = true
					if on == &f.inSelection {
						f.scope = nil
					}
				}
			}
			toggle(&f.caseSens, "Aa", "Match case")
			toggle(&f.wholeWord, "ab", "Match whole word")
			toggle(&f.regex, ".*", "Use regular expression")
			toggle(&f.inSelection, "[ ]", "Find in selection")
			ui.Text(c, f.status()).FontSize(t.FontSize - 2).TextColor(t.TextMuted).FontFeatures("tnum").MinWidth(t.Space(16)).Role(ui.RoleStatus)
			if button("↑", "Previous match").Clicked() {
				f.step(ed, -1)
			}
			if button("↓", "Next match").Clicked() {
				f.step(ed, 1)
			}
			if button("⇄", "Replace").Clicked() {
				f.replacing = !f.replacing
			}
			if button("✕", "Close").Clicked() {
				closeBar()
			}
		})
		if f.replacing && !ed.o.readOnly {
			ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				in := field(&f.replace, "Replace")
				switch {
				case in.Submitted() && c.Modifiers()&ui.Cmd != 0:
					f.replaceAll(ed)
				case in.Submitted():
					f.replaceOne(ed)
				case in.Shortcut(0, ui.KeyEscape):
					closeBar()
				}
				if ui.Button(c, "Replace").Padding(t.Space(0.5), t.Space(2)).FontSize(t.FontSize - 1).Clicked() {
					f.replaceOne(ed)
				}
				if ui.Button(c, "Replace All").Padding(t.Space(0.5), t.Space(2)).FontSize(t.FontSize - 1).Clicked() {
					f.replaceAll(ed)
				}
			})
		}
	})
	if isMac() {
		switch {
		case c.Shortcut(ui.Cmd, ui.KeyG):
			f.step(ed, 1)
		case c.Shortcut(ui.Shift|ui.Cmd, ui.KeyG):
			f.step(ed, -1)
		}
	} else {
		switch {
		case c.Shortcut(0, ui.KeyF3):
			f.step(ed, 1)
		case c.Shortcut(ui.Shift, ui.KeyF3):
			f.step(ed, -1)
		}
	}
}

// shield takes no input, but keeps the presses on what an editor shows
// over its text, as its find bar, from the editor around it: the input
// goes to the innermost element taking it as it comes, which then leaves
// it to the buttons and fields inside, as they take it.
func shield(ui.InputEvent) bool { return false }

// selectAll selects every match, as alt-enter in Zed's search bar.
func (f *finder) selectAll(ed *editor) {
	f.search(ed)
	if len(f.matches) == 0 {
		return
	}
	sels := make([]sel, 0, len(f.matches))
	for _, m := range f.matches {
		sels = append(sels, ed.newSel(m.start, m.end))
	}
	ed.setSels(sels, revealFit)
	f.open = false
	ed.refocus = true
}

// goToLine is the bar going to a line: line[:column], or +n and -n
// lines from the caret, as Zed's go to line, which shows the line as it
// is typed and goes back where it was on Escape.
type goToLine struct {
	open    bool
	text    string
	focus   bool
	savedY  float64
	savedX  float32
	preview int
}

func (g *goToLine) show(ed *editor) {
	g.open, g.text, g.focus = true, "", true
	g.savedY, g.savedX = ed.scrollY, ed.scrollX
	g.preview = -1
	ed.needsBuild = true
}

func (g *goToLine) close(ed *editor) {
	g.open = false
	ed.setScroll(g.savedY)
	ed.scrollX = g.savedX
	g.preview = -1
	ed.refocus = true
}

// target parses the text into a line and column, from 0.
func (g *goToLine) target(ed *editor) (int, int, bool) {
	s := strings.TrimSpace(g.text)
	if s == "" {
		return 0, 0, false
	}
	cur := ed.view.text.LineOf(ed.primary().head)
	rel := 0
	if s[0] == '+' || s[0] == '-' {
		rel = 1
		if s[0] == '-' {
			rel = -1
		}
		s = s[1:]
	}
	parts := strings.SplitN(s, ":", 2)
	line, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	col := 0
	if len(parts) == 2 {
		if c, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && c > 0 {
			col = c - 1
		}
	}
	if rel != 0 {
		line = cur + rel*line
	} else {
		line--
	}
	return max(0, min(line, ed.view.text.LineCount()-1)), col, true
}

func (g *goToLine) build(c *ui.Context, ed *editor) {
	t := c.Theme()
	bar := ui.Row(c).Key("code go to line").Absolute().Top(t.Space(1)).Left(0).Right(0).Justify(ui.Center).PassThrough()
	bar.Children(func() {
		panel := ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Padding(t.Space(1.5), t.Space(2)).Background(t.Surface).
			Border(1, t.Border).Radius(t.Radius).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.18))
		panel.HandleInput(shield)
		panel.Children(func() {
			in := ui.TextInputBase(c, &g.text).Width(t.Space(60)).Padding(t.Space(1), t.Space(2)).Background(t.Background).
				Border(1, t.Border).Radius(t.Radius - 2).Label("Go to line").Placeholder(fmt.Sprintf("Line 1 to %d, then :column", ed.view.text.LineCount()))
			if g.focus {
				g.focus = false
				in.Focus()
			}
			line, col, ok := g.target(ed)
			if ok && line != g.preview {
				g.preview = line
				off := offsetOf(ed.view.text, Pos{line, col})
				if cr, ok := ed.caretRect(off); ok {
					ed.setScroll(float64(cr.Y) - float64(ed.viewH-ed.view.lineHeight)/2)
				}
			}
			cur := posOf(ed.view.text, ed.primary().head)
			ui.Text(c, fmt.Sprintf("Current line %d of %d, column %d", cur.Line+1, ed.view.text.LineCount(), cur.Col+1)).
				FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			switch {
			case in.Submitted():
				if line, col, ok := g.target(ed); ok {
					off := offsetOf(ed.view.text, Pos{line, col})
					g.open = false
					ed.setSels([]sel{ed.newSel(off, off)}, revealCenter)
					ed.refocus = true
				}
			case in.Shortcut(0, ui.KeyEscape):
				g.close(ed)
			}
		})
	})
}
