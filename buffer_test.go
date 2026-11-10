package stacklog

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
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

// Each case here is a rendering decision, not a formatting accident: durations are
// nanoseconds, float32 round-trips at 32 bits, and a value nobody understands is
// refused rather than guessed at.
func TestAppendAnyWritesEachSupportedType(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"int", 12, "12"},
		{"int8", int8(-8), "-8"},
		{"int16", int16(-1600), "-1600"},
		{"int32", int32(-200000), "-200000"},
		{"int64", int64(-9007199254740993), "-9007199254740993"},
		{"uint", uint(12), "12"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float64", 1.44, "1.44"},
		{"float64 with an exponent", 1e300, "1e+300"},
		{"float64 at full precision", 3.141592653589793, "3.141592653589793"},
		{"float32 round-trips at 32 bits", float32(1.1), "1.1"},
		{"time.Duration is nanoseconds", 1500 * time.Microsecond, "1500000"},
		{"bool", true, "true"},
		{"string", "vlad", "vlad"},
		{"error", errors.New("connection reset"), "connection reset"},
		{"rune is an alias for int32", 'A', "65"},
		{"unsupported type", time.Now(), "<unsupported>"},
	}

	for _, c := range cases {
		var sb StackBuffer

		sb.AppendAny(c.value)

		if got := string(sb.Buffer[:sb.BufLen]); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// A nil interface never reaches the error case in a type switch, because it has no
// dynamic type. It needs its own case, and this is what pins it: the guard inside
// AppendError cannot cover the path a caller actually takes.
func TestNilValueWritesNil(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"nil any", nil, "<nil>"},
		{"nil error interface", error(nil), "<nil>"},
	}

	for _, c := range cases {
		var sb StackBuffer

		sb.AppendAny(c.value)

		if got := string(sb.Buffer[:sb.BufLen]); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	var direct StackBuffer

	direct.AppendError(nil)

	if got := string(direct.Buffer[:direct.BufLen]); got != "<nil>" {
		t.Errorf("AppendError(nil) wrote %q, want \"<nil>\"", got)
	}
}

// The scratch arrays on the value path are sized for the worst case. A smaller array
// grows the slice, and a grown slice is on the heap. This covers the types that were
// added, not only the one that was here first - an [10]byte float buffer allocated
// once for every nine values and this test is what would have caught it.
func TestValuePathAllocatesNothing(t *testing.T) {
	values := []any{
		1.44,
		3.141592653589793,
		1e300,
		float32(1.1),
		1500 * time.Microsecond,
		errors.New("connection reset"),
		"vlad",
		true,
		uint64(18446744073709551615),
	}

	var sb StackBuffer

	for _, v := range values {
		allocs := testing.AllocsPerRun(200, func() {
			sb.BufLen = 0
			sb.AppendAny(v)
		})

		if allocs != 0 {
			t.Errorf("%T allocated %.1f times per call, want 0", v, allocs)
		}
	}
}
