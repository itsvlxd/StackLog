package stacklog

// Padding describes the whitespace around a component. Left and Right are spaces
// written inside the line. Top and Bottom are lines, and they are not symmetric:
// Top is only honoured at the start of a line, and Bottom is never emitted
// mid-line — it is accumulated as the maximum across the line and flushed at the
// end by addBottomPadding.
type Padding struct {
	Top    int
	Left   int
	Bottom int
	Right  int
}

// CompStyle is a single component's colour and padding. Every method returns a
// copy, so a style is immutable and can be shared safely between loggers.
//
// The zero value means "no colour set": 0 is used as the sentinel in
// addColorStyle, so black cannot be expressed through these fields.
type CompStyle struct {
	p     Padding
	bg    Color
	bgRGB ColorRGB
	fg    Color
	fgRGB ColorRGB
}

// NewStyle returns an empty style. Colours and padding are set through the
// builder methods, each of which returns a modified copy.
func NewStyle() CompStyle {
	var cs CompStyle

	return cs
}

// Padding sets the whitespace for this style and returns the modified copy.
func (cs CompStyle) Padding(p Padding) CompStyle {
	cs.p = p

	return cs
}

// Foreground sets the foreground colour and returns the modified copy. val is
// accepted as a numeric colour index, a Color, a ColorRGB, or a hex string.
func (cs CompStyle) Foreground(val any) CompStyle {
	parseColorVal(val, &cs.fg, &cs.fgRGB)

	return cs
}

// Background sets the background colour and returns the modified copy. Same
// accepted forms as Foreground.
func (cs CompStyle) Background(val any) CompStyle {
	parseColorVal(val, &cs.bg, &cs.bgRGB)

	return cs
}

// parseColorVal converts an accepted colour form into a colour index or an RGB
// triple.
//
// Two behaviours worth knowing:
//
//   - The result depends on the Go type the caller happens to use. Foreground(5)
//     produces 5, Foreground(Color(5)) produces 6, because the +1 shift only
//     exists on the Color branch. That is a known inconsistency.
//   - There is no default branch, so an unhandled type leaves the colour
//     unchanged. A hex string that is not valid falls back to white through
//     HexToRGB, which cannot report failure.
func parseColorVal(val any, color *Color, rgb *ColorRGB) {
	switch c := val.(type) {
	case int:
		*color = Color(c)
	case int32:
		*color = Color(c)
	case int64:
		*color = Color(c)
	case uint:
		*color = Color(c)
	case uint32:
		*color = Color(c)
	case uint64:
		*color = Color(c)
	case Color:
		// +1 shifts the value space so that 0 can be reserved as "not set".
		// The cost is that Color(0) becomes unreachable.
		*color = c + 1
	case ColorRGB:
		*rgb = c
	case string:
		if c == "reset" {
			*color = 0
		} else {
			*rgb = HexToRGB(c)
		}
	}
}

// addColorStyle writes the escape sequences for a style into the buffer. A zero
// colour is treated as "not set" and emits nothing, which is why the sentinel and
// the value collide.
func addColorStyle(buf *StackBuffer, style CompStyle) {
	if style.bgRGB != (ColorRGB{}) {
		buf.BgColor(style.bgRGB)
	} else if style.bg > 0 {
		buf.BgColor(style.bg)
	}

	if style.fgRGB != (ColorRGB{}) {
		buf.FgColor(style.fgRGB)
	} else if style.fg > 0 {
		buf.FgColor(style.fg)
	}
}

// addPreStyle writes the opening half of a component: Top padding, colour, then
// Left padding. Top is only emitted when this component is the first one on the
// line; after that, lineStart is cleared so a mid-line Top cannot break the line.
func (sl *StackLog) addPreStyle(style CompStyle) {
	buf := sl.Buffer
	sp := style.p

	if buf.lineStart && sp.Top > 0 {
		for i := 0; i < sp.Top; i++ {
			buf.WriteNL()
		}
	}
	buf.lineStart = false

	addColorStyle(buf, style)

	if sp.Left > 0 {
		for i := 0; i < sp.Left; i++ {
			buf.WriteBlank()
		}
	}
}

// addPostStyle writes the closing half of a component: the colour is re-emitted,
// Right padding is written, and the Bottom value is recorded as the maximum
// across the line rather than emitted here. ResetColor runs last so the padding
// inside the line stays inside the coloured block.
//
// The colour is re-emitted even when it is already active. That is wasted bytes,
// not a correctness problem, and it is asserted in TestRenderLine so any change
// to it stays visible.
func (sl *StackLog) addPostStyle(style CompStyle) {
	buf := sl.Buffer
	sp := style.p

	addColorStyle(buf, style)

	if sp.Right > 0 {
		for i := 0; i < sp.Right; i++ {
			buf.WriteBlank()
		}
	}

	if sp.Bottom > buf.maxBottom {
		buf.maxBottom = sp.Bottom
	}

	buf.ResetColor()
}
