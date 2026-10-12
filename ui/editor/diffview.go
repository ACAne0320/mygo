package editor

import (
	"github.com/egoist/mygo/ui"
)

// DiffState is a diff viewer's state, kept by the app across
// frames: the texts it compares, its options, and what it keeps of its
// own, as the lines expanded and its scroll position. Zero options are
// the defaults, those of Pierre's diffs.
type DiffState struct {
	// Old and New are the texts compared; empty buffers are made when
	// nil. New is the one edited when Editable, and can be the buffer of
	// an editor's too: an edit in either shows in both.
	Old, New *Buffer
	// Layout shows the texts side by side, or one over the other.
	Layout DiffLayout
	// Editable lets the new text be edited where it shows, with all an
	// editor does; the diff follows as it is typed.
	Editable bool
	// Inline highlights what changed within changed lines.
	Inline DiffInline
	// Indicators mark changed lines: with bars, + and -, or nothing.
	Indicators DiffIndicators
	// ContextLines is how many unchanged lines show around changes, 3
	// when 0, none when negative; the others collapse into separators
	// that expand them, ExpansionLines (100 when 0) a click, all of them
	// with Shift. ExpandUnchanged shows them all.
	ContextLines    int
	ExpansionLines  int
	ExpandUnchanged bool
	// NoBackground leaves changed lines without their tint, and
	// NoLineNumbers the gutter without numbers.
	NoBackground  bool
	NoLineNumbers bool
	// Wrap wraps long lines rather than scrolling them sideways.
	Wrap bool
	// FitContent makes the viewer as high as its lines, as a file of a
	// list of them, rather than scrolling them itself.
	FitContent bool
	// Font, LineHeight, TabSize and Theme are those of State.
	Font       ui.Font
	LineHeight float32
	TabSize    int
	Theme      *Theme
	// Bindings add key bindings to the editors of the sides.
	Bindings []Binding
	// HunkActions shows Revert and Keep on each change, which put the old
	// lines back in New, or make the new lines Old's, as an agent's
	// changes are reviewed.
	HunkActions bool
	// LineSelection lets the numbers of lines be clicked and dragged to
	// select lines, which OnLineSelection reports.
	LineSelection   bool
	OnLineSelection func(DiffLineRange)
	// OnGutterButton shows a + on the gutter of the line under the
	// pointer, as to comment on it, and is called with the line when it
	// is clicked.
	OnGutterButton func(DiffLineRange)
	// Annotations show what RenderAnnotation builds under lines, as
	// comments of a review, as high as each asks; the other side of a
	// split view leaves the room empty.
	Annotations      []DiffAnnotation
	RenderAnnotation func(c *ui.Context, a DiffAnnotation)

	model    diffModel
	eds      [2]editor // the old side's and the new side's
	sides    [2]diffSide
	oldView  view // the old text, for its lines in a unified view
	scroll   sharedScroll
	expanded map[int]diffExpansion
	expandV  int
	selected DiffLineRange
	hasSel   bool
	prepared diffPrepared
	gapList  []diffGap
	partial  *diffPartial
}

// DiffLayout is how a diff viewer lays out its two texts.
type DiffLayout uint8

const (
	// DiffSplit shows the old text on the left and the new on the right,
	// their lines aligned.
	DiffSplit DiffLayout = iota
	// DiffUnified shows the new text, with the lines deleted from the
	// old above where they were.
	DiffUnified
)

// DiffInline is how a diff viewer highlights what changed within lines.
type DiffInline uint8

const (
	// DiffInlineWordAlt highlights words, joining those one character
	// apart, as "a b c" changed reads as one highlight.
	DiffInlineWordAlt DiffInline = iota
	// DiffInlineWord highlights each word changed.
	DiffInlineWord
	// DiffInlineWordLine highlights from the first word changed to the
	// last.
	DiffInlineWordLine
	// DiffInlineChar highlights characters.
	DiffInlineChar
	// DiffInlineNone highlights nothing within lines.
	DiffInlineNone
)

// DiffIndicators is how a diff viewer marks changed lines.
type DiffIndicators uint8

const (
	// DiffBars draws a bar at the gutter's edge, striped for deletions.
	DiffBars DiffIndicators = iota
	// DiffClassic shows + and - before the lines.
	DiffClassic
	// DiffNoIndicators marks lines by their background alone.
	DiffNoIndicators
)

// DiffSide is a side of a diff: the old text or the new.
type DiffSide uint8

const (
	DiffOld DiffSide = iota
	DiffNew
)

// DiffLineRange is lines Start to End, inclusive, from 0, of a side.
type DiffLineRange struct {
	Side       DiffSide
	Start, End int
}

