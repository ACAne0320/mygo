//go:build linux && !android && (amd64 || arm64)

package linux

import "testing"

// TestOnKDE tells KDE Plasma, whose KWin draws title bars, from the tiling
// compositors that also decorate windows themselves.
func TestOnKDE(t *testing.T) {
	for desktop, want := range map[string]bool{
		"KDE":           true,
		"kde":           true,
		"ubuntu:KDE":    true,
		"GNOME":         false,
		"sway":          false,
		"Hyprland":      false,
		"":              false,
		"KDE-something": false,
	} {
		got := onKDE(func(k string) string {
			if k == "XDG_CURRENT_DESKTOP" {
				return desktop
			}
			return ""
		})
		if got != want {
			t.Errorf("XDG_CURRENT_DESKTOP=%q: KDE %v, want %v", desktop, got, want)
		}
	}
}
