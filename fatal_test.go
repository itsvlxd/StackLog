package stacklog

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fatal calls os.Exit, so nothing inside this process can observe what it actually
// wrote. The test re-runs itself: the child logs through a buffered writer and calls
// Fatal, and the parent reads the file the child was pointed at.
//
// The child never reaches its own deferred Close, because os.Exit skips deferred
// functions — which is the trap. Without the flush in Fatal, a 256 KB buffer holding
// two short lines writes nothing at all, and the line that explains the crash is lost
// with them. That is the data loss this guards.
func TestFatalFlushesABufferedWriter(t *testing.T) {
	if os.Getenv("STACKLOG_FATAL_CHILD") == "1" {
		f, err := os.Create(os.Getenv("STACKLOG_FATAL_FILE"))
		if err != nil {
			t.Fatal(err)
		}

		defer f.Close()

		bw := bufio.NewWriterSize(f, 256*1024)

		l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(bw)

		l.Info("written before the crash")
		l.Fatal("shutting down, the port is still in use")

		return
	}

	path := filepath.Join(t.TempDir(), "fatal.log")

	cmd := exec.Command(os.Args[0], "-test.run=TestFatalFlushesABufferedWriter")
	cmd.Env = append(os.Environ(), "STACKLOG_FATAL_CHILD=1", "STACKLOG_FATAL_FILE="+path)

	if err := cmd.Run(); err == nil {
		t.Fatal("the child was expected to exit with a status, got none")
	} else if code := cmd.ProcessState.ExitCode(); code != 1 {
		t.Errorf("child exited with %d, want 1 from os.Exit(1)", code)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got := string(data)

	if !strings.Contains(got, "written before the crash") {
		t.Errorf("the buffered line that preceded Fatal was lost: %q", got)
	}

	if !strings.Contains(got, "shutting down, the port is still in use") {
		t.Errorf("the Fatal line itself was lost: %q", got)
	}
}

// A writer with no Flush is the ordinary case and must keep working: nothing is
// asked of it, and the exit still happens.
func TestFatalOnAWriterThatCannotFlush(t *testing.T) {
	if os.Getenv("STACKLOG_FATAL_CHILD") == "2" {
		l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&failWriter{errors.New("disk full")})

		l.Fatal("no flush available")

		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatalOnAWriterThatCannotFlush")
	cmd.Env = append(os.Environ(), "STACKLOG_FATAL_CHILD=2")

	combined, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("the child was expected to exit with a status, got none")
	}

	if !strings.Contains(string(combined), "stacklog: write failed") {
		t.Errorf("a failing write was not reported: %q", string(combined))
	}
}
