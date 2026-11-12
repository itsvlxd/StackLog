package stacklog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The policy tests are all about the negative half: "this writer is not a terminal, so nothing
// coloured arrives". That is the half that used to break people, and it is the half that can be
// asserted in a test binary whose own stdout is whatever the test runner decided.
//
// The positive half - a real terminal, colour arrives - was verified by hand under script(1), which
// allocates a pty, and it is deliberately not a permanent test. A test whose verdict depends on
// whether the machine running it has a terminal attached fails in CI for reasons that have nothing
// to do with the code.

func containsEscape(b []byte) bool {
	return bytes.ContainsRune(b, 0x1b)
}

func TestColorsAutoLeavesABufferWriterUncoloured(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{}).WithWriter(&out)

	if err := l.Error("no terminal, no escapes"); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "ERRO no terminal, no escapes\n" {
		t.Errorf("got %q", got)
	}
}

func TestColorsAutoLeavesARegularFileUncoloured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	l := CreateLogger(Config{}).WithWriter(f)

	if err := l.Error("a log file is not a terminal"); err != nil {
		f.Close()
		t.Fatal(err)
	}

	// Closed before reading, so the file on disk is the whole write rather than a partial one.
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if containsEscape(data) {
		t.Errorf("a log file received escape bytes: %q", data)
	}

	if string(data) != "ERRO a log file is not a terminal\n" {
		t.Errorf("got %q", data)
	}
}

// /dev/null is a character device, which is exactly why the rule is "standard output or standard
// error, attached to a character device" rather than "is a character device". This test is the fd
// half of that rule earning its keep.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o666)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if stat.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("%s is not a character device here, so this test proves nothing", os.DevNull)
	}

	if isTerminal(f) {
		t.Errorf("%s was treated as a terminal", os.DevNull)
	}

	var out bytes.Buffer

	l := CreateLogger(Config{}).WithWriter(&out)

	if err := l.Error("colour is a decision, not a bit"); err != nil {
		t.Fatal(err)
	}

	if containsEscape(out.Bytes()) {
		t.Errorf("the default config wrote escapes into a non-terminal writer: %q", out.String())
	}
}

func TestColorsOnColoursANonTerminalWriter(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Colors: ColorsOn}).WithWriter(&out)

	if err := l.Error("explicit ask, explicit answer"); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "\x1b[31mERRO\x1b[0m explicit ask, explicit answer\n" {
		t.Errorf("got %q", got)
	}
}

func TestColorsOffSuppressesColourTheStyleWouldOtherwiseWrite(t *testing.T) {
	var out bytes.Buffer

	// The default preset already paints ERRO red, so this config would write escapes without
	// ColorsOff - there is nothing extra to force.
	l := CreateLogger(Config{
		Colors: ColorsOff,
	}).WithWriter(&out)

	if err := l.Error("off means off"); err != nil {
		t.Fatal(err)
	}

	if containsEscape(out.Bytes()) {
		t.Errorf("got %q, want no escape bytes", out.String())
	}
}

// NoColors is the kill switch it has always been. It outranks ColorsOn and it can never turn colour
// on, which is the promise the pre-existing colours_test.go and README examples rely on.
func TestNoColorsOutranksColorsOn(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{
		Colors: ColorsOn,
		Style:  Style{NoColors: true},
	}).WithWriter(&out)

	if err := l.Error("kill switch"); err != nil {
		t.Fatal(err)
	}

	if containsEscape(out.Bytes()) {
		t.Errorf("got %q, want no escape bytes", out.String())
	}
}

func TestUnknownColorsNameBehavesAsAuto(t *testing.T) {
	for _, name := range []string{"", "always", "Auto", "when-terminal"} {
		var out bytes.Buffer

		l := CreateLogger(Config{Colors: name}).WithWriter(&out)

		if err := l.Error("unrecognised is not an error"); err != nil {
			t.Fatal(err)
		}

		if containsEscape(out.Bytes()) {
			t.Errorf("Colors %q wrote escapes to a non-terminal writer: %q", name, out.String())
		}
	}
}

// The decision is made when the writer is known. Mutating Config.Colors afterwards is the shape of
// call this package documents as legal for style components, and it does not change the answer -
// which is stated here as a rule, not left as a surprise.
func TestColorsMutatedAfterTheWriterIsKnownHasNoEffect(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{}).WithWriter(&out)

	l.Config.Colors = ColorsOn

	if err := l.Error("too late"); err != nil {
		t.Fatal(err)
	}

	if containsEscape(out.Bytes()) {
		t.Errorf("Config.Colors was changed after WithWriter and the logger still obeyed it: %q", out.String())
	}
}

// WithWriter is the re-resolution point: a Config that already says ColorsOn, handed to a new
// writer, gets colour on that writer too.
func TestWithWriterResolvesTheDecisionAgain(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{}).WithWriter(&bytes.Buffer{})

	l.Config.Colors = ColorsOn

	coloured := l.WithWriter(&out)

	if err := coloured.Error("a new writer, a new answer"); err != nil {
		t.Fatal(err)
	}

	if !containsEscape(out.Bytes()) {
		t.Errorf("WithWriter did not re-resolve Colors: %q", out.String())
	}
}

// A derived logger inherits its parent's writer, so it inherits the verdict that writer produced.
// Both directions are asserted: an inherited "no", and an inherited "yes" that came from ColorsOn
// rather than from a terminal.
func TestDerivedLoggerInheritsTheVerdictOfItsParent(t *testing.T) {
	var out bytes.Buffer

	auto := CreateLogger(Config{}).WithWriter(&bytes.Buffer{})
	autoChild := auto.WithPrefix("http").WithWriter(&out)

	if autoChild.emitColor {
		t.Error("a derived logger turned colour on for a writer its parent had already refused")
	}

	if err := autoChild.Error("inherited refusal"); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "ERRO http: inherited refusal\n" {
		t.Errorf("got %q", got)
	}

	out.Reset()

	on := CreateLogger(Config{Colors: ColorsOn}).WithWriter(&bytes.Buffer{})
	onChild := on.WithPrefix("http").WithWriter(&out)

	if err := onChild.Error("inherited permission"); err != nil {
		t.Fatal(err)
	}

	if !containsEscape(out.Bytes()) {
		t.Errorf("ColorsOn did not survive derivation: %q", out.String())
	}
}

// A JSON logger states its colour decision in both vocabularies, because the README promises
// NoColors reads back as true there and ColorsOff is where the new policy puts the same answer.
func TestJSONLoggerStatesItsColourDecision(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Format: FormatJSON}).WithWriter(&out)

	if l.Config.Colors != ColorsOff {
		t.Errorf("Colors reads %q, want %q", l.Config.Colors, ColorsOff)
	}

	if !l.Config.Style.NoColors {
		t.Error("a JSON logger no longer reads back with NoColors set")
	}

	if err := l.Error("one object"); err != nil {
		t.Fatal(err)
	}

	if containsEscape(out.Bytes()) {
		t.Errorf("JSON output carries escape bytes: %q", out.String())
	}
}

func TestIsTerminalOnWriterKinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.log")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var buf bytes.Buffer

	cases := []struct {
		name string
		w    io.Writer
		want bool
	}{
		{"regular file", f, false},
		{"bytes.Buffer", &buf, false},
		{"io.Discard", io.Discard, false},
	}

	for _, c := range cases {
		if got := isTerminal(c.w); got != c.want {
			t.Errorf("%s: isTerminal = %v, want %v", c.name, got, c.want)
		}
	}
}
