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
//
// Two behaviours a caller can rely on:
//   - A numeric value outside 0-255 is clamped to the nearest valid end, not
//     wrapped. 300 becomes 255, -1 becomes 0.
//   - A type that is not one of the accepted forms leaves the style unchanged and
//     nothing is emitted. There is no error path, because a style builder has
//     nowhere to return one and a logger should not panic on a bad input.
func (cs CompStyle) Foreground(val any) CompStyle {
	parseColorVal(val, &cs.fg, &cs.fgRGB, &cs.fgSet)

	return cs
}

// Background sets the background colour and returns the modified copy. Same
// accepted forms and the same clamping as Foreground.
func (cs CompStyle) Background(val any) CompStyle {
	parseColorVal(val, &cs.bg, &cs.bgRGB, &cs.bgSet)

	return cs
}

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

// parseColorVal converts an accepted colour form into a colour index or an RGB
// triple and records that a colour was given. The caller-facing rules - clamping
// and unhandled types - are documented on Foreground and Background, where a
// developer meets them. What is specific to this function is stated here:
//   - "reset" clears the flag rather than setting a colour, so ColorReset means
//     "nothing set", not "reset to default". Black is 0.
//   - A hex string that is not valid falls back to white through HexToRGB, which
//     cannot report failure.
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

// hasColor reports whether this style opens a colour on this logger: a colour was
// set, and colour output is not switched off. The two questions are not the same
// and the difference is the whole gate — a style can carry colour fields that
// NoColors suppresses, and a style can be non-zero from padding alone while
// writing no colour at all. addColorStyle writes on this answer and addPostStyle
// resets on it, so the rule has one home.
// hasColor is the single gate both halves of a component go through. It reads the answer this logger
// settled when its writer was chosen - see resolveColor and colorpolicy.go - rather than consulting
// Config.Colors per line, and NoColors still outranks everything above it.
func (sl *StackLog) hasColor(style CompStyle) bool {
	return sl.emitColor && (style.fgSet || style.bgSet)
}

// addColorStyle writes the opening escape sequences for a style into the buffer.
// It returns nothing: whether a component gets a reset is decided by addPostStyle
// asking hasColor, not by a report from here.
func (sl *StackLog) addColorStyle(buf *StackBuffer, style CompStyle) {
	if !sl.hasColor(style) {
		return
	}

	if style.bgSet {
		if style.bgRGB != (ColorRGB{}) {
			buf.BgColor(style.bgRGB)
		} else {
			buf.BgColor(style.bg)
		}
	}

	if style.fgSet {
		if style.fgRGB != (ColorRGB{}) {
			buf.FgColor(style.fgRGB)
		} else {
			buf.FgColor(style.fg)
		}
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

	sl.addColorStyle(buf, style)

	if sp.Left > 0 {
		for i := 0; i < sp.Left; i++ {
			buf.WriteBlank()
		}
	}
}

// addPostStyle writes the closing half of a component: Right padding, the Bottom
// value recorded as the maximum across the line rather than emitted here, and the
// reset. ResetColor runs last so padding inside the line stays inside the coloured
// block.
//
// No colour is written here. addPreStyle opened it and nothing between the two
// closed it, so re-emitting it only repeats a state the terminal already holds.
// That repeat used to cost 4 to 11 bytes per component.
func (sl *StackLog) addPostStyle(style CompStyle) {
	buf := sl.Buffer
	sp := style.p

	if sp.Right > 0 {
		for i := 0; i < sp.Right; i++ {
			buf.WriteBlank()
		}
	}

	if sp.Bottom > buf.maxBottom {
		buf.maxBottom = sp.Bottom
	}

	// Only a component that opened a colour has a reset to write. Padding is not
	// colour, and NoColors suppresses a style that does carry one.
	if sl.hasColor(style) {
		buf.ResetColor()
	}
}
