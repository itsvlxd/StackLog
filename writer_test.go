package stacklog

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
)

type failWriter struct{ err error }

func (f *failWriter) Write(p []byte) (int, error) { return 0, f.err }

type shortWriter struct{ n int }

func (s *shortWriter) Write(p []byte) (int, error) { return s.n, nil }

func TestWriteErrorIsReturned(t *testing.T) {
	want := errors.New("disk full")

	l := CreateLogger(Config{})
	l.wr = &failWriter{want}

	if err := l.Error("hi"); !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestShortWriteIsAnError(t *testing.T) {
	l := CreateLogger(Config{})
	l.wr = &shortWriter{1}

	if err := l.Error("hi"); !errors.Is(err, ErrShortWrite) {
		t.Errorf("got %v, want ErrShortWrite", err)
	}
}

func TestSuccessfulWriteReturnsNil(t *testing.T) {
	var buf bytes.Buffer

	l := CreateLogger(Config{})
	l.wr = &buf

	if err := l.Error("hi"); err != nil {
		t.Errorf("got %v, want nil", err)
	}

	// The end-to-end test that was not possible while os.Stdout was hardcoded.
	want := "\x1b[31mERRO\x1b[0m hi\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

func TestBufferIsResetAfterAFailedWrite(t *testing.T) {
	l := CreateLogger(Config{})
	l.wr = &failWriter{errors.New("closed")}

	l.Error("hi")

	if l.Buffer.BufLen != 0 {
		t.Errorf("BufLen = %d after a failed write, want 0", l.Buffer.BufLen)
	}
}

func TestWithWriterNilFallsBackToStdout(t *testing.T) {
	l := CreateLogger(Config{}).WithWriter(nil)

	if l.wr != os.Stdout {
		t.Errorf("nil writer did not fall back to os.Stdout")
	}
}

func TestWithWriterDoesNotChangeTheOriginal(t *testing.T) {
	base := CreateLogger(Config{})
	other := base.WithWriter(io.Discard)

	if base.wr == other.wr {
		t.Error("the copy shares the writer with the original")
	}
}
