package stacklog

import "testing"

// The +1 shift is gone, so the Go type the caller uses no longer changes the
// result. This is the test that would have caught the old inconsistency.
func TestParseColorValIsConsistent(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want Color
		set  bool
	}{
		{"int-branch", 5, 5, true},
		{"color-branch", Color(5), 5, true},
		{"black-is-a-colour", Color(0), 0, true},
		{"unhandled-type", 1.5, 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var col Color
			var rgb ColorRGB
			var set bool
			parseColorVal(c.in, &col, &rgb, &set)
			if col != c.want || set != c.set {
				t.Errorf("got %d/%v, want %d/%v", col, set, c.want, c.set)
			}
		})
	}
}

// "reset" clears the flag rather than setting a colour, so Foreground(ColorReset)
// means nothing set and the preset guard fills the component.
func TestParseColorValReset(t *testing.T) {
	var c Color = 9
	var rgb ColorRGB
	var set bool = true
	parseColorVal(ColorReset, &c, &rgb, &set)
	if c != 0 || set {
		t.Errorf("got %d/%v, want 0/false", c, set)
	}
}

// CreateLogger applies the named preset, and a component the caller set is not
// touched by it. These tests assert that a single override stands against a full
// preset.

func TestCallerStyleSurvivesCreateLogger(t *testing.T) {
	c := Config{Style: Style{Level: [MaxLevel]CompStyle{NewStyle().Foreground("#D75F5F")}}}
	l := CreateLogger(c)

	if l.Config.Style.Level[0] != c.Style.Level[0] {
		t.Errorf("level 0 overwritten: got %v, want %v", l.Config.Style.Level[0], c.Style.Level[0])
	}

	if l.Config.Style.Level[1] == (CompStyle{}) {
		t.Error("level 1 lost the preset")
	}
}

// Params is guarded per component, the same rule as Level. A partial style is
// kept as it was written, and the entries left at the zero value still get the
// package default.
func TestPartialParamsStyle(t *testing.T) {
	c := Config{Style: Style{Params: [MaxLevel]ParamsStyle{
		{}, {}, {},
		{Param: NewStyle().Foreground("#5F87D7"), Equal: grayFGStyle, Value: whiteFGStyle},
		{},
	}}}
	l := CreateLogger(c)

	if l.Config.Style.Params[3] != c.Style.Params[3] {
		t.Error("params 3 overwritten")
	}

	for i := LogLevel(0); i < MaxLevel; i++ {
		if i == 3 {
			continue
		}
		if l.Config.Style.Params[i] == (ParamsStyle{}) {
			t.Errorf("params %d lost the preset", i)
		}
	}
}

// Black is now a colour rather than the sentinel, so an explicit black is
// expressible. Foreground(ColorReset) still means nothing set, so the guard fills
// that component from the preset.
func TestBlackIsExpressibleInForeground(t *testing.T) {
	c := Config{Style: Style{Level: [MaxLevel]CompStyle{NewStyle().Foreground(Color(0))}}}
	l := CreateLogger(c)

	if !l.Config.Style.Level[0].fgSet || l.Config.Style.Level[0].fg != 0 {
		t.Error("black was not kept as a colour")
	}
}

func TestExplicitNoColourIsFilledByThePreset(t *testing.T) {
	c := Config{Style: Style{Level: [MaxLevel]CompStyle{NewStyle().Foreground(ColorReset)}}}
	l := CreateLogger(c)

	if l.Config.Style.Level[0] == (CompStyle{}) {
		t.Error("the preset did not fill a component left unset by ColorReset")
	}
}
