package editor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	typed       string // the query as the matches were last chosen for
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

// update searches again when the text, the query or the options
// changed, and selects the match at or after the caret as the query is
// typed, as Zed's search does.
func (f *finder) update(ed *editor) {
	if f.query != f.typed {
		f.typed = f.query
		f.scrollTo = true
	}
	f.search(ed)
	if !f.scrollTo || len(f.matches) == 0 {
		return
	}
	f.scrollTo = false
	f.current = f.nearest(ed.primary().start())
	m := f.matches[f.current]
	ed.setSels([]sel{ed.newSel(m.start, m.end)}, revealNone)
	if cr, ok := ed.caretRect(m.start); ok && (float64(cr.Y) < ed.scrollY || float64(cr.Y) > ed.scrollY+float64(ed.viewH)-float64(ed.view.lineHeight)) {
		ed.setScroll(float64(cr.Y) - float64(ed.viewH)/3)
	}
}

// search finds the matches of the query in the text again when either
// changed.
func (f *finder) search(ed *editor) {
	opts := [4]bool{f.caseSens, f.wholeWord, f.regex, f.inSelection}
	if !f.stale && f.version == ed.view.version && f.searched == f.query && f.options == opts {
		return
	}
	if opts[3] != f.options[3] {
		// Finding in the selection takes the selections there are now.
		f.scope = nil
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
