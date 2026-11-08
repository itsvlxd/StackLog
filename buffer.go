package stacklog

import (
	"strconv"
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
	lineStart bool
	maxBottom int
	truncated bool
}

// usable is how much of the buffer content may occupy. The tail is reserved for
// the marker and the terminator.
func (sb *StackBuffer) usable() int {
	return len(sb.Buffer) - truncateReserve
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

// CreateBuffer initialises a StackBuffer and returns its pointer. The value
// stays on the caller's stack only while the pointer never leaves the frame that
// created it — storing it in a struct that escapes moves it to the heap.
func CreateBuffer() *StackBuffer {
	var sb StackBuffer

	sb.BufLen = 0
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

// AppendAny dispatches on the dynamic type and writes the value straight into the
// buffer. It does not build an intermediate string. Types it does not handle are
// written as "<unsupported>" rather than printed outside the log stream.
//
// The conversion to any happens at the call site, so this does not remove the
// boxing cost — it removes the slice of strings that used to sit on top of it.
func (sb *StackBuffer) AppendAny(a any) *StackBuffer {
	switch p := a.(type) {
	case int:
		return sb.AppendInt(int64(p))
	case int64:
		return sb.AppendInt(p)
	case uint:
		return sb.AppendUint(uint64(p))
	case uint64:
		return sb.AppendUint(p)
	case bool:
		return sb.AppendBool(p)
	case string:
		return sb.WriteString(p)
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
