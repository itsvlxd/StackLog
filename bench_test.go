package stacklog

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

var benchOut []byte
var progressOut int

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

// The progress path, measured separately because it makes a different promise: a tick that is not due is
// arithmetic and two comparisons with no syscall and no allocation, and the redraws it skips are the
// whole reason a bar is not 5,000,000 writes.
func BenchmarkProgressTickSkipped(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(discardForBench{})
	p := l.WithProgress(b.N, ProgressOptions{Every: 10 * time.Second})

	for i := 0; i < b.N; i++ {
		p.Tick(1)
	}

	progressOut = p.done
}

// One bar line composed and written, with the bar left colourless - ColorsOff, so what is measured is
// the composition and the write with nothing in it. write is called directly because a bar that is not
// due does not draw, and a benchmark that ticked its way to a draw would mostly be measuring the skipping.
func BenchmarkProgressRedrawNoColors(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(discardForBench{})
	p := l.WithProgress(1000, ProgressOptions{Width: 40})
	p.sl.isTTY = true

	for i := 0; i < b.N; i++ {
		p.done = i % 1000
		p.filled, p.pct = p.filledCells(), p.percent()

		if err := p.write(DebugLevel, "progress", false); err != nil {
			b.Fatal(err)
		}
	}

	progressOut = p.done
}

// The same line with the preset's bar palette in it, because a default bar is a coloured bar now and
// colour has a price here the way it has one everywhere else in this package: three runs of escapes and
// their resets per line, and a bar that can only be 290 cells wide before the fixed buffer says no.
func BenchmarkProgressRedraw(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOn}).WithWriter(discardForBench{})
	p := l.WithProgress(1000, ProgressOptions{Width: 40})
	p.sl.isTTY = true

	for i := 0; i < b.N; i++ {
		p.done = i % 1000
		p.filled, p.pct = p.filledCells(), p.percent()

		if err := p.write(DebugLevel, "progress", false); err != nil {
			b.Fatal(err)
		}
	}

	progressOut = p.done
}

// The result line for a bar whose counts fit in a byte: one ordinary log line, and the allocations are
// the Progress object and its params slice. Both result benchmarks set the counts directly, so this is
// the line alone with no redraws and no milestones in it.
func BenchmarkProgressDone(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(discardForBench{})

	for i := 0; i < b.N; i++ {
		p := l.WithProgress(4, ProgressOptions{})
		p.done = 4

		if err := p.Done("bench finished"); err != nil {
			b.Fatal(err)
		}
	}
}

// The same line with counts a real job would have - 120,000 items and three seconds - because values
// above the runtime's cached range need boxing, and a benchmark that only ever measures small numbers is
// measuring a case that does not ship.
func BenchmarkProgressDoneLargeCounts(b *testing.B) {
	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(discardForBench{})

	for i := 0; i < b.N; i++ {
		p := l.WithProgress(120000, ProgressOptions{})
		p.done = 120000
		p.start = p.start.Add(-3 * time.Second)

		if err := p.Done("bench finished"); err != nil {
			b.Fatal(err)
		}
	}
}

type discardForBench struct{}

func (discardForBench) Write(p []byte) (int, error) { return len(p), nil }
