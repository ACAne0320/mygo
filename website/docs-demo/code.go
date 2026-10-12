package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const demoGreeter = `package greeter

// Greet greets name, or the world.
func Greet(name string) string {
	if name == "" {
		name = "world"
	}
	return "Hello, " + name + "!"
}
`

const demoGreeterEdited = `package greeter

import "strings"

// Greet greets name, or everyone.
func Greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "everyone"
	}
	return "Hello, " + name + "!"
}
`

func init() {
	golang := editor.LanguageFor("greeter.go")
	code := editor.NewBuffer(demoGreeter)
	code.SetLanguage(golang)
	st := &editor.State{Buffer: code, Label: "greeter.go", WrapGuides: []int{80}}
	demo("code-editor", func(c *ui.Context) {
		ui.Box(c).AlignSelf(ui.Stretch).Height(230).Border(1, c.Theme().Border).Radius(8).Clip().Children(func() {
			editor.View(c, st).Fill()
		})
		pos := st.CursorPos()
		carets := ""
		if n := len(st.Selections()); n > 1 {
			carets = fmt.Sprintf(" · %d carets", n)
		}
		status(c, fmt.Sprintf("Ln %d, Col %d%s · %d lines", pos.Line+1, pos.Col+1, carets, code.LineCount()))
	})

	before, after := editor.NewBuffer(demoGreeter), editor.NewBuffer(demoGreeterEdited)
	before.SetLanguage(golang)
	after.SetLanguage(golang)
	diff := &editor.DiffState{Old: before, New: after, Editable: true, HunkActions: true}
	demo("diff-viewer", func(c *ui.Context) {
		editor.DiffView(c, diff).AlignSelf(ui.Stretch).Height(250).Border(1, c.Theme().Border).Radius(8).Clip()
		added, deleted := diff.Stats()
		status(c, fmt.Sprintf("+%d −%d · edit the right side, or revert and keep its changes", added, deleted))
	})
}
