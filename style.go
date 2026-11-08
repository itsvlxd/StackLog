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
// The set flags carry "a colour was given" separately from the colour itself, so
// black is an expressible colour rather than the sentinel. The zero value still
// means "nothing set", which is what the preset guard in CreateLogger relies on.
type CompStyle struct {
	p     Padding
	bg    Color
	bgRGB ColorRGB
	bgSet bool
	fg    Color
	fgRGB ColorRGB
	fgSet bool
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
	parseColorVal(val, &cs.fg, &cs.fgRGB, &cs.fgSet)

	return cs
}

// Background sets the background colour and returns the modified copy. Same
// accepted forms as Foreground.
func (cs CompStyle) Background(val any) CompStyle {
	parseColorVal(val, &cs.bg, &cs.bgRGB, &cs.bgSet)

	return cs
}

// parseColorVal converts an accepted colour form into a colour index or an RGB
// triple, and records that a colour was given.
//
// Two behaviours worth knowing:
//
//   - There is no default branch, so an unhandled type leaves the colour
//     unchanged and the flag unset. A hex string that is not valid falls back to
//     white through HexToRGB, which cannot report failure.
//   - "reset" clears the flag rather than setting a colour, so Foreground(ColorReset)
//     means "nothing set". Black is Foreground(0), which is now reachable.
//
// clampColor brings a value into the range Color can hold. A value above 255 has
// no colour meaning, so the closest valid answer is the top of the range. A
// negative value is not a colour index, so it goes to the bottom. This replaces
// the silent wrap that Color(c) performed.
func clampColor(v int64) Color {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return Color(v)
}

func parseColorVal(val any, color *Color, rgb *ColorRGB, set *bool) {
	switch c := val.(type) {
	case int:
		*color, *set = clampColor(int64(c)), true
	case int32:
		*color, *set = clampColor(int64(c)), true
	case int64:
		*color, *set = clampColor(c), true
	case uint:
		*color, *set = clampColor(int64(c)), true
	case uint32:
		*color, *set = clampColor(int64(c)), true
	case uint64:
		// int64(c) would wrap for values above 2^63, so this one is checked
		// before the conversion.
		if c > 255 {
			*color, *set = 255, true
		} else {
			*color, *set = Color(c), true
		}
	case Color:
		*color, *set = c, true
	case ColorRGB:
		*rgb, *set = c, true
	case string:
		if c == "reset" {
			*color, *set = 0, false
		} else {
			*rgb, *set = HexToRGB(c), true
		}
	default:
		// An unhandled type leaves the colour unchanged and clears the flag, so
		// nothing is emitted. A colour setter has nowhere to return an error, and a
		// logger should not panic on a bad input, so leaving it unset is the only
		// safe behaviour available. Stated here so it is a contract rather than an
		// accident of an omitted case.
		*set = false
	}
}

// addColorStyle writes the escape sequences for a style into the buffer and
// reports whether anything was written. That report is what decides the reset in
// addPostStyle, so the gate is stated once here rather than at every call site.
func (sl *StackLog) addColorStyle(buf *StackBuffer, style CompStyle) bool {
	if sl.Config.Style.NoColors {
		return false
	}

	wrote := false

	if style.bgSet {
		if style.bgRGB != (ColorRGB{}) {
			buf.BgColor(style.bgRGB)
		} else {
			buf.BgColor(style.bg)
		}
		wrote = true
	}

	if style.fgSet {
		if style.fgRGB != (ColorRGB{}) {
			buf.FgColor(style.fgRGB)
		} else {
			buf.FgColor(style.fg)
		}
		wrote = true
	}

	return wrote
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

	sl.addColorStyle(buf, style)

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

	wrote := sl.addColorStyle(buf, style)

	if sp.Right > 0 {
		for i := 0; i < sp.Right; i++ {
			buf.WriteBlank()
		}
	}

	if sp.Bottom > buf.maxBottom {
		buf.maxBottom = sp.Bottom
	}

	// The reset only belongs to a component that actually opened a colour. Without
	// this, a style with nothing set still writes an escape sequence.
	if wrote {
		buf.ResetColor()
	}
}
