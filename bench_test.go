package stacklog

import (
	"io"
	"log/slog"
	"testing"
)

var benchOut []byte

// Render path only — no flush. This is the fair measurement of your code.
// You cannot benchmark the full call against log/slog yet, because os.Stdout
// is hardcoded: a benchmark that flushes to stdout measures the terminal or the
// pipe, not the logger. That is the practical reason for the io.Writer field.
func BenchmarkRenderPath(b *testing.B) {
	// ColorsOn because this benchmark measures the coloured render path. Under ColorsAuto a logger
	// writing to a plain buffer or to io.Discard writes no escapes at all, which would make this
	// benchmark measure the uncoloured line and report it as the coloured one.
	l := CreateLogger(Config{Colors: ColorsOn})
	l.params = []any{"k", 1}

	for i := 0; i < b.N; i++ {
		l.Buffer.Reset()
		l.addLevelPrefix(3)
		l.Buffer.WriteString("hi")
		l.addParams(3)
		l.Buffer.WriteNL()
	}

	benchOut = l.Buffer.Buffer[:l.Buffer.BufLen] // keep the work, do not discard it
}

// Comparison against log/slog writing to io.Discard. Not directly comparable to
// the benchmark above until your logger can write to a writer.
func BenchmarkSlogToDiscard(b *testing.B) {
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	for i := 0; i < b.N; i++ {
		l.Error("hi", "k", 1)
	}
}

// The same call in both formats, to io.Discard, so what is being measured is the builder
// rather than the terminal. Escaping is the extra work JSON mode takes on, and a caller
// choosing a format should be able to see what it costs.
//
// ColorsOn on the text pair: io.Discard is not a terminal, so the default policy would suppress
// the escapes these benchmarks exist to price.
func BenchmarkErrorToDiscardText(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOn}).WithWriter(io.Discard).WithParams("k", 1)

	for i := 0; i < b.N; i++ {
		l.Error("hi")
	}
}

func BenchmarkErrorToDiscardJSON(b *testing.B) {
	l := CreateLogger(Config{Format: FormatJSON}).WithWriter(io.Discard).WithParams("k", 1)

	for i := 0; i < b.N; i++ {
		l.Error("hi")
	}
}

// The colour budget, and the number that tells a reader what a style actually costs. The
// same line with NoColors set lands within noise of the same line in JSON mode: what a
// coloured line costs more than a plain one is the escapes, not the JSON machinery.
//
// Guarded here because "this package is fast" is only a claim if the part that makes it
// slower is measured.
func BenchmarkErrorToDiscardTextNoColors(b *testing.B) {
	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(io.Discard).WithParams("k", 1)

	for i := 0; i < b.N; i++ {
		l.Error("hi")
	}
}

// The realistic line, because a two-character message measures the wrong thing. At "hi" there
// is no run of ordinary text to copy in bulk, so JSON mode looks as cheap as plain text and the
// escaping never shows up. With the message length the package is actually used at, JSON costs
// what escaping costs - which is the number the fixed-buffer escape writer has to stay sharp on.
func BenchmarkErrorToDiscardTextLong(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOn}).WithWriter(io.Discard).WithParams("user", "vlad", "ms", 12.4)

	for i := 0; i < b.N; i++ {
		l.Error("upstream returned 502 for user:42")
	}
}

func BenchmarkErrorToDiscardJSONLong(b *testing.B) {
	l := CreateLogger(Config{Format: FormatJSON}).WithWriter(io.Discard).WithParams("user", "vlad", "ms", 12.4)

	for i := 0; i < b.N; i++ {
		l.Error("upstream returned 502 for user:42")
	}
}

// The pair for log/slog at the message length it is actually used at. The short benchmark
// measures the machinery around a two-character string; a real log line carries a sentence and
// a couple of attributes, and that is where the difference in design shows up.
func BenchmarkSlogTextLong(b *testing.B) {
	l := slog.New(slog.NewTextHandler(io.Discard, nil))

	for i := 0; i < b.N; i++ {
		l.Error("upstream returned 502 for user:42", "user", "vlad", "ms", 12.4)
	}
}

func BenchmarkSlogJSONLong(b *testing.B) {
	l := slog.New(slog.NewJSONHandler(io.Discard, nil))

	for i := 0; i < b.N; i++ {
		l.Error("upstream returned 502 for user:42", "user", "vlad", "ms", 12.4)
	}
}
