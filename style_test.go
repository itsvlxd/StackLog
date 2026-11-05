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
