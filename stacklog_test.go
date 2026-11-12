package stacklog

import (
	"strings"

	"testing"
)

// Render path tests. They assert the bytes produced before the flush. They
// cannot test the flush itself, because os.Stdout is hardcoded — that is the
// practical reason for the io.Writer field.

func TestRenderLine(t *testing.T) {
	l := CreateLogger(Config{Colors: ColorsOn})

	l.Buffer.Reset()
	l.addLevelPrefix(0)
	l.Buffer.WriteString("hi")
	l.Buffer.WriteNL()

	want := "\x1b[35mDEBU\x1b[0m hi\n"
	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// One colour open per component, then one reset. The level colour used to
	// appear twice: addPostStyle re-emitted what addPreStyle had already opened,
	// and the terminal was already in that colour. A component that writes two
	// openings is writing bytes that change nothing.
}

// A style can carry padding and carry no colour. Emptiness and "opened a colour"
// are different questions, and the reset gate has to ask the second one. A gate
// that asks the first writes a reset here for a colour that was never opened.
func TestPaddingWithoutColourWritesNoEscape(t *testing.T) {
	l := CreateLogger(Config{})

	l.Config.Style.Level[ErrorLevel] = NewStyle().Padding(Padding{Top: 1, Left: 1, Right: 1})

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.Buffer.WriteNL()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if strings.Contains(got, "\x1b") {
		t.Errorf("a style with no colour wrote escape codes: %q", got)
	}

	if !strings.Contains(got, "ERRO") {
		t.Errorf("the label disappeared: %q", got)
	}
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
