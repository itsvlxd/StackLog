package stacklog

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteStringAndTruncation(t *testing.T) {
	var sb StackBuffer
	sb.WriteString("hello")
	if string(sb.Buffer[:sb.BufLen]) != "hello" {
		t.Errorf("got %q", string(sb.Buffer[:sb.BufLen]))
	}

	// Content stops at the usable limit, not at the end of the array. The tail is
	// reserved for the marker and the terminator, and a cut write marks the line.
	sb.Reset()
	sb.BufLen = 1020
	sb.WriteString("1234567890")
	if sb.BufLen != 1020 {
		t.Errorf("BufLen = %d, want %d - content must not enter the reserved tail", sb.BufLen, 1020)
	}
	if !sb.truncated {
		t.Error("a cut write must mark the line")
	}
}

// A truncated line must still carry its terminator. Before the reserve was added,
// a full buffer dropped the newline and two log calls merged into one run of text.
func TestTruncatedLineKeepsItsTerminator(t *testing.T) {
	l := CreateLogger(Config{Style: Style{NoColors: true}})

	var buf bytes.Buffer
	l = l.WithWriter(&buf)

	l.Info(strings.Repeat("A", 1100))
	l.Info("second line")

	out := buf.String()

	if got := strings.Count(out, "\n"); got != 2 {
		t.Errorf("newline count = %d, want 2: the lines merged. tail %q", got, out[1015:])
	}

	if !strings.Contains(out, "...\nINFO second line\n") {
		t.Errorf("the truncated line did not carry the marker before the terminator. tail %q", out[1015:])
	}

	if len(out) != 1024+17 {
		t.Errorf("flushed %d bytes, want %d", len(out), 1024+17)
	}
}

func TestAppendIntTruncatesAtBufferEnd(t *testing.T) {
	var sb StackBuffer
	sb.BufLen = 1020
	sb.AppendInt(9223372036854775807) // 19 digits, no room in the content area
	if sb.BufLen != 1020 {
		t.Errorf("BufLen = %d, want 1020", sb.BufLen)
	}
	if !sb.truncated {
		t.Error("a cut write must mark the line")
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