// DiffAnnotation is room under a line of a side for what
// RenderAnnotation builds there, Height DIPs high. Key identifies it
// among the others.
type DiffAnnotation struct {
	Side   DiffSide
	Line   int
	Height float32
	Key    any
}

// DiffHunk is a change: the old lines OldStart to OldEnd, exclusive,
// replaced by the new lines NewStart to NewEnd.
type DiffHunk struct {
	OldStart, OldEnd, NewStart, NewEnd int
}

type diffExpansion struct{ top, bottom int }

func (d *DiffState) context() int {
	switch {
	case d.ContextLines < 0:
		return 0
	case d.ContextLines == 0:
		return 3
	}
	return d.ContextLines
}

func (d *DiffState) expansionLines() int {
	if d.ExpansionLines <= 0 {
		return 100
	}
	return d.ExpansionLines
}

func (d *DiffState) ensure() {
	if d.Old == nil {
		d.Old = NewBuffer("")
	}
	if d.New == nil {
		d.New = NewBuffer("")
	}
	if d.expanded == nil {
		d.expanded = map[int]diffExpansion{}
	}
	d.model.set(d.Old, d.New)
	d.model.update()
}

// Hunks returns the changes between the texts.
func (d *DiffState) Hunks() []DiffHunk {
	d.ensure()
	out := make([]DiffHunk, len(d.model.hunks))
	for i, h := range d.model.hunks {
		out[i] = DiffHunk{h.A0, h.A1, h.B0, h.B1}
	}
	return out
}

// Stats returns how many lines the changes add and delete.
func (d *DiffState) Stats() (added, deleted int) {
	d.ensure()
	return d.model.stats()
}

// Revert puts the old lines of hunk i back in New.
func (d *DiffState) Revert(i int) {
	d.ensure()
	d.model.revert(i)
}

// Accept makes the new lines of hunk i Old's, so it is no longer a
// change.
func (d *DiffState) Accept(i int) {
	d.ensure()
	d.model.accept(i)
}

// ExpandAll shows every unchanged line; CollapseAll collapses them again
// into separators.
func (d *DiffState) ExpandAll() {
	d.ensure()
	d.ExpandUnchanged = true
}

func (d *DiffState) CollapseAll() {
	d.ExpandUnchanged = false
	d.expanded = map[int]diffExpansion{}
	d.expandV++
}

// SelectedLines returns the lines selected by their numbers, and whether
// there are.
func (d *DiffState) SelectedLines() (DiffLineRange, bool) { return d.selected, d.hasSel }

// SetSelectedLines selects lines, as clicking their numbers does; ok
// false clears the selection.
func (d *DiffState) SetSelectedLines(r DiffLineRange, ok bool) {
	if r.End < r.Start {
		r.Start, r.End = r.End, r.Start
	}
	d.selected, d.hasSel = r, ok
}

// GoToHunk puts the new side's caret at the start of hunk i and scrolls
// it to the middle of the view.
func (d *DiffState) GoToHunk(i int) {
	d.ensure()
	if i < 0 || i >= len(d.model.hunks) {
		return
	}
	h := d.model.hunks[i]
	ed := d.newEditor()
	r := &ed.view.text
	off := r.LineStart(min(h.B0, r.LineCount()-1))
	ed.setSels([]sel{ed.newSel(off, off)}, revealCenter)
}

func (d *DiffState) newEditor() *editor { return &d.eds[1] }

