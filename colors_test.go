package stacklog

import "testing"

func TestColorTargets(t *testing.T) {
	cases := []struct {
		name string
		call func(*StackBuffer)
		want string
	}{
		{"bg-color-4", func(b *StackBuffer) { b.BgColor(Color(4)) }, "\x1b[44m"},
		{"fg-color-4", func(b *StackBuffer) { b.FgColor(Color(4)) }, "\x1b[34m"},
		{"bg-rgb", func(b *StackBuffer) { b.BgColor(ColorRGB{1, 2, 3}) }, "\x1b[48;2;1;2;3m"},
		{"fg-rgb", func(b *StackBuffer) { b.FgColor(ColorRGB{1, 2, 3}) }, "\x1b[38;2;1;2;3m"},
		{"reset", func(b *StackBuffer) { b.ResetColor() }, "\x1b[0m"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sb StackBuffer
			c.call(&sb)
			if got := string(sb.Buffer[:sb.BufLen]); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// knownGap marks inputs that should produce a failure, not a colour.
// Today HexToRGB cannot report failure, so these assert current behaviour and
// log the gap. They must be rewritten when the signature changes.
func TestHexToRGB(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     ColorRGB
		knownGap bool
	}{
		{"valid-red", "#FF0000", ColorRGB{255, 0, 0}, false},
		{"valid-mixed-case", "#aBcDeF", ColorRGB{171, 205, 239}, false},
		{"valid-black", "#000000", ColorRGB{0, 0, 0}, false},
		{"invalid-chars", "#zzzzzz", ColorRGB{0, 0, 0}, true},
		{"empty", "", ColorRGB{255, 255, 255}, true},
		{"shorthand", "#abc", ColorRGB{255, 255, 255}, true},
		{"no-hash", "FF0000", ColorRGB{255, 255, 255}, true},
		{"too-long", "#1234567", ColorRGB{255, 255, 255}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := HexToRGB(c.in)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
			if c.knownGap {
				t.Logf("known gap: %q produced a colour instead of a failure", c.in)
			}
		})
	}
}

func FuzzHexToRGB(f *testing.F) {
	f.Add("#FF0000")
	f.Add("#abc")
	f.Add("")
	f.Add("#zzzzzz")

	f.Fuzz(func(t *testing.T, s string) {
		HexToRGB(s) // must not panic for any input
	})
}

// The boundaries of the three branches in writeColor. Asserting the middle of a
// range proves nothing; the edges are where an off-by-one shows up.
func TestColorBoundaries(t *testing.T) {
	cases := []struct {
		name string
		call func(*StackBuffer)
		want string
	}{
		{"fg-normal-last", func(b *StackBuffer) { b.FgColor(Color(7)) }, "\x1b[37m"},
		{"fg-bright-first", func(b *StackBuffer) { b.FgColor(Color(8)) }, "\x1b[90m"},
		{"fg-bright-last", func(b *StackBuffer) { b.FgColor(Color(15)) }, "\x1b[97m"},
		{"fg-extended-first", func(b *StackBuffer) { b.FgColor(Color(16)) }, "\x1b[38;5;16m"},
		{"bg-normal-last", func(b *StackBuffer) { b.BgColor(Color(7)) }, "\x1b[47m"},
		{"bg-bright-first", func(b *StackBuffer) { b.BgColor(Color(8)) }, "\x1b[100m"},
		{"bg-bright-last", func(b *StackBuffer) { b.BgColor(Color(15)) }, "\x1b[107m"},
		{"bg-extended-first", func(b *StackBuffer) { b.BgColor(Color(16)) }, "\x1b[48;5;16m"},
		{"fg-extended-last", func(b *StackBuffer) { b.FgColor(Color(255)) }, "\x1b[38;5;255m"},
		{"bg-extended-last", func(b *StackBuffer) { b.BgColor(Color(255)) }, "\x1b[48;5;255m"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sb StackBuffer
			c.call(&sb)
			if got := string(sb.Buffer[:sb.BufLen]); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Values outside the range Color can hold are clamped, not wrapped. A value above
// 255 has no colour meaning, so the closest valid answer is the top of the range;
// a negative value is not a colour index at all, so it goes to the bottom.
func TestColorValuesAreClampedNotWrapped(t *testing.T) {
	cases := []struct {
		in   any
		want Color
	}{
		{300, 255},
		{256, 255},
		{255, 255},
		{0, 0},
		{-1, 0},
		{int64(1 << 40), 255},
		{uint64(1 << 63), 255},
	}

	for _, c := range cases {
		var col Color = 7
		var rgb ColorRGB
		var set bool
		parseColorVal(c.in, &col, &rgb, &set)
		if col != c.want || !set {
			t.Errorf("%v: got %d/%v, want %d/true", c.in, col, set, c.want)
		}
	}

	t.Log("clamping is a contract now: a value outside 0-255 resolves to the nearest valid end rather than wrapping.")
}
