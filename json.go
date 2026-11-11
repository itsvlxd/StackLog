package stacklog

import (
	"time"
	"unicode/utf8"
)

// Format selects what one log call writes.
//
// FormatText is the coloured line, and it is the zero value: a Config that says nothing
// about format writes exactly what it wrote before the field existed. An unknown name
// resolves to FormatText, which is the same rule Style.Preset applies to a preset name.
//
// FormatJSON writes one JSON object per call, newline terminated - JSON Lines. The
// one-write-per-line contract survives. Pretty-printed output would break it, so that is
// not this format.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// jsonTailReserve is the room a JSON line holds back from its content area for the bytes
// it must always emit after the content: the brace closing params, the truncated field,
// the brace closing the object, and the terminator.
//
// Content stops at 1004 bytes in JSON mode against 1020 in text mode. A JSON line is only
// worth reading if the structure that opens it is always closed, so the tail is reserved
// rather than hoped for.
const jsonTailReserve = 1 + len(`,"truncated":true`) + 1 + 1

// jsonUnsupported is the same token text mode writes for a type the package cannot
// render. Keeping one vocabulary across both formats means a caller can grep for it in
// either. It is a string in JSON rather than null so that "this could not be rendered" is
// distinguishable from "this value is nil".
const jsonUnsupported = "<unsupported>"

const jsonHexDigits = "0123456789abcdef"

// levelNames are the JSON values for each level. Style.LevelPrefix is not read in JSON
// mode: there the level is a field with a fixed set of values, and a display label like
// DEBU or DBG would either duplicate it or replace it with something a parser cannot match
// on. A JSON mode therefore cannot use the label sets the presets define.
var levelNames = [MaxLevel]string{"debug", "info", "warn", "error", "fatal"}

// Entry is the JSON form of one log line. A caller can decode any line written in JSON
// mode into it, which is what makes the field names part of the API rather than an
// accident of the writer.
//
// Time is empty when the logger was created without Config.Timestamp. Params is empty when
// the call carried none, and a pair that did not fit is absent from it entirely - which is
// knowable because Truncated is set. A value the package cannot render decodes as the
// string "<unsupported>".
type Entry struct {
	Time      string         `json:"time,omitempty"`
	Level     string         `json:"level"`
	Prefix    string         `json:"prefix,omitempty"`
	Msg       string         `json:"msg"`
	Params    map[string]any `json:"params,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}

// applyJSONFormat makes a logger and its buffer agree about JSON mode. It runs in
// CreateLogger and in derive, so a format set at creation reaches every derived logger the
// way MinLevel does.
//
// Colours are suppressed by setting NoColors rather than by writing a second suppression
// path: the flag exists, it already wins over every component, and escape sequences inside
// a JSON string are noise to a parser whether or not the caller asked for them.
func applyJSONFormat(config *Config, sb *StackBuffer) {
	if config.Format != FormatJSON {
		return
	}

	config.Style.NoColors = true
	sb.reserve = jsonTailReserve
}

// addJSONTime writes "time":RFC3339, into the buffer.
//
// The value is formatted straight into a 64-byte stack array rather than through time.Now
// .Format, so the normal layouts cost no allocation. An absurd Config.TimeFormat grows
// that array and allocates, which is the caller's choice rather than the package's.
//
// A timestamp that will not fit whole is dropped rather than cut. A parser cannot tell a
// truncated timestamp from a corrupt clock, so a line with no time is the honest output.
func (sl *StackLog) addJSONTime() {
	buf := sl.Buffer

	mark, was := buf.BufLen, buf.truncated

	var tb [64]byte
	formatted := time.Now().AppendFormat(tb[:0], sl.Config.TimeFormat)

	buf.WriteString(`"time":`)
	buf.AppendByte('"')
	buf.Write(formatted)
	buf.AppendByte('"')
	buf.AppendByte(',')

	if buf.truncated && !was {
		buf.BufLen = mark
	}
}

// addJSONLevel writes "level":"error",. The level is the canonical name taken from
// levelNames, never the label in Style.LevelPrefix.
func (sl *StackLog) addJSONLevel(level LogLevel) {
	buf := sl.Buffer

	buf.WriteString(`"level":"`)
	buf.WriteString(levelNames[level])
	buf.WriteString(`",`)
}

// addJSONPrefix writes "prefix":"http", with the text-mode trailing colon dropped: the
// colon is punctuation for a human reading a line, and a field name already says what it
// is. It is dropped whole rather than cut if it will not fit.
func (sl *StackLog) addJSONPrefix() {
	buf := sl.Buffer

	mark, was := buf.BufLen, buf.truncated

	buf.WriteString(`"prefix":`)
	buf.writeJSONString(sl.prefix)
	buf.AppendByte(',')

	if buf.truncated && !was {
		buf.BufLen = mark
	}
}

// addJSONMsg writes "msg":"...",. This is the field that gives way: a message too long for
// the line is cut inside its own string, ends with the marker, and closes its quote. The
// object stays parseable because the cut happens before the value ends rather than after
// the structure does.
func (sl *StackLog) addJSONMsg(message string) {
	buf := sl.Buffer

	buf.WriteString(`"msg":`)
	buf.writeJSONString(message)
	buf.AppendByte(',')
}

