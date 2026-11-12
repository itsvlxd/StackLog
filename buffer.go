package stacklog

import (
	"strconv"
	"time"
)

// truncateMark is appended to a line whose content was cut by the fixed buffer, so
// a reader can tell a truncated line from a complete one.
const truncateMark = "..."

// truncateReserve is the tail of the buffer held back from content writes so the
// marker and the line terminator always fit. Without the reserve, a full buffer
// drops the terminator and merges two log lines into one run of text.
const truncateReserve = len(truncateMark) + 1

// StackBuffer is a fixed, pre-allocated byte buffer. It never grows. Content
// writes stop at 1020 bytes - 1024 less the reserved tail - and a write that is
// cut marks the line so the marker can be appended before the flush.
//
// lineStart and maxBottom carry the padding rules across a single log line:
// Top applies only at the start of a line, Bottom is never emitted mid-line and
// is flushed as the maximum across the whole line.
type StackBuffer struct {
	Buffer    [1024]byte
	BufLen    int
	reserve   int
	lineStart bool
	maxBottom int
	truncated bool
}

// usable is how much of the buffer content may occupy. The tail is reserved for
// the marker and the terminator. reserve is a field rather than the constant because
// a JSON line needs a wider tail than a text line: the structure that opens has to be
// closed, and 4 bytes is not enough to close an object.
//
// A buffer built as a zero value gets the text reserve rather than none. StackBuffer is
// exported with exported fields, so "var sb StackBuffer" is a legal way to make one, and a
// zero reserve would let a content write run into the tail the package promised to keep.
func (sb *StackBuffer) usable() int {
	reserve := sb.reserve
	if reserve < truncateReserve {
		reserve = truncateReserve
	}

	return len(sb.Buffer) - reserve
}

// markTruncated writes the marker into the reserved tail. It is a no-op on a line
// that was not cut.
func (sb *StackBuffer) markTruncated() *StackBuffer {
	if !sb.truncated {
		return sb
	}

	copy(sb.Buffer[sb.BufLen:], truncateMark)
	sb.BufLen += len(truncateMark)

	return sb
}

// writeTail copies bytes past the content limit into the reserved tail, the way the marker
// does. It exists for a frame's clearing sequence: content writes stop at usable(), so a bar
// long enough to be cut would otherwise lose the erase-in-line bytes, and a bar that loses them
// leaves the remnants of a longer frame on the reader's screen. The tail is 4 bytes in text mode
// and 20 in JSON mode, and the sequence is 3, so the copy is always inside the buffer.
//
// A frame does not also carry the "..." marker. Both cannot fit in a 4-byte tail, and between a
// progress bar that says it was cut and one that stops erasing itself, the second is the worse
// failure. The clamp on bar width is what keeps this case from arriving at all.
func (sb *StackBuffer) writeTail(s string) *StackBuffer {
	copy(sb.Buffer[sb.BufLen:], s)
	sb.BufLen += len(s)

	return sb
}

// CreateBuffer initialises a StackBuffer and returns its pointer. The value
// stays on the caller's stack only while the pointer never leaves the frame that
// created it — storing it in a struct that escapes moves it to the heap.
func CreateBuffer() *StackBuffer {
	var sb StackBuffer

	sb.BufLen = 0
	sb.reserve = truncateReserve
	sb.lineStart = true

	return &sb
}

// AppendInt writes the base-10 form of n into the buffer. The 24-byte scratch
// array covers the worst case for an int64, which is 20 bytes including the
// sign. Nothing escapes, so this allocates nothing.
func (sb *StackBuffer) AppendInt(n int64) *StackBuffer {
	var num [24]byte
	return sb.writeInto(strconv.AppendInt(num[:0], n, 10))
}

// AppendUint writes the base-10 form of n into the buffer. The worst case for a
// uint64 is 20 digits, so the same 24-byte array is enough.
func (sb *StackBuffer) AppendUint(n uint64) *StackBuffer {
	var num [24]byte
	return sb.writeInto(strconv.AppendUint(num[:0], n, 10))
}

// AppendBool writes "true" or "false" into the buffer. Five bytes is exactly the
// worst case, so there is no margin and no allocation.
func (sb *StackBuffer) AppendBool(b bool) *StackBuffer {
	var bool [5]byte
	return sb.writeInto(strconv.AppendBool(bool[:0], b))
}

// AppendFloat writes n in the shortest decimal that reads back as the same
// float64: 1.44, not 1.44E+00. The 32-byte scratch array covers the worst case,
// which is about 24 bytes for a value like -1.2345678901234568e-308, so the append
// never grows the slice and nothing escapes to the heap.
func (sb *StackBuffer) AppendFloat(n float64) *StackBuffer {
	return sb.appendFloat(n, 64)
}

// appendFloat does the work. bitSize decides what shortest means: -1 at 32 is the
// shortest decimal that reads back as the same float32, which is why float32 must
// not travel through the 64-bit path. float32(1.1) at bitSize 64 is
// 1.100000023841858, and at 32 it is 1.1.
func (sb *StackBuffer) appendFloat(n float64, bitSize int) *StackBuffer {
	var float [32]byte

	return sb.writeInto(strconv.AppendFloat(float[:0], n, 'g', -1, bitSize))
}

