package stacklog_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/itsvlxd/stacklog"
)

// Every assertable example here sets NoColors. An `// Output:` block must match the
// bytes exactly, and escape sequences make that unreadable and brittle. The flag is
// what makes the examples verifiable rather than merely illustrative.
//
// Fatal is absent on purpose. It calls os.Exit(1), which would kill the test binary
// before the other examples ran. There is no honest way to show it in an example
// that is also runnable, so it is a documented gap rather than a fake.

func Example() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	logger.Debug("cache miss for key user:42")
	logger.Info("request completed in 12ms")
	logger.Warn("retry 3 of 5 for user:42, the upstream is slow")
	logger.Error("upstream returned 502 for user:42")

	// Output:
	// DEBU cache miss for key user:42
	// INFO request completed in 12ms
	// WARN retry 3 of 5 for user:42, the upstream is slow
	// ERRO upstream returned 502 for user:42
}

// MinLevel drops every call below it, before the line is built. Written to a buffer
// here so the dropped calls are visibly absent rather than merely claimed.
func ExampleStackLog_Info_filtered() {
	var buf bytes.Buffer

	logger := stacklog.CreateLogger(stacklog.Config{
		MinLevel: stacklog.WarnLevel,
		Style:    stacklog.Style{NoColors: true},
	}).WithWriter(&buf)

	logger.Debug("cache miss for key user:42")
	logger.Info("request completed in 12ms")
	logger.Warn("retry 3 of 5 for user:42")
	logger.Error("upstream returned 502 for user:42")

	fmt.Print(buf.String())

	// Output:
	// WARN retry 3 of 5 for user:42
	// ERRO upstream returned 502 for user:42
}

func ExampleStackLog_WithParams() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	kv := logger.WithParams("request_id", "b7f1c2", "user", "vlad")

	kv.Error("upstream returned 502, retried 3 times")

	// Output:
	// ERRO upstream returned 502, retried 3 times request_id=b7f1c2 user=vlad
}

// A value whose type AppendAny does not handle is written as <unsupported> rather
// than being formatted outside the log stream. The key is still written; only the
// value is refused.
func ExampleStackLog_WithParams_unsupported() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	logger.WithParams("when", time.Now()).Warn("a time.Time is not a supported value")

	// Output:
	// WARN a time.Time is not a supported value when=<unsupported>
}

func ExampleStackLog_WithPrefix() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	prefixed := logger.WithParams("request_id", "b7f1c2").WithPrefix("http")

	prefixed.Warn("handler entered, upstream slow")

	// The original logger is never modified, so a base logger can be shared and
	// specialised per call site.
	logger.Info("this call has no prefix and no params")

	// Output:
	// WARN http: handler entered, upstream slow request_id=b7f1c2
	// INFO this call has no prefix and no params
}

func ExampleStackLog_WithWriter() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	var buf bytes.Buffer

	if err := logger.WithWriter(&buf).Error("this line goes to the buffer, not to stdout"); err != nil {
		fmt.Println("write failed:", err)
		return
	}

	fmt.Print(buf.String())

	// Output:
	// ERRO this line goes to the buffer, not to stdout
}

// A writer that accepts fewer bytes than the line is a failure, not a partial
// success, because that is what the io.Writer contract says.
type halfWriter struct{}

func (halfWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func ExampleStackLog_Error() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	err := logger.WithWriter(halfWriter{}).Error("the writer accepted fewer bytes than the line")

	fmt.Println("error:", err)

	// Output:
	// error: stacklog: short write
}

// The buffer never grows. Content stops at 1020 bytes, which leaves a reserved tail
// so a cut line can always carry the marker and its terminator. Two log calls can
// never merge into one run of text.
func ExampleStackLog_Info_truncated() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	var buf bytes.Buffer
	logger.WithWriter(&buf).Info(strings.Repeat("A", 1100))

	out := buf.String()

	fmt.Printf("flushed %d bytes, tail %q\n", len(out), out[len(out)-8:])

	// Output:
	// flushed 1024 bytes, tail "AAAA...\n"
}

// Left and Right are spaces written inside the component. With NoColors there is no
// coloured block, but the padding is still part of the line, so %q makes it visible.
func ExamplePadding() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	logger.Config.Style.Level[stacklog.ErrorLevel] = stacklog.NewStyle().
		Padding(stacklog.Padding{Left: 1, Right: 1})

	var buf bytes.Buffer
	logger.WithWriter(&buf).Error("padded level")

	fmt.Printf("%q\n", buf.String())

	// Output:
	// " ERRO  padded level\n"
}

// Presets are compile-only here. The point of a preset is the colour and the label
// set, and an `// Output:` block cannot assert colour without becoming unreadable.
// This one documents the shape; the assertable behaviour is covered above.
func ExampleStyle() {
	badge := stacklog.CreateLogger(stacklog.Config{
		Timestamp:  true,
		TimeFormat: "15:04:05",
		Style:      stacklog.Style{Preset: stacklog.PresetBadge},
	})

	muted := stacklog.CreateLogger(stacklog.Config{
		Style: stacklog.Style{Preset: stacklog.PresetMuted},
	})

	// A component set by the caller overrides the preset on its own. The rest of the
	// preset stands. An unknown preset name resolves to the default.
	custom := stacklog.CreateLogger(stacklog.Config{
		Style: stacklog.Style{
			Preset:      stacklog.PresetHighContrast,
			LevelPrefix: [stacklog.MaxLevel]string{"!!"},
		},
	})

	badge.Error("the level is a block, with padding inside it")
	muted.Warn("low-chroma palette, full-word labels")
	custom.Info("only level 0 would be the override; this is level 1 from the preset")
}

// A JSON line is one object per call, newline terminated, with colours suppressed because
// escape codes inside a JSON string are noise to a parser. Entry is exported, so the shape is
// something a caller decodes rather than guesses. Timestamp is left off here because an
// // Output: block has to match bytes exactly.
func ExampleEntry() {
	var out bytes.Buffer

	logger := stacklog.CreateLogger(stacklog.Config{Format: stacklog.FormatJSON}).WithWriter(&out)

	logger.WithPrefix("http").WithParams("user", "vlad", "ms", 12.4).Error("upstream returned 502")

	var e stacklog.Entry

	if err := json.Unmarshal(out.Bytes(), &e); err != nil {
		fmt.Println(err)

		return
	}

	fmt.Printf("%s %s %s %+v %v\n", e.Level, e.Prefix, e.Msg, e.Params, e.Truncated)

	// Output:
	// error http upstream returned 502 map[ms:12.4 user:vlad] false
}
