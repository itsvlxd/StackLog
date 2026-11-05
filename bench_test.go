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
	l := CreateLogger(Config{})
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
