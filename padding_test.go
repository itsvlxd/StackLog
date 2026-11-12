package stacklog

import (
	"strings"
	"testing"
)

// Rules asserted here:
//   Top    — only at the start of a line.
//   Bottom — not mid-line. Accumulated as the max across the line, flushed at
//            the end, after the line-ending newline.
//   Left/Right — anywhere.

func TestBottomFlushedAtEndOfLine(t *testing.T) {
	l := CreateLogger(Config{Colors: ColorsOn})
	l.Config.Style.Level[ErrorLevel].p = Padding{Bottom: 2}

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.Buffer.WriteNL()
	l.addBottomPadding()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])
	want := "\x1b[31mERRO\x1b[0m hi\n\n\n"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBottomTakesMaxAcrossTheLine(t *testing.T) {
	l := CreateLogger(Config{})
	l.Config.Style.Level[ErrorLevel] = NewStyle().Padding(Padding{Bottom: 1}).Foreground(5)
	l.Config.Style.Params[ErrorLevel] = ParamsStyle{Value: NewStyle().Padding(Padding{Bottom: 3})}
	l.params = []any{"k", 1}

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.addParams(3)
	l.Buffer.WriteNL()
	l.addBottomPadding()

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if strings.Count(got, "\n") != 4 {
		t.Errorf("expected max(1,3)=3 blank lines after the line, got %d newlines: %q",
			strings.Count(got, "\n"), got)
	}
}

func TestBottomNotEmittedMidLine(t *testing.T) {
	l := CreateLogger(Config{})
	l.Config.Style.Level[ErrorLevel] = NewStyle().Padding(Padding{Bottom: 2}).Foreground(5)
	l.params = []any{"k", 1}

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.addParams(3)

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if strings.Contains(got, "\n") {
		t.Errorf("padding emitted mid-line: %q", got)
	}
}

func TestTopHonouredAtLineStart(t *testing.T) {
	l := CreateLogger(Config{})
	l.Config.Style.Level[ErrorLevel] = NewStyle().Padding(Padding{Top: 2}).Foreground(5)

	l.Buffer.Reset()
	l.addLevelPrefix(3)

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if strings.Count(got, "\n") != 2 {
		t.Errorf("Top=2 at line start produced %d newlines, want 2: %q", strings.Count(got, "\n"), got)
	}
}

func TestTopNotHonouredMidLine(t *testing.T) {
	l := CreateLogger(Config{})
	l.Config.Style.Level[ErrorLevel] = NewStyle().Foreground(5)
	l.Config.Style.Params[ErrorLevel] = ParamsStyle{Param: NewStyle().Padding(Padding{Top: 1})}
	l.params = []any{"k", 1}

	l.Buffer.Reset()
	l.addLevelPrefix(3)
	l.Buffer.WriteString("hi")
	l.addParams(3)

	got := string(l.Buffer.Buffer[:l.Buffer.BufLen])

	if strings.Contains(got, "\n") {
		t.Errorf("mid-line Top emitted a newline: %q", got)
	}
}
