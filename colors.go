package stacklog

// Color is a colour index. 0-7 are the eight base colours, 8-15 the bright
// variants, and 16-255 the 256-colour palette. It is uint8, so a value outside
// 0-255 cannot exist as a Color. Anything larger is clamped when it is accepted
// through Foreground or Background.
type Color uint8

// colorTarget selects which half of a colour pair writeColor applies to.
type colorTarget uint8

// ColorRGB is a truecolour triple. Each component is a Color, so 0-255.
//
// A zero ColorRGB is treated as "no colour set", so ColorRGB{0,0,0} - black -
// takes the index path rather than the RGB path. It renders the same black through
// a different escape sequence, so the colour is the same and the code is not.
type ColorRGB struct {
	R Color
	G Color
	B Color
}

// ColorReset is not a colour. It is the value that clears a component's flag, so
// Foreground(ColorReset) means "nothing set" rather than "reset to default". Black
// is Foreground(0).
const ColorReset string = "reset"

// TargetFG and TargetBG select the foreground or background half of a colour pair.
// They are only used inside the package.
const (
	TargetFG colorTarget = 0
	TargetBG colorTarget = 1
)

// appendDecimal writes v as one to three decimal digits at the end of dst and returns
// the new length. dst must have room for three bytes.
func appendDecimal(dst []byte, n int, v uint8) int {
	switch {
	case v >= 100:
		dst[n], dst[n+1], dst[n+2] = '0'+v/100, '0'+(v/10)%10, '0'+v%10
		return n + 3
	case v >= 10:
		dst[n], dst[n+1] = '0'+v/10, '0'+v%10
		return n + 2
	default:
		dst[n] = '0' + v
		return n + 1
	}
}

func (sb *StackBuffer) writeRGBColor(target colorTarget, rgb ColorRGB) *StackBuffer {
	var e [24]byte
	n := 2
	e[0], e[1] = 0x1b, '['

	if target == TargetFG {
		e[2], e[3] = '3', '8'
	} else {
		e[2], e[3] = '4', '8'
	}
	n += 2

	e[n], e[n+1], e[n+2] = ';', '2', ';'
	n += 3

	n = appendDecimal(e[:], n, uint8(rgb.R))
	e[n] = ';'
	n++
	n = appendDecimal(e[:], n, uint8(rgb.G))
	e[n] = ';'
	n++
	n = appendDecimal(e[:], n, uint8(rgb.B))
	e[n] = 'm'
	n++

	return sb.writeInto(e[:n])
}

func (sb *StackBuffer) writeColor(target colorTarget, c Color) *StackBuffer {
	var e [24]byte
	n := 2
	e[0], e[1] = 0x1b, '['

	switch {
	case c >= 16:
		if target == TargetFG {
			e[2], e[3] = '3', '8'
		} else {
			e[2], e[3] = '4', '8'
		}
		n += 2
		e[n], e[n+1], e[n+2] = ';', '5', ';'
		n += 3
		n = appendDecimal(e[:], n, uint8(c))
	case c >= 8:
		base := uint8(90)
		if target == TargetBG {
			base = 100
		}
		n = appendDecimal(e[:], n, base+(uint8(c)-8))
	default:
		base := uint8(30)
		if target == TargetBG {
			base = 40
		}
		n = appendDecimal(e[:], n, base+uint8(c))
	}

	e[n] = 'm'
	n++

	return sb.writeInto(e[:n])
}

func hexCharToByte(ch byte) Color {
	if ch >= '0' && ch <= '9' {
		return Color(ch - '0')
	}
	if ch >= 'A' && ch <= 'F' {
		return Color(10 + (ch - 'A'))
	}
	if ch >= 'a' && ch <= 'f' {
		return Color(10 + (ch - 'a'))
	}
	return 0
}

// HexToRGB converts a #RRGGBB string to a colour triple.
//
// It cannot report failure. Any input that is not exactly seven characters starting
// with # returns white. That is a known gap: an invalid hex string should produce
// an error, not a colour.
func HexToRGB(hex string) ColorRGB {
	var (
		r Color
		g Color
		b Color
	)

	if len(hex) != 7 || hex[0] != '#' {
		return ColorRGB{R: 255, G: 255, B: 255}
	}

	r = (hexCharToByte(hex[1]) * 16) + hexCharToByte(hex[2])
	g = (hexCharToByte(hex[3]) * 16) + hexCharToByte(hex[4])
	b = (hexCharToByte(hex[5]) * 16) + hexCharToByte(hex[6])

	return ColorRGB{R: r, G: g, B: b}
}

// ResetColor writes the ANSI reset sequence. It is written by addPostStyle at the
// end of every component that opened a colour, which is why each line ends clean
// and the next line starts from a known state. Callers do not normally need it.
func (sb *StackBuffer) ResetColor() *StackBuffer {
	return sb.WriteString("\x1b[0m")
}

// FgColor writes the escape sequence for a foreground colour. val is a Color index
// or a ColorRGB triple; any other type writes nothing at all. Indices 0-7 are the
// base colours, 8-15 the bright variants, 16-255 the 256-colour palette.
func (sb *StackBuffer) FgColor(val any) *StackBuffer {
	switch c := val.(type) {
	case Color:
		return sb.writeColor(TargetFG, c)
	case ColorRGB:
		return sb.writeRGBColor(TargetFG, c)
	default:
		return sb
	}
}

// BgColor writes the escape sequence for a background colour. Same accepted forms
// and the same index mapping as FgColor, against the 40-47 and 100-107 ranges.
func (sb *StackBuffer) BgColor(val any) *StackBuffer {
	switch c := val.(type) {
	case Color:
		return sb.writeColor(TargetBG, c)
	case ColorRGB:
		return sb.writeRGBColor(TargetBG, c)
	default:
		return sb
	}
}
