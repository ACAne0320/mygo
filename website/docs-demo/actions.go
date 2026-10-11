package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func init() {
	saved, editing := 0, true
	demo("button", func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Cancel").Disabled(!editing).Clicked() {
				editing = false
			}
			if ui.PrimaryButton(c, "").Children(func() {
				ui.Icon(c, iconSave)
				ui.Text(c, "Save").SingleLine()
			}).Clicked() {
				saved++
				editing = true
			}
			if ui.Button(c, "").Label("Delete").Tooltip("Delete").Children(func() { ui.Icon(c, iconTrash) }).Clicked() {
				saved = 0
			}
		})
		switch {
		case !editing:
			status(c, "Canceled: Save again to edit")
		default:
			status(c, fmt.Sprintf("Saved %d times", saved))
		}
	})
}
