package stacklog

// Preset names. An unknown name resolves to PresetDefault.
const (
	PresetDefault      = "default"
	PresetPlain        = "plain"
	PresetBadge        = "badge"
	PresetHighContrast = "high-contrast"
)

// presets maps a name to a builder. Each returns a fresh Style, so a caller can
// take one and modify a component without touching a shared value.
var presets = map[string]func() Style{
	PresetDefault:      defaultPreset,
	PresetPlain:        plainPreset,
	PresetBadge:        badgePreset,
	PresetHighContrast: highContrastPreset,
}

// loadPreset fills the components left as the zero value from the named preset.
// A component the caller set is not touched, so a single override stands against
// a full preset.
func loadPreset(style *Style) {
	build := presets[style.Preset]
	if build == nil {
		build = presets[PresetDefault]
	}

	base := build()

	if style.Timestamp == (CompStyle{}) {
		style.Timestamp = base.Timestamp
	}

	if style.Prefix == (CompStyle{}) {
		style.Prefix = base.Prefix
	}

	for i := LogLevel(0); i < MaxLevel; i++ {
		if style.Level[i] == (CompStyle{}) {
			style.Level[i] = base.Level[i]
		}

		if style.Params[i] == (ParamsStyle{}) {
			style.Params[i] = base.Params[i]
		}
	}
}

// defaultPreset is the package palette: grey timestamp and prefix, coloured level
// text, grey keys, white values.
func defaultPreset() Style {
	s := Style{
		Timestamp: NewStyle().Foreground(242),
		Prefix:    grayFGStyle,
	}

	for i := LogLevel(0); i < MaxLevel; i++ {
		s.Level[i] = NewStyle().Padding(levelPadding[i]).Foreground(levelFgColor[i])
		s.Params[i] = ParamsStyle{
			Param: NewStyle().Foreground(levelFgColor[i]),
			Equal: grayFGStyle,
			Value: whiteFGStyle,
		}
	}

	return s
}

// plainPreset sets nothing. With no flag set, addColorStyle emits nothing and
// addPostStyle does not write a reset, so the line is plain text.
func plainPreset() Style {
	return Style{}
}

// badgePreset renders the level as a block: the level colour becomes the
// background, the text is white, and the block carries left and right padding.
func badgePreset() Style {
	s := Style{
		Timestamp: NewStyle().Foreground(242),
		Prefix:    grayFGStyle,
	}

	for i := LogLevel(0); i < MaxLevel; i++ {
		s.Level[i] = NewStyle().
			Background(levelFgColor[i]).
			Foreground(15).
			Padding(Padding{Left: 1, Right: 1})

		s.Params[i] = ParamsStyle{
			Param: NewStyle().Foreground(levelFgColor[i]),
			Equal: grayFGStyle,
			Value: whiteFGStyle,
		}
	}

	return s
}

// highContrastPreset uses indices 17 and above only. Indices 9 to 16 are broken
// in writeColor until the ANSI16 card is fixed, so a bright palette cannot use
// them yet.
func highContrastPreset() Style {
	bright := [MaxLevel]Color{244, 39, 214, 196, 202}

	s := Style{
		Timestamp: NewStyle().Foreground(250),
		Prefix:    NewStyle().Foreground(250),
	}

	for i := LogLevel(0); i < MaxLevel; i++ {
		s.Level[i] = NewStyle().Foreground(bright[i])
		s.Params[i] = ParamsStyle{
			Param: NewStyle().Foreground(bright[i]),
			Equal: NewStyle().Foreground(245),
			Value: NewStyle().Foreground(255),
		}
	}

	return s
}
