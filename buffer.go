package stacklog

import (
	"strconv"
)

// StackBuffer is a fixed, pre-allocated byte buffer. It never grows. Writes
// copy into the remaining space and truncate silently at 1024 bytes.
//
// lineStart and maxBottom carry the padding rules across a single log line:
// Top applies only at the start of a line, Bottom is never emitted mid-line and
// is flushed as the maximum across the whole line.
type StackBuffer struct {
	Buffer    [1024]byte
	BufLen    int
	lineStart bool
	maxBottom int
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
	out := strconv.AppendInt(num[:0], n, 10)
	len := copy(sb.Buffer[sb.BufLen:], out)
	sb.BufLen += len
	return sb
}

// AppendUint writes the base-10 form of n into the buffer. The worst case for a
// uint64 is 20 digits, so the same 24-byte array is enough.
func (sb *StackBuffer) AppendUint(n uint64) *StackBuffer {
	var num [24]byte
	out := strconv.AppendUint(num[:0], n, 10)
	len := copy(sb.Buffer[sb.BufLen:], out)
	sb.BufLen += len
	return sb
}

// AppendBool writes "true" or "false" into the buffer. Five bytes is exactly the
// worst case, so there is no margin and no allocation.
func (sb *StackBuffer) AppendBool(b bool) *StackBuffer {
	var bool [5]byte
	out := strconv.AppendBool(bool[:0], b)
	len := copy(sb.Buffer[sb.BufLen:], out)
	sb.BufLen += len
	return sb
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

// AppendByte writes a single byte. Unlike Write and WriteString it does not
// check the remaining space, so it panics when the buffer is full.
func (sb *StackBuffer) AppendByte(b byte) *StackBuffer {
	if sb.BufLen < len(sb.Buffer) {
		sb.Buffer[sb.BufLen] = b
		sb.BufLen++
	}

	return sb
}

// Write appends the provided byte slice directly into the current buffer.
// copy truncates at the end of the buffer, so a value longer than the remaining
// space is cut silently.
func (sb *StackBuffer) Write(data []byte) *StackBuffer {
	n := copy(sb.Buffer[sb.BufLen:], data)

	sb.BufLen += n

	return sb
}

// WriteString copies a string into the current buffer, truncating at 1024 bytes.
func (sb *StackBuffer) WriteString(message string) *StackBuffer {
	n := copy(sb.Buffer[sb.BufLen:], message)

	sb.BufLen += n

	return sb
}

// WriteBlank writes a single space.
func (sb *StackBuffer) WriteBlank() *StackBuffer {
	sb.AppendByte(' ')

	return sb
}

// WriteNL ends the current line.
func (sb *StackBuffer) WriteNL() *StackBuffer {
	sb.AppendByte('\n')

	return sb
}

// Reset clears the active buffer length back to 0, preparing it for the next log
// payload. It also resets the line state: lineStart goes back to true and the
// accumulated Bottom padding is dropped.
func (sb *StackBuffer) Reset() {
	sb.BufLen = 0
	sb.lineStart = true
	sb.maxBottom = 0
}
