package stacklog

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// countingWriter records how many Write calls reach it, which is the only way to test the
// one-write-per-call contract for a payload that spans several lines.
type countingWriter struct {
	calls int
	buf   bytes.Buffer
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.calls++

	return c.buf.Write(p)
}

type lockedWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.Write(p)
}

// Plain is the mode with nothing in it. No level, no label, no prefix, no timestamp, no
// colour, no padding - only what the caller handed over, plus the terminator.
func TestPlainWritesExactlyWhatItIsGiven(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{}).WithWriter(&out)

	if err := l.Plain("stacklog"); err != nil {
		t.Fatal(err)
	}

	if out.String() != "stacklog\n" {
		t.Errorf("wrote %q, want %q", out.String(), "stacklog\n")
	}
	if strings.ContainsRune(out.String(), 0x1b) {
		t.Errorf("Plain emitted an escape byte: %q", out.String())
	}
}

// Style, prefix and padding are all inert here. A logger with a coloured level, a left-padded
// label and a prefix set still writes the payload alone - which is the point of the mode rather
// than an accident of it.
func TestPlainIgnoresStylePrefixAndPadding(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Timestamp: true}).WithWriter(&out).WithPrefix("http")

	l.Config.Style.Level[InfoLevel] = NewStyle().
		Foreground("#D75F5F").
		Padding(Padding{Top: 1, Bottom: 1, Left: 2, Right: 2})

	if err := l.Plain("a banner is not a log line"); err != nil {
		t.Fatal(err)
	}

	if out.String() != "a banner is not a log line\n" {
		t.Errorf("wrote %q, want the payload alone", out.String())
	}
	if n := strings.Count(out.String(), "\n"); n != 1 {
		t.Errorf("%d newlines: padding leaked into a plain write: %q", n, out.String())
	}
}

// MinLevel filters log levels. Plain is not a log level, so a logger set to warn still prints
// its banner. Asserted because "everything below WarnLevel is dropped" reads as if it covers
// every method on the type.
func TestPlainIsNotFilteredByMinLevel(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{MinLevel: WarnLevel}).WithWriter(&out)

	l.Debug("dropped")

	if err := l.Plain("kept"); err != nil {
		t.Fatal(err)
	}

	if out.String() != "kept\n" {
		t.Errorf("wrote %q, want the banner to survive the filter", out.String())
	}
}

// On a JSON logger, a plain write is not wrapped. That is the deliberate choice - a banner as a
// JSON string is useless to the person it was printed for - and it is asserted here so nobody
// discovers it from a parser error at 2am.
func TestPlainStaysPlainOnAJSONLogger(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Format: FormatJSON}).WithWriter(&out)

	if err := l.Plain("S T A C K L O G"); err != nil {
		t.Fatal(err)
	}

	if out.String() != "S T A C K L O G\n" {
		t.Errorf("wrote %q, want the payload untouched", out.String())
	}

	if json.Valid(out.Bytes()) {
		t.Error("a plain write should not become JSON")
	}

	// And the JSON log line on the same logger is still JSON, so the stream is the banner plus
	// objects - which is what the docs tell a caller to expect.
	if err := l.Error("upstream 502"); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2: %q", len(lines), out.String())
	}
	if json.Valid([]byte(lines[0])) {
		t.Error("the banner parsed as JSON")
	}
	if !json.Valid([]byte(lines[1])) {
		t.Errorf("the log line did not parse as JSON: %q", lines[1])
	}
}

// A multi-line payload is one Write, not one Write per line. That is what makes a framed ASCII
// wordmark usable, and the fixed buffer is what makes it a contract: the whole banner still has
// to fit in the content area or it gets cut.
func TestPlainWritesAMultiLinePayloadInOneCall(t *testing.T) {
	w := &countingWriter{}

	l := CreateLogger(Config{}).WithWriter(w)

	// Rows of the wordmark the showcase actually prints. Block characters are three bytes each and
	// two of them - ▀ and ▄ - are half-cells, so this is the payload Plain must carry without
	// touching: no escaping, no splitting, no reordering.
	logo := "▄██████ ███████\n███▄  ▀▀  ▀███▀▀▀\n▀███████▄  ███"

	if err := l.Plain(logo); err != nil {
		t.Fatal(err)
	}

	if w.calls != 1 {
		t.Errorf("%d Write calls for one Plain call, want 1", w.calls)
	}

	got := w.buf.String()

	if !strings.HasPrefix(got, "▄███") || !strings.HasSuffix(got, "███\n") {
		t.Errorf("the payload was altered: %q", got)
	}
	// Three logo rows carry two newlines between them; Plain adds the terminator.
	if n := strings.Count(got, "\n"); n != 3 {
		t.Errorf("%d newlines, want 3 (two inside the payload plus the terminator)", n)
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Error("block characters were escaped")
	}
}

