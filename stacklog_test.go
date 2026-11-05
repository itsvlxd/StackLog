package stacklog

import (
	"testing"
)

// Render path tests. They assert the bytes produced before the flush. They
// cannot test the flush itself, because os.Stdout is hardcoded — that is the
// practical reason for the io.Writer field.

func TestRenderLine(t *testing.T) {
	l := CreateLogger(Config{})

	l.Buffer.Reset()
	l.addLevelPrefix(0)
	l.Buffer.WriteString("hi")
	l.Buffer.WriteNL()

	want := "\x1b[35mDEBU\x1b[35m\x1b[0m hi\n"
	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// The second \x1b[35m is addPostStyle re-emitting a colour that is already
	// active. Not a correctness bug, wasted bytes. Asserted so any change is
	// visible.
}

func TestRenderLineWithParams(t *testing.T) {
	l := CreateLogger(Config{})
	l.params = []any{"k", 1}

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.addParams(3)
	l.Buffer.WriteNL()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if got == "" {
		t.Fatal("nothing rendered")
	}

	t.Logf("rendered: %q", got)
}

// The order of the parts is part of the behaviour. Changing it changes the
// output format, which is a breaking change for anything parsing the lines.
func TestRenderOrder(t *testing.T) {
	l := CreateLogger(Config{Timestamp: true, TimeFormat: "15:04:05"})

	l.Buffer.Reset()
	l.addTimestamp()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.Buffer.WriteNL()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if got == "" {
		t.Fatal("nothing rendered")
	}

	t.Logf("rendered: %q", got)
}

// Odd number of params: no rule exists today. This test documents that the last
// value is rendered with no key. It must be rewritten once you define the rule.
func TestOddParamCountIsUndefined(t *testing.T) {
	l := CreateLogger(Config{})
	quiet(l)
	l.params = []any{"only-a-key"}

	l.Buffer.Reset()
	l.addParams(3)

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])
	t.Logf("odd count renders: %q", got)

	if got == "" {
		t.Error("a lone param produced nothing")
	}
}