// addJSONParams writes "params":{ "key":value,... } and reports whether the field is
// open, because the brace that closes it is emitted with the rest of the tail and only if
// the object is actually there.
//
// A pair is written whole or not at all. The position is saved before the pair, the
// truncated flag is cleared for the attempt, and the position is restored if the attempt
// reports truncation - so the output never contains half a pair: no dangling "key":, no
// opening quote with no closing one. Clearing the flag per pair is what makes that work:
// truncated is sticky across the line, and a signal raised by the message cannot be allowed
// to make a later pair look intact.
//
// Pairs after a dropped one are still tried, because a value too long for the line says
// nothing about the value that follows it. If nothing fits, the field is absent and truncated
// is still set - a reader has to be able to tell "no params" from "params were lost".
//
// An odd number of arguments drops the dangling value: a JSON object needs a key for every
// value, and inventing one is worse than dropping the pair. Duplicate keys are legal JSON and
// are written as given; every decoder a Go caller reaches takes the last one.
func (sl *StackLog) addJSONParams() bool {
	buf := sl.Buffer

	fieldMark, fieldWas := buf.BufLen, buf.truncated

	buf.WriteString(`"params":{`)

	var wrote, cut bool

	for i := 0; i+1 < len(sl.params); i += 2 {
		pairMark := buf.BufLen
		buf.truncated = false

		if wrote {
			buf.AppendByte(',')
		}

		buf.writeJSONKey(sl.params[i])
		buf.AppendByte(':')
		buf.writeJSONValue(sl.params[i+1])

		if buf.truncated {
			buf.BufLen = pairMark
			cut = true
		} else {
			wrote = true
		}
	}

	buf.truncated = fieldWas || cut

	if !wrote {
		buf.BufLen = fieldMark

		return false
	}

	buf.AppendByte(',')

	return true
}

// writeJSONKey writes k as an object key. A key that is already a string is escaped like
// one; any other type travels through the same rendering text mode uses, inside quotes, so
// WithParams(1, "x") writes "1":"x" rather than dropping the pair.
func (sb *StackBuffer) writeJSONKey(k any) {
	if s, ok := k.(string); ok {
		sb.writeJSONString(s)
		return
	}

	sb.AppendByte('"')
	sb.AppendAny(k)
	sb.AppendByte('"')
}

// writeJSONValue writes v as a JSON value, keeping the Go type: numbers are numbers, a bool
// is a bool, a duration is a number of nanoseconds because that is what a Duration is, a nil
// interface is null, and a type the package cannot render is the string <unsupported>.
//
// error travels as a string, which is the only thing it has to say.
func (sb *StackBuffer) writeJSONValue(v any) {
	switch p := v.(type) {
	case string:
		sb.writeJSONString(p)
	case bool:
		sb.AppendBool(p)
	case int:
		sb.AppendInt(int64(p))
	case int8:
		sb.AppendInt(int64(p))
	case int16:
		sb.AppendInt(int64(p))
	case int32:
		sb.AppendInt(int64(p))
	case int64:
		sb.AppendInt(p)
	case time.Duration:
		sb.AppendInt(int64(p))
	case uint:
		sb.AppendUint(uint64(p))
	case uint8:
		sb.AppendUint(uint64(p))
	case uint16:
		sb.AppendUint(uint64(p))
	case uint32:
		sb.AppendUint(uint64(p))
	case uint64:
		sb.AppendUint(p)
	case float32:
		sb.appendFloat(float64(p), 32)
	case float64:
		sb.AppendFloat(p)
	case error:
		sb.writeJSONString(p.Error())
	case nil:
		sb.WriteString("null")
	default:
		sb.writeJSONString(jsonUnsupported)
	}
}

