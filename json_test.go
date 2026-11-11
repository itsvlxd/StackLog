package stacklog

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func jsonLogger(config Config) (*StackLog, *bytes.Buffer) {
	var out bytes.Buffer

	config.Format = FormatJSON

	l := CreateLogger(config).WithWriter(&out)

	return l, &out
}

// The contract the whole mode rests on: every line is parseable, and it decodes into
// the exported Entry. A JSON logger that produces output a parser rejects is worse than no
// JSON logger, because the caller has already built something on top of it.
func TestJSONLineIsValidAndDecodesIntoEntry(t *testing.T) {
	l, out := jsonLogger(Config{Timestamp: true})

	l.Config.Style.LevelPrefix[ErrorLevel] = "!!"

	if err := l.WithPrefix("http").WithParams("retry", 3).Error("upstream returned 502"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("output is not valid JSON: %q", line)
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	if got.Level != "error" {
		t.Errorf("level = %q, want error", got.Level)
	}
	if got.Prefix != "http" {
		t.Errorf("prefix = %q, want http", got.Prefix)
	}
	if got.Msg != "upstream returned 502" {
		t.Errorf("msg = %q", got.Msg)
	}
	if got.Params["retry"] != float64(3) {
		t.Errorf("params retry = %v, want 3", got.Params["retry"])
	}
	if got.Truncated {
		t.Error("truncated is set on a line that was not cut")
	}
	if !strings.HasSuffix(line, "\n") {
		t.Error("a JSON line must end with a terminator")
	}
	if n := strings.Count(line, "\n"); n != 1 {
		t.Errorf("%d newlines in one line: %q", n, line)
	}
	if got.Time == "" {
		t.Error("time field missing on a logger created with Config.Timestamp")
	}
	if _, err := time.Parse(time.RFC3339, got.Time); err != nil {
		t.Errorf("time %q is not RFC3339: %v", got.Time, err)
	}
}

// The four ways a Go string breaks JSON are the double quote, the backslash, a control
// byte, and invalid UTF-8. Each one is escaped, and the decoded value comes back as what was
// logged rather than as something near it.
func TestJSONEscapesWhatJSONCannotCarry(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  string
		exact string
	}{
		{"quote", `he said "no"`, `he said "no"`, `\"no\"`},
		{"backslash", `C:\Users\vlad`, `C:\Users\vlad`, `\\Users\\vlad`},
		{"newline", "first\nsecond", "first\nsecond", `\n`},
		{"tab", "a\tb", "a\tb", `\t`},
		{"carriage-return", "a\rb", "a\rb", `\r`},
		{"bell-no-short-escape", "a\x07b", "a\x07b", `\u0007`},
		{"valid-multibyte", "serviciu \u0219", "serviciu \u0219", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, out := jsonLogger(Config{})

			if err := l.WithParams("k", c.in).Info(c.in); err != nil {
				t.Fatal(err)
			}

			line := out.String()

			if !json.Valid([]byte(line)) {
				t.Fatalf("not valid JSON: %q", line)
			}

			if c.exact != "" && !strings.Contains(line, c.exact) {
				t.Errorf("expected the escape %s in %q", c.exact, line)
			}

			var got Entry
			if err := json.Unmarshal([]byte(line), &got); err != nil {
				t.Fatal(err)
			}

			if got.Msg != c.want {
				t.Errorf("msg round trip = %q, want %q", got.Msg, c.want)
			}
			if got.Params["k"] != c.want {
				t.Errorf("param round trip = %v, want %q", got.Params["k"], c.want)
			}
		})
	}
}

// An invalid byte is not a legal JSON character, and passing it through produces a line a
// parser rejects at the byte. The replacement character stands in, and the line stays
// parseable.
func TestJSONInvalidUTF8BecomesReplacement(t *testing.T) {
	bad := "value \xff\xfe tail"

	l, out := jsonLogger(Config{})

	if err := l.Info(bad); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("an invalid byte produced unparseable output: %q", line)
	}

	if strings.Contains(line, "\xff") || strings.Contains(line, "\xfe") {
		t.Errorf("raw invalid bytes are still in the line: %q", line)
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	if !utf8.ValidString(got.Msg) {
		t.Errorf("decoded msg is not valid UTF-8: %q", got.Msg)
	}
}

