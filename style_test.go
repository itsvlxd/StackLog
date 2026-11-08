package stacklog

import "testing"

// The same conceptual colour, two different results, depending on the Go type
// the caller happens to use. This is the real finding, not the +1 itself.
func TestParseColorValIsTypeDependent(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want Color
	}{
		{"int-branch", 5, 5},
		{"color-branch", Color(5), 6}, // +1: 0 is reserved as "not set"
		{"int-zero", 0, 0},            // 0 means "not set" in addColorStyle
		{"color-zero", Color(0), 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var col Color
			var rgb ColorRGB
			parseColorVal(c.in, &col, &rgb)
			if col != c.want {
				t.Errorf("got %d, want %d", col, c.want)
			}
		})
	}

	t.Log("known gap: Foreground(5) and Foreground(Color(5)) produce different colours")
}

func TestParseColorValReset(t *testing.T) {
	var c Color = 9
	var rgb ColorRGB
	parseColorVal(ColorReset, &c, &rgb)
	if c != 0 {
		t.Errorf("got %d, want 0", c)
	}
}

// CreateLogger applies defaults only where the caller left a component unset.
// Level is guarded per component. Params is guarded as a whole array. The two
// rules are not the same, and these tests record what each one does.

func TestCallerStyleSurvivesCreateLogger(t *testing.T) {
	c := Config{Style: Style{Level: [MaxLevel]CompStyle{NewStyle().Foreground("#D75F5F")}}}
	l := CreateLogger(c)

	if l.Config.Style.Level[0] != c.Style.Level[0] {
		t.Errorf("level 0 overwritten: got %v, want %v", l.Config.Style.Level[0], c.Style.Level[0])
	}

	if l.Config.Style.Level[1] == (CompStyle{}) {
		t.Error("level 1 lost the package default")
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
			t.Errorf("params %d lost the package default", i)
		}
	}
}

// Foreground(ColorReset) sets the sentinel 0, which is the zero value, so the
// guard cannot tell "not set" from "set to nothing". An explicit request for no
// colour on a level is replaced by the default.
func TestExplicitNoColourCannotBeExpressed(t *testing.T) {
	c := Config{Style: Style{Level: [MaxLevel]CompStyle{NewStyle().Foreground(ColorReset)}}}
	l := CreateLogger(c)

	if l.Config.Style.Level[0].fg == 0 {
		t.Error("expected the guard to fire for an explicit no-colour request")
	}
}
