package stacklog

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A filter writes its own level and everything more severe, because LogLevel
// ascends in severity. Dropping warn while keeping info would be the wrong feature.
func TestMinLevelWritesItsOwnLevelAndEverythingMoreSevere(t *testing.T) {
	calls := []struct {
		name  string
		run   func(*StackLog) error
		msg   string
		level LogLevel
	}{
		{"debug", func(l *StackLog) error { return l.Debug("m-debug") }, "m-debug", DebugLevel},
		{"info", func(l *StackLog) error { return l.Info("m-info") }, "m-info", InfoLevel},
		{"warn", func(l *StackLog) error { return l.Warn("m-warn") }, "m-warn", WarnLevel},
		{"error", func(l *StackLog) error { return l.Error("m-error") }, "m-error", ErrorLevel},
	}

	for _, min := range []LogLevel{DebugLevel, InfoLevel, WarnLevel, ErrorLevel} {
		var buf bytes.Buffer

		l := CreateLogger(Config{MinLevel: min, Style: Style{NoColors: true}}).WithWriter(&buf)

		for _, c := range calls {
			if err := c.run(l); err != nil {
				t.Fatal(err)
			}
		}

		got := buf.String()

		for _, c := range calls {
			want := c.level >= min

			if written := strings.Contains(got, c.msg); written != want {
				t.Errorf("MinLevel=%v: %s written=%v, want %v. Output %q", min, c.name, written, want, got)
			}
		}
	}
}

// The field was added after the fact. Its zero value is DebugLevel, so every Config
// written before it existed keeps writing every level, unchanged.
func TestZeroValueMinLevelWritesEverything(t *testing.T) {
	var buf bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&buf)

	l.Debug("m-debug")
	l.Info("m-info")
	l.Warn("m-warn")
	l.Error("m-error")

	got := buf.String()

	for _, msg := range []string{"m-debug", "m-info", "m-warn", "m-error"} {
		if !strings.Contains(got, msg) {
			t.Errorf("%s missing from an unset MinLevel: %q", msg, got)
		}
	}
}

// derive copies Config by value, so a filter set on the base reaches everything
// derived from it. The copy carries both the filter and the writer.
func TestDerivedLoggersInheritTheFilter(t *testing.T) {
	var buf bytes.Buffer

	base := CreateLogger(Config{MinLevel: WarnLevel, Style: Style{NoColors: true}}).WithWriter(&buf)

	kv := base.WithParams("k", 1)
	prefixed := base.WithPrefix("svc")

	kv.Debug("dropped-kv")
	prefixed.Info("dropped-prefix")
	kv.Warn("kept-kv")
	prefixed.Error("kept-prefix")

	got := buf.String()

	for _, msg := range []string{"dropped-kv", "dropped-prefix"} {
		if strings.Contains(got, msg) {
			t.Errorf("%s survived the inherited filter: %q", msg, got)
		}
	}

	for _, msg := range []string{"kept-kv", "kept-prefix"} {
		if !strings.Contains(got, msg) {
			t.Errorf("%s missing: %q", msg, got)
		}
	}
}

// The ordering trap the docs already describe for styles, pinned where it bites.
// Changing MinLevel on the base after deriving does not reach the copy.
func TestFilterSetAfterDerivingDoesNotReachTheCopy(t *testing.T) {
	var buf bytes.Buffer

	base := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&buf)
	child := base.WithPrefix("svc")

	base.Config.MinLevel = ErrorLevel

	if base.Enabled(InfoLevel) {
		t.Error("the base should not write info any more")
	}

	if !child.Enabled(InfoLevel) {
		t.Error("the copy was made before the change, so it must still write info")
	}

	child.Info("child-line")
	base.Info("base-line")

	got := buf.String()

	if !strings.Contains(got, "child-line") {
		t.Errorf("the copy lost its own filter: %q", got)
	}

	if strings.Contains(got, "base-line") {
		t.Errorf("the base wrote despite its own filter: %q", got)
	}
}

// Enabled is the comparison print makes, exposed so a caller can skip building the
// message rather than paying for it and throwing it away.
func TestEnabledMatchesWhatPrintWrites(t *testing.T) {
	var buf bytes.Buffer

	l := CreateLogger(Config{MinLevel: WarnLevel, Style: Style{NoColors: true}}).WithWriter(&buf)

	for _, level := range []LogLevel{DebugLevel, InfoLevel, WarnLevel, ErrorLevel, FatalLevel} {
		want := level >= WarnLevel

		if got := l.Enabled(level); got != want {
			t.Errorf("Enabled(%v) = %v, want %v", level, got, want)
		}
	}

	if !l.Enabled(FatalLevel) {
		t.Error("a filter at warn must not hide fatal")
	}
}

// Fatal is the one case a filter must not swallow silently, and it exits the
// process, so it can only be observed from a child.
func TestMinLevelDoesNotHideFatal(t *testing.T) {
	if os.Getenv("STACKLOG_FATAL_CHILD") == "3" {
		f, err := os.Create(os.Getenv("STACKLOG_FATAL_FILE"))
		if err != nil {
			t.Fatal(err)
		}

		defer f.Close()

		l := CreateLogger(Config{MinLevel: WarnLevel, Style: Style{NoColors: true}}).WithWriter(f)

		l.Info("below the filter")
		l.Fatal("shutting down, the port is still in use")

		return
	}

	path := filepath.Join(t.TempDir(), "minlevel-fatal.log")

	cmd := exec.Command(os.Args[0], "-test.run=TestMinLevelDoesNotHideFatal")
	cmd.Env = append(os.Environ(), "STACKLOG_FATAL_CHILD=3", "STACKLOG_FATAL_FILE="+path)

	if err := cmd.Run(); err == nil {
		t.Fatal("the child was expected to exit with a status, got none")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got := string(data)

	if !strings.Contains(got, "shutting down, the port is still in use") {
		t.Errorf("fatal was hidden by a warn filter: %q", got)
	}

	if strings.Contains(got, "below the filter") {
		t.Errorf("info wrote through a warn filter: %q", got)
	}
}

// What a filtered call actually costs: one comparison and a return. Nothing is
// written into the buffer, nothing is flushed, nothing is allocated.
func BenchmarkFilteredCall(b *testing.B) {
	var buf bytes.Buffer

	l := CreateLogger(Config{MinLevel: ErrorLevel, Style: Style{NoColors: true}}).WithWriter(&buf)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		l.Debug("this line is never built")
	}
}
