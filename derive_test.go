package stacklog

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
)

// A derived logger must own its buffer. WithParams, WithPrefix and WithWriter
// copy the struct by value, and a plain copy carries the *StackBuffer pointer:
// every logger derived from one base then builds its line into the same
// 1024-byte array, sharing BufLen, lineStart and the truncation flag.
func TestDerivedLoggersGetTheirOwnBuffer(t *testing.T) {
	base := CreateLogger(Config{})

	kv := base.WithParams("k", 1)

	cases := []struct {
		name string
		buf  *StackBuffer
	}{
		{"base", base.Buffer},
		{"WithParams", kv.Buffer},
		{"WithPrefix", base.WithPrefix("svc").Buffer},
		{"WithWriter", base.WithWriter(io.Discard).Buffer},
		{"WithParams then WithPrefix", kv.WithPrefix("svc").Buffer},
		{"WithWriter then WithPrefix", base.WithWriter(io.Discard).WithPrefix("svc").Buffer},
	}

	owner := make(map[*StackBuffer]string, len(cases))

	for _, c := range cases {
		if first, seen := owner[c.buf]; seen {
			t.Errorf("%s shares its buffer with %s", c.name, first)
		}

		owner[c.buf] = c.name
	}
}

// A fresh buffer has to be in the same state Reset() leaves a used one in. If it
// is not, a derived logger renders differently from a logger that has already
// written a line: lineStart decides whether Top padding is emitted at all, and
// maxBottom is carried across the line before it is flushed.
func TestDerivedBufferStartsWhereResetLeavesOne(t *testing.T) {
	used := CreateLogger(Config{})
	quiet(used)

	if err := used.Info("hi"); err != nil {
		t.Fatal(err)
	}

	fresh := used.WithPrefix("svc").Buffer

	if fresh.BufLen != used.Buffer.BufLen {
		t.Errorf("BufLen %d, want %d", fresh.BufLen, used.Buffer.BufLen)
	}

	if fresh.lineStart != used.Buffer.lineStart {
		t.Errorf("lineStart %v, want %v", fresh.lineStart, used.Buffer.lineStart)
	}

	if fresh.maxBottom != used.Buffer.maxBottom {
		t.Errorf("maxBottom %d, want %d", fresh.maxBottom, used.Buffer.maxBottom)
	}

	if fresh.truncated != used.Buffer.truncated {
		t.Errorf("truncated %v, want %v", fresh.truncated, used.Buffer.truncated)
	}
}

// Each goroutine derives its own logger and writes to its own writer. Before the
// buffers were separated, every logger in this test held the base buffer, so two
// goroutines built lines into one array at the same time.
//
// This one needs -race to be reliable: the corruption it describes is timing
// dependent, so run without the flag and it catches the bug by luck. The buffer
// identity test above is the deterministic guard; this is the one that shows what
// the sharing actually does.
func TestDerivedLoggersWriteTheirOwnLines(t *testing.T) {
	base := CreateLogger(Config{Style: Style{NoColors: true}})

	const workers = 8
	const lines = 200

	out := make([]string, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			var b bytes.Buffer

			l := base.WithPrefix("svc").WithWriter(&b)

			for j := 0; j < lines; j++ {
				l.Info("line")
			}

			out[id] = b.String()
		}(i)
	}

	wg.Wait()

	want := "INFO svc: line\n"

	for i, got := range out {
		if n := strings.Count(got, "\n"); n != lines {
			t.Errorf("logger %d wrote %d terminators, want %d", i, n, lines)

			continue
		}

		if got != strings.Repeat(want, lines) {
			t.Errorf("logger %d wrote something that is not its own line: %q", i, got[:80])
		}
	}
}