// AppendError writes the message from e. A nil error writes "<nil>" so a key is
// never left with an empty value.
//
// This catches an untyped nil only. A nil pointer held in an error interface is not
// nil, and calling Error on it is the caller's bug:
//
//	var e *MyError = nil
//	logger.WithParams("err", e) // e is not nil, so Error runs on a nil receiver
//
// Catching that would need reflection or a recover on the per-line path, which is a
// worse trade than stating the limit.
func (sb *StackBuffer) AppendError(e error) *StackBuffer {
	if e == nil {
		return sb.WriteString("<nil>")
	}

	return sb.WriteString(e.Error())
}

// AppendAny dispatches on the dynamic type and writes the value straight into the
// buffer. It does not build an intermediate string. Types it does not handle are
// written as "<unsupported>" rather than printed outside the log stream.
//
// The conversion to any happens at the call site, so this does not remove the
// boxing cost — it removes the slice of strings that used to sit on top of it.
//
// Handled: every int and uint width, float32 and float64, bool, string, error, and
// time.Duration. A Duration is written as nanoseconds because that is what it is —
// an int64 counted in nanoseconds — so 1500 * time.Microsecond writes 1500000.
//
// Two consequences of the type switch worth knowing. rune is an alias for int32, so
// a rune is written as a number rather than a character. And error is matched after
// the concrete types, so a type that both has a case here and implements error is
// read as the type it names. A nil interface has no dynamic type at all, so it gets
// its own case and writes "<nil>".
func (sb *StackBuffer) AppendAny(a any) *StackBuffer {
	switch p := a.(type) {
	case int:
		return sb.AppendInt(int64(p))
	case int8:
		return sb.AppendInt(int64(p))
	case int16:
		return sb.AppendInt(int64(p))
	case int32:
		return sb.AppendInt(int64(p))
	case int64:
		return sb.AppendInt(p)
	case time.Duration:
		return sb.AppendInt(int64(p))
	case uint:
		return sb.AppendUint(uint64(p))
	case uint8:
		return sb.AppendUint(uint64(p))
	case uint16:
		return sb.AppendUint(uint64(p))
	case uint32:
		return sb.AppendUint(uint64(p))
	case uint64:
		return sb.AppendUint(p)
	case float32:
		return sb.appendFloat(float64(p), 32)
	case float64:
		return sb.AppendFloat(p)
	case bool:
		return sb.AppendBool(p)
	case string:
		return sb.WriteString(p)
	case error:
		return sb.AppendError(p)
	case nil:
		// A nil interface has no dynamic type, so it cannot reach the error case
		// above. Writing "<nil>" here is what makes WithParams("err", err) readable
		// when err happens to be nil, instead of reporting a type nobody recognises.
		return sb.WriteString("<nil>")
	default:
		return sb.WriteString("<unsupported>")
	}
}

// AppendByte writes a single byte inside the content area. It stops at the
// reserved tail rather than at the end of the buffer, so a byte that will not fit
// is dropped and the marker and the terminator keep their room. Nothing panics;
// the guard is the whole behaviour.
//
// This is the method behind the AppendByte contradiction: the comment claimed a
// panic, the code silently drops. The code was right about the panic being wrong
// for a logger, and wrong about the drop being harmless.
func (sb *StackBuffer) AppendByte(b byte) *StackBuffer {
	if sb.BufLen < sb.usable() {
		sb.Buffer[sb.BufLen] = b
		sb.BufLen++
	}

	return sb
}

// writeInto copies data into the content area and records whether anything was
// cut. This is the single place truncation is detected, so every content path
// behaves the same way.
func (sb *StackBuffer) writeInto(data []byte) *StackBuffer {
	if sb.BufLen >= sb.usable() {
		sb.truncated = true
		return sb
	}

	n := copy(sb.Buffer[sb.BufLen:sb.usable()], data)
	sb.BufLen += n

	if n < len(data) {
		sb.truncated = true
	}

	return sb
}

// Write appends the provided byte slice into the current buffer. A value longer
// than the remaining content space is cut, and the line is marked so the flush
// appends the truncation marker.
func (sb *StackBuffer) Write(data []byte) *StackBuffer {
	return sb.writeInto(data)
}

// writeStringInto is the string form of writeInto. It is a separate method rather
// than a conversion at the call, because []byte(message) at a function boundary
// allocates, and the per-line path must not.
func (sb *StackBuffer) writeStringInto(message string) *StackBuffer {
	if sb.BufLen >= sb.usable() {
		sb.truncated = true
		return sb
	}

	n := copy(sb.Buffer[sb.BufLen:sb.usable()], message)
	sb.BufLen += n

	if n < len(message) {
		sb.truncated = true
	}

	return sb
}

// WriteString copies a string into the current buffer, cutting at the content
// limit and marking the line if anything was lost.
func (sb *StackBuffer) WriteString(message string) *StackBuffer {
	return sb.writeStringInto(message)
}

// WriteBlank writes a single space.
func (sb *StackBuffer) WriteBlank() *StackBuffer {
	sb.AppendByte(' ')

	return sb
}

// WriteNL ends the current line. It is the one write allowed into the reserved
// tail, because a dropped terminator merges two log lines and breaks the
// one-line-per-call contract the whole package rests on.
func (sb *StackBuffer) WriteNL() *StackBuffer {
	if sb.BufLen < len(sb.Buffer) {
		sb.Buffer[sb.BufLen] = '\n'
		sb.BufLen++
	}

	return sb
}

// Reset clears the active buffer length back to 0, preparing it for the next log
// payload. It also resets the line state: lineStart goes back to true and the
// accumulated Bottom padding is dropped.
func (sb *StackBuffer) Reset() {
	sb.BufLen = 0
	sb.lineStart = true
	sb.maxBottom = 0
	sb.truncated = false
}