// writeJSONString writes s as a quoted JSON string and reports whether it was cut.
//
// The double quote, the backslash, every control byte below 0x20 and every invalid UTF-8
// sequence are escaped, because those are the four ways a Go string breaks JSON. Valid
// multi-byte runes travel as themselves - JSON allows them and escaping them to \uXXXX makes
// a log line harder to read for no gain. A cut never splits an escape sequence or a rune:
// the loop stops before a piece that will not fit whole.
//
// A cut value ends with the marker and its closing quote, and marks the buffer truncated so
// the line gains "truncated":true.
func (sb *StackBuffer) writeJSONString(s string) bool {
	sb.AppendByte('"')

	room := sb.usable() - sb.BufLen - len(truncateMark) - 1
	if room < 0 {
		room = 0
	}

	var esc [6]byte

	var i, used int

	for i < len(s) && used < room {
		b := s[i]

		// A run of ordinary printable ASCII is copied in one move. This is the point of the
		// fast path: a log message is mostly characters JSON has no opinion about, and deciding
		// that one byte at a time costs more than the decision is worth. Bytes 0x80 and above are
		// left to the decode below, where validity is actually checked rather than assumed.
		if b >= 0x20 && b < 0x80 && b != '"' && b != '\\' {
			start := i

			for i < len(s) && s[i] >= 0x20 && s[i] < 0x80 && s[i] != '"' && s[i] != '\\' {
				i++
			}

			n := i - start

			if n > room-used {
				n = room - used
				i = start + n
			}

			copy(sb.Buffer[sb.BufLen+used:], s[start:start+n])
			used += n

			continue
		}

		var piece []byte

		if b >= 0x80 {
			_, size := utf8.DecodeRuneInString(s[i:])

			if size > 1 {
				// Valid multi-byte UTF-8 needs no escape and JSON accepts it as itself.
				// Escaping it to \\uXXXX would be legal and would make a line harder to read.
				// A character never splits: if it will not fit whole, the cut happens before it.
				if size > room-used {
					break
				}

				copy(sb.Buffer[sb.BufLen+used:], s[i:i+size])
				used += size
				i += size

				continue
			}

			// A byte that is not valid UTF-8 at all. JSON must be valid UTF-8, so the
			// replacement character stands in for it instead of the raw byte going through.
			esc[0], esc[1], esc[2] = 0xEF, 0xBF, 0xBD
			piece = esc[:3]
		} else {
			switch b {
			case '"':
				esc[0], esc[1] = '\\', '"'
				piece = esc[:2]
			case '\\':
				esc[0], esc[1] = '\\', '\\'
				piece = esc[:2]
			case '\n':
				esc[0], esc[1] = '\\', 'n'
				piece = esc[:2]
			case '\r':
				esc[0], esc[1] = '\\', 'r'
				piece = esc[:2]
			case '\t':
				esc[0], esc[1] = '\\', 't'
				piece = esc[:2]
			case 0x08:
				esc[0], esc[1] = '\\', 'b'
				piece = esc[:2]
			case 0x0c:
				esc[0], esc[1] = '\\', 'f'
				piece = esc[:2]
			default:
				if b < 0x20 {
					// A control byte with no short escape. json.Valid and every decoder a Go
					// caller reaches require it escaped.
					esc[0], esc[1], esc[2], esc[3] = '\\', 'u', '0', '0'
					esc[4] = jsonHexDigits[b>>4]
					esc[5] = jsonHexDigits[b&0x0f]
					piece = esc[:6]
				} else {
					// DEL and anything else that is a single byte and legal inside a JSON
					// string as it is.
					esc[0] = b
					piece = esc[:1]
				}
			}
		}

		if len(piece) > room-used {
			break
		}

		copy(sb.Buffer[sb.BufLen+used:], piece)
		used += len(piece)
		i++
	}

	sb.BufLen += used

	cut := i < len(s)

	if cut {
		copy(sb.Buffer[sb.BufLen:], truncateMark)
		sb.BufLen += len(truncateMark)
		sb.truncated = true
	}

	sb.AppendByte('"')

	return cut
}

// writeJSONTail writes a closing piece into the reserved tail. It reports false when the
// piece does not fit, which is the signal that the line cannot be closed at all.
func (sb *StackBuffer) writeJSONTail(s string) bool {
	if sb.BufLen+len(s) > len(sb.Buffer) {
		return false
	}

	copy(sb.Buffer[sb.BufLen:], s)
	sb.BufLen += len(s)

	return true
}

// printJSON builds one JSON object into the fixed buffer and hands it to a single Write.
//
// Every field except the message is written whole or dropped, which is what keeps a full
// buffer from producing output a parser cannot read. The trailing comma each field writes
// after itself is stripped before the structure closes: a dropped field takes its comma
// with it, so no field needs to know whether it came first.
func (sl *StackLog) printJSON(level LogLevel, message string) error {
	buf := sl.Buffer

	buf.AppendByte('{')

	if buf.usable()-buf.BufLen < len(`"level":"","msg":""`)+len(truncateMark)+1 {
		buf.Reset()

		return ErrLineTooLarge
	}

	if sl.Config.Timestamp {
		sl.addJSONTime()
	}

	sl.addJSONLevel(level)

	if len(sl.prefix) > 0 {
		sl.addJSONPrefix()
	}

	sl.addJSONMsg(message)

	paramsOpen := false
	if len(sl.params) > 0 {
		paramsOpen = sl.addJSONParams()
	}

	// A cut inside params leaves the buffer at its content limit, so the closing braces are
	// the last thing standing between a valid line and a broken one.
	if buf.BufLen > 0 && buf.Buffer[buf.BufLen-1] == ',' {
		buf.BufLen--
	}

	if paramsOpen && !buf.writeJSONTail("}") {
		buf.Reset()

		return ErrLineTooLarge
	}

	if buf.truncated && !buf.writeJSONTail(`,"truncated":true`) {
		buf.Reset()

		return ErrLineTooLarge
	}

	if !buf.writeJSONTail("}") {
		buf.Reset()

		return ErrLineTooLarge
	}

	buf.WriteNL()

	return sl.printBuffer()
}