// The message is the field that gives way: cut inside its own string, ending with the
// marker, with the quote closed after it and truncated:true on the object.
//
// Filling the content area with a message also leaves nothing for the params, and they are
// dropped. That is the order the line is built in, and the alternative - deciding in advance
// how much of the buffer a message is allowed to use - is a number nobody asked for.
func TestJSONLongMessageIsCutWithMarker(t *testing.T) {
	l, out := jsonLogger(Config{})

	long := strings.Repeat("A", 1200)

	if err := l.WithParams("user", "vlad").Info(long); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("a cut message produced unparseable output: %q", line)
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	if !got.Truncated {
		t.Error("a cut line must carry truncated:true")
	}
	if !strings.HasSuffix(got.Msg, truncateMark) {
		t.Errorf("cut msg does not end with the marker: %q", got.Msg[len(got.Msg)-8:])
	}
	if got.Params != nil {
		t.Errorf("params present after a message filled the content area: %v", got.Params)
	}
	if len(line) > 1024 {
		t.Errorf("flushed %d bytes, over the fixed buffer", len(line))
	}
	if got.Msg != long[:len(got.Msg)-len(truncateMark)]+truncateMark {
		t.Error("the cut message is not a prefix of what was logged")
	}
}

// A pair that will not fit is not added at all. A half pair - an opening quote with no
// closing one, or "key": with no value - is the specific failure a fixed buffer produces in
// JSON, and it is the one a reader cannot recover from.
func TestJSONPairThatDoesNotFitIsAbsentWhole(t *testing.T) {
	l, out := jsonLogger(Config{})

	huge := strings.Repeat("B", 1100)

	if err := l.WithParams("small", 1, "big", huge, "after", 2).Info("three pairs, one too wide"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("an oversized pair produced unparseable output: %q", line)
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	if !got.Truncated {
		t.Error("dropping a pair must mark the line truncated")
	}
	if _, ok := got.Params["big"]; ok {
		t.Errorf("the pair that did not fit is present: %v", got.Params["big"])
	}
	if got.Params["small"] != float64(1) {
		t.Errorf("the pair before it was lost: %v", got.Params)
	}
	if got.Params["after"] != float64(2) {
		t.Errorf("a pair after a dropped one was not tried: %v", got.Params)
	}
	if strings.Contains(line, `"big":`) {
		t.Errorf("a dangling key was left in the line: %q", line)
	}
	if strings.Contains(line, `,,`) {
		t.Errorf("a rolled-back pair left its comma behind: %q", line)
	}
}

// Colours are suppressed by NoColors, which already exists and already wins over every
// component, and padding is inert rather than translated. An escape byte in a JSON string is
// noise to a parser and a padding newline between objects is a stray line.
func TestJSONModeSuppressesColoursAndPadding(t *testing.T) {
	l, out := jsonLogger(Config{})

	l.Config.Style.Level[ErrorLevel] = NewStyle().
		Foreground("#D75F5F").
		Background(15).
		Padding(Padding{Top: 2, Bottom: 2, Left: 2, Right: 2})

	l.Config.Style.Params[ErrorLevel] = ParamsStyle{
		Param: NewStyle().Foreground(242).Padding(Padding{Top: 1}),
		Value: NewStyle().Foreground(15),
	}

	if err := l.WithParams("user", "vlad").Error("no colour, no padding here"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if strings.ContainsRune(line, 0x1b) {
		t.Errorf("an escape byte reached JSON output: %q", line)
	}
	if !l.Config.Style.NoColors {
		t.Error("JSON mode is expected to suppress colour through NoColors, not beside it")
	}
	if n := strings.Count(line, "\n"); n != 1 {
		t.Errorf("%d newlines in one JSON line: %q", n, line)
	}
	if !json.Valid([]byte(line)) {
		t.Fatalf("padding or colour broke the line: %q", line)
	}
}

// Style.LevelPrefix is a display label. In JSON the level is a field with a fixed set of
// values, so the labels are not read and a JSON mode cannot use the label sets the presets
// define. That is asserted for every preset rather than for the default one.
func TestJSONLevelIsCanonicalWhateverThePresetLabelsSay(t *testing.T) {
	presets := []string{PresetDefault, PresetMuted, PresetBadge, PresetHighContrast}

	want := map[string]string{
		"debug": "debug", "info": "info", "warn": "warn", "error": "error", "fatal": "fatal",
	}

	for _, preset := range presets {
		t.Run(preset, func(t *testing.T) {
			l, out := jsonLogger(Config{Style: Style{Preset: preset}})

			if err := l.Info("m"); err != nil {
				t.Fatal(err)
			}
			if err := l.Warn("m"); err != nil {
				t.Fatal(err)
			}
			if err := l.Error("m"); err != nil {
				t.Fatal(err)
			}

			for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
				var got Entry
				if err := json.Unmarshal([]byte(line), &got); err != nil {
					t.Fatalf("%v on %q", err, line)
				}

				if _, ok := want[got.Level]; !ok {
					t.Errorf("level %q is not one of the canonical names, from preset %s", got.Level, preset)
				}
			}
		})
	}
}

// The Go type is kept: a number is a JSON number, a bool is a bool, a duration is the
// nanoseconds it already is in text mode, a nil interface is null, and a type the package
// cannot render is the same <unsupported> token text mode writes.
func TestJSONValueTypesRoundTrip(t *testing.T) {
	l, out := jsonLogger(Config{})

	pairs := []any{
		"i", 42,
		"i64", int64(-9000000000),
		"u", uint64(4294967296),
		"f64", 1.44,
		"f32", float32(1.1),
		"b", true,
		"dur", 1500 * time.Microsecond,
		"err", errors.New("connection reset by peer"),
		"nil", nil,
		"struct", time.Now(),
	}

	if err := l.WithParams(pairs...).Info("one of everything"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("not valid JSON: %q", line)
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	checks := map[string]any{
		"i":      float64(42),
		"i64":    float64(-9000000000),
		"u":      float64(4294967296),
		"f64":    1.44,
		"f32":    1.1,
		"b":      true,
		"dur":    float64(1500000),
		"err":    "connection reset by peer",
		"nil":    nil,
		"struct": jsonUnsupported,
	}

	for key, want := range checks {
		if got.Params[key] != want {
			t.Errorf("%s = %#v, want %#v", key, got.Params[key], want)
		}
	}

	// null and <unsupported> are different tokens on purpose. Collapsing them would make
	// "the package could not render this" indistinguishable from "this value is nil".
	if !strings.Contains(line, `"nil":null`) {
		t.Errorf("a nil interface is not written as null: %q", line)
	}
	// A number stays a number: unquoted, so a parser reads it as one.
	if !strings.Contains(line, `"i":42`) {
		t.Errorf("an int was not written as a JSON number: %q", line)
	}
}

// Format is an additive field. An empty Config, an unknown format name, and FormatText all
// write the coloured line the package has always written.
func TestFormatUnknownOrEmptyWritesText(t *testing.T) {
	cases := []string{"", "Text", "yaml", FormatText}

	for _, format := range cases {
		var out bytes.Buffer

		l := CreateLogger(Config{Format: format}).WithWriter(&out)

		if err := l.Error("still a text line"); err != nil {
			t.Fatal(err)
		}

		line := out.String()

		if !strings.Contains(line, "\x1b[31mERRO\x1b[0m") {
			t.Errorf("format %q did not write the text line it should: %q", format, line)
		}
		if json.Valid([]byte(line)) {
			t.Errorf("format %q wrote JSON, not text: %q", format, line)
		}
	}
}

// Format travels with the Config copy, so a JSON logger derived with WithPrefix or
// WithParams is still a JSON logger, and its fresh buffer still holds back the wider tail.
func TestDerivedLoggerKeepsTheFormatAndTheTail(t *testing.T) {
	base, _ := jsonLogger(Config{})
	child := base.WithPrefix("http").WithParams("user", "vlad")

	if child.Config.Format != FormatJSON {
		t.Fatal("a derived logger lost the format")
	}
	if got := child.Buffer.usable(); got != len(child.Buffer.Buffer)-jsonTailReserve {
		t.Errorf("derived buffer content area = %d, want %d", got, len(child.Buffer.Buffer)-jsonTailReserve)
	}

	var out bytes.Buffer
	child = child.WithWriter(&out)

	if err := child.Info("derived still writes JSON"); err != nil {
		t.Fatal(err)
	}

	if !json.Valid(out.Bytes()) {
		t.Fatalf("derived logger wrote unparseable output: %q", out.String())
	}
	if strings.ContainsRune(out.String(), 0x1b) {
		t.Errorf("a derived JSON logger emitted an escape byte: %q", out.String())
	}
}

// The wider tail is a number a caller can hit, so it is asserted rather than described.
func TestJSONContentAreaIsNarrowerThanText(t *testing.T) {
	text := CreateBuffer()
	j := CreateLogger(Config{Format: FormatJSON}).Buffer

	if got := text.usable(); got != 1020 {
		t.Errorf("text content area = %d, want 1020", got)
	}
	if got := j.usable(); got != 1004 {
		t.Errorf("JSON content area = %d, want 1004", got)
	}

	// A buffer built as a zero value is legal - StackBuffer is exported with exported
	// fields - and it must not be handed a tail 0 bytes wide.
	var direct StackBuffer

	if got := direct.usable(); got != 1020 {
		t.Errorf("a zero-value buffer content area = %d, want the text reserve 1020", got)
	}
}

// No params means no params field, not an empty object: a reader who decodes Entry gets
// Params == nil and can tell the two cases apart.
func TestJSONOmitsEmptyFields(t *testing.T) {
	l, out := jsonLogger(Config{})

	if err := l.Info("no params, no prefix, no time"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	for _, absent := range []string{`"params"`, `"prefix"`, `"time"`, `"truncated"`} {
		if strings.Contains(line, absent) {
			t.Errorf("%s appears in a line that has no reason to carry it: %q", absent, line)
		}
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	if got.Params != nil {
		t.Errorf("Params decoded as %v, want nil", got.Params)
	}
}

// An odd number of arguments drops the dangling value. A JSON object needs a key for every
// value, and inventing one is worse than dropping the pair.
func TestJSONOddParamCountDropsTheDanglingValue(t *testing.T) {
	l, out := jsonLogger(Config{})

	if err := l.WithParams("user", "vlad", "orphan").Warn("one value with no key"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("the dangling value broke the line: %q", line)
	}

	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}

	if len(got.Params) != 1 || got.Params["user"] != "vlad" {
		t.Errorf("params = %v, want only user=vlad", got.Params)
	}
}

// A key that is not a string keeps the text-mode rendering inside quotes rather than
// vanishing, so WithParams(1, "x") is readable in both formats.
func TestJSONNonStringKeysAreQuotedNotDropped(t *testing.T) {
	l, out := jsonLogger(Config{})

	if err := l.WithParams(1, "one", true, "flag").Info("keys that are not strings"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if !json.Valid([]byte(line)) {
		t.Fatalf("a non-string key produced unparseable output: %q", line)
	}

	var raw map[string]json.RawMessage
	var envelope Entry
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Fatal(err)
	}

	if envelope.Params["1"] != "one" {
		t.Errorf("integer key was not written as \"1\": %q", line)
	}
	if envelope.Params["true"] != "flag" {
		t.Errorf("bool key was not written as \"true\": %q", line)
	}
}

// Every byte a JSON line can need after its content area has to fit in the tail it reserves.
// If this arithmetic is ever wrong, lines get dropped at the closing brace and the failure
// looks like a parser problem rather than a buffer problem.
func TestJSONTailFitsItsReserve(t *testing.T) {
	tail := "}" + `,"truncated":true` + "}" + "\n"

	if len(tail) != jsonTailReserve {
		t.Errorf("the tail a JSON line can emit is %d bytes, the reserve holds back %d", len(tail), jsonTailReserve)
	}

	l, out := jsonLogger(Config{Timestamp: true, TimeFormat: time.RFC3339Nano})

	if err := l.WithParams("a", strings.Repeat("x", 4000)).Info(strings.Repeat("y", 4000)); err != nil {
		t.Fatalf("a maximal line failed: %v", err)
	}

	line := out.String()

	if len(line) > len(l.Buffer.Buffer) {
		t.Errorf("flushed %d bytes from a %d byte buffer", len(line), len(l.Buffer.Buffer))
	}
	if !json.Valid([]byte(line)) {
		t.Fatalf("the maximal line is not parseable: %q", line[len(line)-40:])
	}
}

// JSON mode keeps the claim the package exists to make. The escaping writer works inside the
// fixed array, the timestamp formats into a stack array, and a dropped pair is a BufLen
// assignment rather than an allocation.
func TestJSONLinePathAllocatesNothing(t *testing.T) {
	l, _ := jsonLogger(Config{Timestamp: true})
	l = l.WithParams("user", "vlad", "ms", 12.4, "retry", 3, "ok", true)

	got := testing.AllocsPerRun(200, func() {
		l.Info("service started, listening on 0.0.0.0:8080")
	})

	if got != 0 {
		t.Errorf("the JSON line path allocates %.0f times per call, want 0", got)
	}
}

// MinLevel filters before anything is built, in both formats. A JSON logger that filtered
// after building the line would be doing work it then throws away.
func TestJSONModeRespectsMinLevel(t *testing.T) {
	l, out := jsonLogger(Config{MinLevel: WarnLevel})

	l.Debug("dropped")
	l.Info("dropped")

	if err := l.Warn("kept"); err != nil {
		t.Fatal(err)
	}

	if n := strings.Count(out.String(), "\n"); n != 1 {
		t.Errorf("%d lines written, want 1: %q", n, out.String())
	}

	var got Entry
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatal(err)
	}
	if got.Level != "warn" {
		t.Errorf("level = %q, want warn", got.Level)
	}
}

// Text mode is the default and the mode every existing caller is in. Asserted as a line count
// and an escape, because a format field that quietly changed the default line would be a
// breaking change wearing a patch.
func TestTextModeIsUnchangedByJSONMode(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Timestamp: false}).WithWriter(&out)

	if err := l.Error("a text line, unchanged"); err != nil {
		t.Fatal(err)
	}

	line := out.String()

	if line != "\x1b[31mERRO\x1b[0m a text line, unchanged\n" {
		t.Errorf("the text line changed: %q", line)
	}
	if json.Valid([]byte(line)) {
		t.Error("text output parses as JSON")
	}
}