// DiffView builds the diff viewer.
func DiffView(c *ui.Context, d *DiffState) ui.Element {
	d.ensure()
	t := c.Theme()
	ct := d.Theme.resolve(t)
	opts := func(old bool) options {
		o := options{
			readOnly: old || !d.Editable, font: d.Font, lineHeight: d.LineHeight, tabSize: d.TabSize,
			wrap: d.Wrap, lineNumbers: LineNumbersOn, whitespace: WhitespaceNone, autoClose: true,
			theme: d.Theme, bindings: d.Bindings, fitContent: d.FitContent, minDigits: 3,
		}
		if d.NoLineNumbers {
			o.lineNumbers = LineNumbersOff
		}
		o.label = "New text"
		if old {
			o.label = "Old text"
		}
		return o
	}
	d.sides[0] = diffSide{d: d, old: true, hide: d.sides[0].hide, blks: d.sides[0].blks}
	d.sides[1] = diffSide{d: d, unified: d.Layout == DiffUnified, hide: d.sides[1].hide, blks: d.sides[1].blks}
	if d.Layout == DiffUnified {
		d.oldView.theme = ct
		d.oldView.setBuffer(d.Old)
		d.oldView.sync(nil)
		o := opts(false)
		o.decor = &d.sides[1]
		if d.Indicators == DiffClassic {
			o.signs = true
		}
		root := ui.Box(c).Clip()
		root.Draw(func(p *ui.Painter, r ui.Rect) {
			ed := &d.eds[1]
			ed.layout(r.W, r.H, p.Scale())
			d.oldView.setStyle(ed.view.font, ed.o.lineHeight, p.Scale(), ed.tabSize())
			d.oldView.setWrap(ed.view.wrap, ed.view.wrapWidth)
			d.prepare(true)
			ed.layout(r.W, r.H, p.Scale())
		})
		root.Children(func() {
			ed := d.eds[1].build(c, d.New, o)
			ed.Fill()
			if d.FitContent {
				ed.FillWidth()
			}
		})
		if d.FitContent {
			root.Height(float32(d.eds[1].view.heights.total()))
		}
		return root
	}
	// Side by side: two editors sharing their scroll.
	d.scroll.eds = []*editor{&d.eds[0], &d.eds[1]}
	row := ui.Row(c).Clip()
	row.Draw(func(p *ui.Painter, r ui.Rect) {
		w := (r.W - 1) / 2
		for i := range d.eds {
			d.eds[i].layout(w, r.H, p.Scale())
		}
		d.prepare(true)
		for i := range d.eds {
			d.eds[i].layout(w, r.H, p.Scale())
		}
		if !d.FitContent {
			d.scroll.set(d.scroll.y)
			// The new side reveals its carets for both.
			ed := &d.eds[1]
			if ed.reveal != revealNone {
				ed.doReveal()
				d.scroll.set(ed.scrollY)
			}
			if ed0 := &d.eds[0]; ed0.reveal != revealNone {
				ed0.doReveal()
				d.scroll.set(ed0.scrollY)
			}
		}
	})
	row.Children(func() {
		for i, b := range []*Buffer{d.Old, d.New} {
			o := opts(i == 0)
			o.decor = &d.sides[i]
			o.sharedScroll = &d.scroll
			if d.FitContent {
				o.sharedScroll = nil
			}
			o.noScrollbar = i == 0
			if d.Indicators == DiffClassic {
				o.signs = true
			}
			ed := d.eds[i].build(c, b, o)
			ed.Grow(1).Basis(0).FillHeight()
			if i == 0 {
				ui.Box(c).Width(1).FillHeight().Background(ct.Background.Mix(t.Border, 0.6))
			}
		}
	})
	if d.FitContent {
		row.Height(float32(max(d.eds[0].view.heights.total(), d.eds[1].view.heights.total())))
	}
	return row
}

// expand reveals lines of the gap starting at old line a: at its top,
// below the change before it (dir 1), at its bottom (dir -1), or all.
func (d *DiffState) expand(a int, dir int, all bool) {
	e := d.expanded[a]
	n := d.expansionLines()
	switch {
	case all:
		e.top += 1 << 30
	case dir > 0:
		e.top += n
	default:
		e.bottom += n
	}
	d.expanded[a] = e
	d.expandV++
}

// selectLine selects a line by its number, or extends the selection to
// it from its nearer end.
func (d *DiffState) selectLine(side DiffSide, line int, extend bool) {
	switch {
	case extend && d.hasSel && d.selected.Side == side:
		if line < d.selected.Start {
			d.selected.Start = line
		} else {
			d.selected.End = line
		}
	case d.hasSel && d.selected.Side == side && d.selected.Start == line && d.selected.End == line:
		d.hasSel = false
	default:
		d.selected, d.hasSel = DiffLineRange{Side: side, Start: line, End: line}, true
	}
	if d.OnLineSelection != nil && d.hasSel {
		d.OnLineSelection(d.selected)
	}
}

// activeHunk returns the hunk under the pointer on either side, or else
// the one holding the new side's newest caret, or -1.
func (d *DiffState) activeHunk() int {
	for i := range d.eds {
		ed := &d.eds[i]
		if !ed.pointerIn || ed.hoverLine < 0 {
			continue
		}
		side := &d.sides[i]
		if i == 1 && side.unified {
			if j := d.model.hunkAtNew(ed.hoverLine); j >= 0 {
				return j
			}
			continue
		}
		if j := side.hunkOf(ed.hoverLine); j >= 0 {
			return j
		}
		if i == 1 {
			if j := d.model.hunkAtNew(ed.hoverLine); j >= 0 {
				return j
			}
		}
	}
	ed := &d.eds[1]
	if ed.focused && len(ed.sels) > 0 {
		line := ed.view.text.LineOf(ed.primary().head)
		return d.model.hunkAtNew(line)
	}
	return -1
}

// isDark reports whether a background is dark.
func isDark(c ui.Color) bool {
	l := 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
	return l < 128
}
