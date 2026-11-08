package stacklog

type (
	Color       uint8
	colorTarget uint8

	ColorRGB struct {
		R Color
		G Color
		B Color
	}
)

const ColorReset string = "reset"

const (
	TargetFG colorTarget = 0
	TargetBG colorTarget = 1
)

func (sb *StackBuffer) writeEachByte(c Color) *StackBuffer {
	if c >= 100 {
		sb.AppendByte('0' + byte(c/100))
		sb.AppendByte('0' + byte((c/10)%10))
		sb.AppendByte('0' + byte(c%10))
	} else if c >= 10 {
		sb.AppendByte('0' + byte(c/10))
		sb.AppendByte('0' + byte(c%10))
	} else {
		sb.AppendByte('0' + byte(c))
	}

	return sb
}

func (sb *StackBuffer) writeRGBColor(target colorTarget, rgb ColorRGB) *StackBuffer {
	sb.AppendByte(0x1b)
	sb.AppendByte('[')

	var ansiTarget Color

	if target == TargetFG {
		ansiTarget = 38
	} else {
		ansiTarget = 48
	}

	sb.writeEachByte(ansiTarget)

	sb.WriteString(";2;")

	sb.writeEachByte(rgb.R)
	sb.AppendByte(';')
	sb.writeEachByte(rgb.G)
	sb.AppendByte(';')
	sb.writeEachByte(rgb.B)

	sb.AppendByte('m')

	return sb
}

func (sb *StackBuffer) writeColor(target colorTarget, c Color) *StackBuffer {
	var initColor Color = c

	sb.AppendByte(0x1b)
	sb.AppendByte('[')

	switch true {
	case c >= 16:
		// ANSI256 - \x1b[38;5;166m
		var ansiTarget Color

		if target == TargetFG {
			ansiTarget = 38
		} else {
			ansiTarget = 48
		}

		sb.writeEachByte(ansiTarget)
		sb.WriteString(";5;")
		sb.writeEachByte(c)
	case c >= 8:
		// ANSI16 - \x1b[42m
		var ansiBase Color
		if target == TargetFG {
			if initColor > 7 {
				ansiBase = 90
			} else {
				ansiBase = 30
			}
		} else {
			if initColor > 7 {
				ansiBase = 100
			} else {
				ansiBase = 40
			}
		}

		c = ansiBase + (initColor % 8)

		sb.writeEachByte(c)
	default:
		// ANSI8 - \x1b[32m
		var ansiBase Color
		if target == TargetFG {
			ansiBase = 30
		} else {
			ansiBase = 40
		}

		c = ansiBase + initColor

		sb.writeEachByte(c)
	}

	sb.AppendByte('m')

	return sb
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

func (sb *StackBuffer) ResetColor() *StackBuffer {
	sb.AppendByte(0x1b)
	sb.AppendByte('[')
	sb.AppendByte('0')
	sb.AppendByte('m')

	return sb
}

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
