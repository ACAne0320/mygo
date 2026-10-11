# Transforms

`Translate`, `Scale`, and `Rotate` change an element and everything inside
it visually, while layout reserves its original space. They work on boxes,
text, images, controls, and custom drawings. The pointer follows what is
drawn, including nested transforms and clips:

```go
ui.Column(c).Size(180, 110).Padding(12).Gap(8).
	Scale(1.1, 0.9).Rotate(12).Translate(40, 0).
	Children(func() {
		ui.Text(c, "Transformed controls")
		ui.TextInput(c, &app.name)
	})
```

Transforms in a method chain act in call order: this example scales, then
rotates, then translates. A parent's transform acts after its children's.
Each scale and rotation uses the center of the element's border box by
default. `TransformOrigin(0, 0)` uses its top-left; `(1, 1)` its bottom-right.
Other fractions, including values outside the box, are allowed. Negative
scales reflect content; a zero scale makes the element invisible and gives
it no pointer hit region. Transform values must be finite.

For reusable compositions, build a `Transform` and set it with `Transform`:

```go
pose := ui.Scaling(1.2, 1.2).Then(ui.Rotation(15)).Then(ui.Translation(30, 0))
ui.Box(c).Transform(pose).TransformOrigin(0, 0)
```

The zero `Transform` is identity. Setting `Transform` replaces the element's
previous transform; the convenience methods append to it. Set transforms
in the view, before the frame is laid out. `Rotate` on an icon keeps its
existing behavior of rotating the SVG artwork around its center. To rotate
an icon's whole border box, use `Transform(ui.Rotation(degrees))`.

## Coordinates, clips, and overlays

`Bounds()` is the previous frame's transformed bounding rectangle in
window DIPs. `LayoutBounds()` is its box before transforms. `LocalToWindow`
and `WindowToLocal` convert points relative to that box; `WindowToLocal`
reports false when the transform has no inverse. `PointerPosition`,
`Dragged`, and `HandleInput` report local positions and vectors. Captured
pointer input stays local even outside the transformed box.

`Draw` and `DrawOver` receive the original layout coordinates, as before:
the painter applies the transform to everything they draw, including paths,
text, images, shadows, and effects. Clips retain the coordinate system in
which they were pushed and compose through nested elements. IME candidate
windows and accessibility receive the transformed bounding rectangles of
carets and elements. Popovers stay upright in the overlay and attach to
the transformed bounds of their anchors, including during layout transitions.

## Animating transforms

Transforms take animated values as well as fixed ones: a spring that keeps
its velocity, a curve, keyframes, or a value following the pointer (see
[Motion](motion.md)):

```go
tile := ui.Box(c).Key("tile").Size(80, 50)
target := float32(0)
if app.expanded {
	target = 120
}
tile.Translate(tile.AnimateTo("x", target, ui.Snappy), 0)
```

[Transitions](transitions.md) scale and rotate elements coming and going,
and move an element appearing from where another showed (`Match`), with
transforms too.

## Rendering

Metal, Direct3D 11, and OpenGL apply general transforms to their instanced
geometry on the GPU. Fills, borders, shadows, images, text, and effects keep
local coordinates; backdrop reads map back into window coordinates. Metal
preserves wide-gamut colors under transforms.

Nested rounded or transformed clips use retained coverage masks. Moving
content under a stationary clip reuses that mask without CPU pixel work.
Hard clips sharing their content's coordinates, including transformed text
inputs, are evaluated directly in the fragment shader and reuse their
ancestor's mask. Changing a complex clip recomputes its coverage; resizing
it replaces its texture in place rather than accumulating cached images.

The CPU compositor remains the renderer for machines without a GPU and for
small resting updates. Active transformed motion uses the GPU where one is
available. Text bitmaps are sampled bilinearly when transformed, and
transformed text uses grayscale coverage. GPU antialiasing follows the CPU
renderer in device-pixel coordinates.

The gallery's **Motion** page shows nested transformed editable controls,
an anchored popover, springs retargeted and thrown, keyframe sequences, and
shared elements.
