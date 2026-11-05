package stacklog

import (
	"testing"
)

func TestWriteStringAndTruncation(t *testing.T) {
	var sb StackBuffer
	sb.WriteString("hello")
	if string(sb.Buffer[:sb.BufLen]) != "hello" {
		t.Errorf("got %q", string(sb.Buffer[:sb.BufLen]))
	}

	// Fill to the end, then write past it. copy() must truncate, not overrun.
	sb.Reset()
	sb.BufLen = 1020
	sb.WriteString("1234567890")
	if sb.BufLen != 1024 {
		t.Errorf("BufLen = %d, want 1024 (copy must truncate)", sb.BufLen)
	}
}

func TestAppendIntTruncatesAtBufferEnd(t *testing.T) {
	var sb StackBuffer
	sb.BufLen = 1020
	sb.AppendInt(9223372036854775807) // 19 digits, 4 bytes of room
	if sb.BufLen != 1024 {
		t.Errorf("BufLen = %d, want 1024", sb.BufLen)
	}
}

// The claim this package makes. This is the test that protects it.
func TestBufferPathAllocatesNothing(t *testing.T) {
	var sb StackBuffer
	v := any(123)

	allocs := testing.AllocsPerRun(1000, func() {
		sb.BufLen = 0
		sb.AppendAny(v)
	})

	if allocs != 0 {
		t.Errorf("buffer path allocated %.1f times per call, want 0", allocs)
	}
}
