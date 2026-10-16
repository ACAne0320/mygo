package ui

import (
	"testing"
	"time"
)

// layerSpy is an effect that draws nothing, and notes the bitmaps it
// draws from.
type layerSpy struct {
	ids   map[uint64]bool
	sizes [][2]int
	parts [][2]int
}

func (s *layerSpy) EffectMargin() Point { return Point{X: 3, Y: 3} }

func (s *layerSpy) PaintEffect(p *Painter, box Rect, content *Bitmap, inset Point) {
	s.ids[content.img.ID()] = true
	s.sizes = append(s.sizes, [2]int{content.img.W, content.img.H})
	w, h := content.Size()
	s.parts = append(s.parts, [2]int{w, h})
}

// TestEffectLayerKept moves an element with an effect across pixels, as
// an animation takes it, growing it a little: it keeps the bitmap it
// renders into, its texture with it, rather than a bitmap a size, and the
// effect reads as much of it as the element and the margin take.
func TestEffectLayerKept(t *testing.T) {
	fx := &layerSpy{ids: map[uint64]bool{}}
	moved := false
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Width(400).Children(func() {
			w := float32(200)
			if moved {
				coreBox(c).Height(37)
				w = 230
			}
			coreBox(c).Key("fx").Width(w).Height(30).Background(RGB(255, 0, 0)).
				Transition(ElementTransition{Duration: 400 * time.Millisecond, Ease: Linear}).Effect(fx)
		})
	}, 400, 200)
	tt.Frame()
	moved = true
	for range 30 {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	if len(fx.ids) != 1 {
		t.Errorf("%d bitmaps, want 1", len(fx.ids))
	}
	for i, s := range fx.sizes {
		if s[0]%effectLayerStep != 0 || s[1]%effectLayerStep != 0 {
			t.Fatalf("bitmap of %v, not in steps of %d", s, effectLayerStep)
		}
		if p := fx.parts[i]; p[0] > s[0] || p[1] > s[1] || p[0] < 200+6 || p[1] < 30+6 {
			t.Fatalf("part of %v of a bitmap of %v", p, s)
		}
	}
	if last := fx.parts[len(fx.parts)-1]; last != [2]int{230 + 6, 30 + 6} {
		t.Errorf("last part %v, want the box and its margin", last)
	}
}

// TestEffectLayerTransient: the bitmap of an element's effect is drawn
// anew whenever it shows, so renderers may free its texture once frames
// stop drawing it.
func TestEffectLayerTransient(t *testing.T) {
	var st *state
	tt := coreNewTester(func(c *context) {
		e := coreBox(c).Width(50).Height(20).Background(RGB(0, 0, 255)).Effect(&layerSpy{ids: map[uint64]bool{}})
		st = e.st
	}, 100, 100)
	tt.Frame()
	if st.effectLayer == nil || !st.effectLayer.img.Transient {
		t.Errorf("layer %v, not transient", st.effectLayer)
	}
}

// imageEffect draws its content as an image, through ImageRegion or
// Image, over the box and its margin.
type imageEffect struct{ region bool }

func (imageEffect) EffectMargin() Point { return Point{X: 3, Y: 3} }

func (fx imageEffect) PaintEffect(p *Painter, box Rect, content *Bitmap, inset Point) {
	s := p.Scale()
	w, h := content.Size()
	area := Rect{X: box.X - inset.X/s, Y: box.Y - inset.Y/s, W: float32(w) / s, H: float32(h) / s}
	if fx.region {
		p.ImageRegion(content, Rect{}, area, ImageOptions{})
	} else {
		p.Image(content, area, FillBox)
	}
}

// TestEffectDrawsItsPart: an effect drawing its content as an image draws
// the content's part of the layer, at its size, rather than the whole
// layer squeezed into it.
func TestEffectDrawsItsPart(t *testing.T) {
	for _, region := range []bool{true, false} {
		img := coreNewTester(func(c *context) {
			coreBox(c).Fill().Background(RGB(255, 255, 255)).Padding(10).Children(func() {
				coreBox(c).Width(70).Height(20).Background(RGB(255, 0, 0)).Effect(imageEffect{region})
			})
		}, 200, 100).Image()
		for _, x := range []int{12, 45, 77} { // the box spans 10 to 80
			if p := img.RGBAAt(x, 20); p.R != 255 || p.G > 10 {
				t.Errorf("region %v: at x %d %v, want red", region, x, p)
			}
		}
		if p := img.RGBAAt(85, 20); p.G < 245 {
			t.Errorf("region %v: beyond the box %v, want white", region, p)
		}
	}
}