// The truncation rule reaches Plain too. A payload past the content area is cut and marked -
// which for a logo means a broken frame, and for the caller means knowing it happened rather
// than finding out from the output.
func TestPlainIsTruncatedByTheSameRule(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out)

	if err := l.Plain(strings.Repeat("█", 1100)); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if len(got) != 1024 {
		t.Errorf("flushed %d bytes, want 1024", len(got))
	}
	if !strings.HasSuffix(got, "...\n") {
		t.Errorf("a cut plain payload does not end with the marker: %q", got[len(got)-8:])
	}
}

// The claim the goroutines showcase makes: a derived logger per goroutine is the safe pattern,
// and the shared writer receives whole lines. Run with -race, which is the part that actually
// tests it - a torn line would show up as a bad count, but the race is on the buffer and the
// race detector is what finds it.
func TestDerivedLoggersPerGoroutineWriteWholeLines(t *testing.T) {
	const perGoroutine = 200

	w := &lockedWriter{}

	base := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(w)

	names := []string{"db", "http", "cache", "queue", "sync", "mail", "cdn", "api"}

	var wg sync.WaitGroup

	for _, name := range names {
		wg.Add(1)

		go func(name string) {
			defer wg.Done()

			l := base.WithPrefix(name)

			for i := 0; i < perGoroutine; i++ {
				l.Info("line written by " + name)
			}
		}(name)
	}

	wg.Wait()

	out := w.buf.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	if len(lines) != len(names)*perGoroutine {
		t.Fatalf("%d lines written, want %d", len(lines), len(names)*perGoroutine)
	}

	// A torn line is a line that does not match the shape every whole line has.
	shape := regexp.MustCompile(`^INFO (\w+): line written by (\w+)$`)

	perPrefix := map[string]int{}

	for _, line := range lines {
		groups := shape.FindStringSubmatch(line)
		if groups == nil {
			t.Fatalf("a line arrived incomplete: %q", line)
		}

		// The prefix on the line is the logger that wrote it, so a line written by db under the
		// http logger would be caught here as well as a line torn in half.
		if groups[1] != groups[2] {
			t.Fatalf("a line crossed loggers: %q", line)
		}

		perPrefix[groups[1]]++
	}

	for _, name := range names {
		if perPrefix[name] != perGoroutine {
			t.Errorf("%s wrote %d lines, want %d", name, perPrefix[name], perGoroutine)
		}
	}
}

// Plain keeps the allocation claim. The payload is copied into the fixed buffer, the terminator
// is the last byte of the reserved tail, and nothing on this path is boxed.
func TestPlainPathAllocatesNothing(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{}).WithWriter(&out).WithPrefix("http").WithParams("user", "vlad")

	got := testing.AllocsPerRun(200, func() {
		l.Plain("service started, listening on 0.0.0.0:8080")
	})

	if got != 0 {
		t.Errorf("the plain path allocates %.0f times per call, want 0", got)
	}
}

// A plain write reports a failed write the way every other method does, and it resets the
// buffer, so one failure does not corrupt the following line.
func TestPlainReportsWriteErrors(t *testing.T) {
	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(failingWriter{})

	if err := l.Plain("payload"); err == nil {
		t.Error("a failing writer should surface as an error from Plain")
	}

	var out bytes.Buffer

	recovered := l.WithWriter(&out)

	if err := recovered.Plain("next line"); err != nil {
		t.Fatal(err)
	}

	if out.String() != "next line\n" {
		t.Errorf("the line after a failure wrote %q, want it intact", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }
