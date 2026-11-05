package stacklog

import (
	"io"
	"testing"
)

var sink *StackLog

// A test that calls the hot path 1000 times prints 1000 lines. Point the writer
// at io.Discard instead of redirecting os.Stdout, because the logger holds its
// own writer from the moment it is created.
func quiet(l *StackLog) {
	l.wr = io.Discard
}

// Escape analysis is a compile-time property. There is no runtime API for it, so
// you cannot test "this stays on the stack" directly. What you can test is the
// observable consequence: whether a value allocates.
//
// These tests assert the consequence. The escape analysis itself is checked with
// `go build -gcflags=-m`, which is a build flag, not a test, and its output
// changes between compiler versions, so it should not be a CI assertion.

// The hot path. This is the claim the package makes.
func TestHotPathAllocatesNothing(t *testing.T) {
	l := CreateLogger(Config{})
	quiet(l)

	n := testing.AllocsPerRun(1000, func() {
		l.Debug("hi")
	})

	if n != 0 {
		t.Errorf("hot path allocated %.1f per call, want 0", n)
	}
}

// A buffer created inside a function that does not let the pointer escape stays
// on the stack.
func TestStackBufferStaysOnStackWhenNotStored(t *testing.T) {
	n := testing.AllocsPerRun(1000, func() {
		var sb StackBuffer
		sb.AppendInt(123)
	})

	if n != 0 {
		t.Errorf("got %.1f allocs, want 0 (the buffer never left the frame)", n)
	}
}

// The same buffer, stored in a struct that escapes, moves to the heap. This is
// the boundary the project's name claims it can cross — it cannot.
func TestStackBufferMovesToHeapWhenStoredInAnEscapingStruct(t *testing.T) {
	n := testing.AllocsPerRun(1000, func() {
		sink = CreateLogger(Config{})
	})

	if n == 0 {
		t.Error("expected allocations: the struct and the buffer both escape")
	}

	t.Logf("CreateLogger allocates %.1f per call (struct + buffer)", n)
}

// Boxing. This measures the real call site rather than asserting a theory about
// it. Escape analysis reports `any(123) escapes to heap`, but AllocsPerRun
// reports 0 for that same call when the value was already heap-allocated once by
// the closure capture. So boxing is a cost at the point where a value is
// converted, not necessarily a per-call cost inside a loop.
func TestBoxingAtTheCallSite(t *testing.T) {
	l := CreateLogger(Config{})
	quiet(l)

	n := testing.AllocsPerRun(1000, func() {
		sink = l.WithParams("request_id", value())
	})

	t.Logf("WithParams with a runtime value: %.1f allocs per call", n)

	if n == 0 {
		t.Error("the variadic slice escapes, so the call site should allocate")
	}
}

var seed int = 7

func value() int {
	seed = seed*3 + 1
	return seed
}

// WithParams copies the struct. Discarding the result changes the measured
// answer, because escape analysis is per call site. A real caller keeps it, so
// these tests keep it.
func TestWithParamsCopiesAndEscapes(t *testing.T) {
	l := CreateLogger(Config{})
	quiet(l)

	kept := testing.AllocsPerRun(1000, func() {
		sink = l.WithParams("k", 1)
	})

	discarded := testing.AllocsPerRun(1000, func() {
		_ = l.WithParams("k", 1)
	})

	t.Logf("WithParams, result kept: %.1f allocs per call", kept)
	t.Logf("WithParams, result discarded: %.1f allocs per call", discarded)

	if kept == 0 {
		t.Error("the struct copy should escape when the result is kept")
	}
}
