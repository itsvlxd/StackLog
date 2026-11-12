package stacklog

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

// The progress tests are mostly about the negative half - "this writer is not a terminal, so it gets
// plain log lines" - because that is the half that corrupts files. frameRecorder keeps every Write
// separate, which is the only way to count redraws: a buffer that concatenates cannot tell six frames on
// one line from six lines.
type frameRecorder struct {
	writes [][]byte
}

func (f *frameRecorder) Write(p []byte) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	f.writes = append(f.writes, cp)

	return len(p), nil
}

func (f *frameRecorder) bytes() []byte {
	var out bytes.Buffer

	for _, w := range f.writes {
		out.Write(w)
	}

	return out.Bytes()
}

func (f *frameRecorder) text() string { return string(f.bytes()) }

func (f *frameRecorder) redraws() int {
	n := 0

	for _, w := range f.writes {
		if len(w) > 0 && w[0] == '\r' {
			n++
		}
	}

	return n
}

// asTerminal is the in-package way of putting a logger on a terminal. isTerminal itself is asserted in
// colorpolicy_test.go, and what these tests need is the pair of verdicts a real terminal produces:
// control bytes allowed, and colour allowed unless the config already said otherwise. Both have to move
// together, or a test measures a terminal that writes no colour, which is a machine nobody has.
func asTerminal(l *StackLog) *StackLog {
	l.isTTY = true
	l.emitColor = resolveColor(l.Config, true)

	return l
}

// The default look of a bar, because a bar with no colour in it looks like a formatting accident. The
// package default is a green filled run on a grey remainder, and it comes from the preset like every
// other component does.
func TestProgressBarIsGreenOnGreyByDefault(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{}).WithWriter(rec)
	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond})

	asTerminal(p.sl)

	p.Tick(2)

	if err := p.Done("index rebuilt"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	if !strings.Contains(text, "\x1b[38;5;70m██") {
		t.Errorf("the filled run is not the preset's green: %q", text)
	}

	if !strings.Contains(text, "\x1b[38;5;242m░░") {
		t.Errorf("the remainder is not the preset's 242 grey: %q", text)
	}
}

// Every preset carries a bar palette, and each one is its own decision rather than a copy of the default.
// Pinned because "the presets are the package look" is a promise the README makes.
func TestEveryPresetCarriesABarPalette(t *testing.T) {
	names := []string{PresetDefault, PresetMuted, PresetBadge, PresetHighContrast}

	for _, name := range names {
		l := CreateLogger(Config{Style: Style{Preset: name}})

		if l.Config.Style.Progress == (ProgressStyle{}) {
			t.Errorf("%s left the bar with no palette", name)
		}
	}

	if got := CreateLogger(Config{Style: Style{Preset: PresetHighContrast}}).Config.Style.Progress.Filled; got != NewStyle().Foreground(82) {
		t.Errorf("high contrast's bar is not 82 green: %+v", got)
	}

	if got := CreateLogger(Config{Style: Style{Preset: PresetBadge}}).Config.Style.Progress.Filled; got == (NewStyle().Foreground(70)) {
		t.Error("the badge preset's bar should be the block treatment, not the default run")
	}
}

// A caller who sets Style.Progress gets exactly what they set; a preset does not overwrite it.
func TestProgressStyleOverrideStandsAgainstThePreset(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Style: Style{Progress: ProgressStyle{
		Filled:  NewStyle().Foreground("#D75F5F"),
		Empty:   NewStyle().Foreground(15),
		Numbers: NewStyle().Foreground(242),
	}}}).WithWriter(rec)

	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(2)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	if !strings.Contains(text, "\x1b[38;2;215;95;95m") {
		t.Errorf("the caller's hex did not reach the filled run: %q", text)
	}

	if strings.Contains(text, "\x1b[38;5;70m") {
		t.Errorf("the preset overrode a style the caller set: %q", text)
	}
}

// Colour suppression gates the bar's colours and nothing else. A frame without its clearing sequence
// stops erasing the longer line before it, which is worse than a colourless bar.
func TestProgressSuppressionGatesBarColourNotTheClearSequence(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(2)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	if !strings.Contains(text, barFilled+barFilled+barEmpty+barEmpty) {
		t.Errorf("suppression removed the bar as well as its colour: %q", text)
	}

	if strings.Contains(text, "\x1b[38;5;") || strings.Contains(text, "\x1b[0m") {
		t.Errorf("ColorsOff let a styled bar paint: %q", text)
	}

	if !strings.Contains(text, progressClear) {
		t.Errorf("suppression dropped the clearing sequence, which is not what it gates: %q", text)
	}
}

// The line is a log line first and a bar second: level label, the message the caller named, the
// logger's params, the progress numbers, and the bar at the end of it.
func TestProgressLineCarriesLevelMessageParamsAndBar(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).
		WithWriter(&out).
		WithParams("file", "index.sqlite")

	p := l.WithProgress(4, ProgressOptions{
		Level:   InfoLevel,
		Message: "rebuilding index",
		Params:  []any{"job", "reindex"},
		Every:   time.Nanosecond,
	})

	p.Tick(2)
	p.Tick(2)

	if err := p.Done("index rebuilt"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	for _, want := range []string{
		"INFO rebuilding index file=index.sqlite job=reindex ████████████████████░░░░░░░░░░░░░░░░░░░░ 2/4 50% ",
		"INFO rebuilding index file=index.sqlite job=reindex ████████████████████████████████████████ 4/4 100% ",
		"INFO index rebuilt file=index.sqlite job=reindex ████████████████████████████████████████ 4/4 100% ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line:\n got: %q\nwant: %q", got, want)
		}
	}

	if !strings.Contains(got, barFilled+barEmpty) {
		t.Errorf("a file row has no bar on it: %q", got)
	}

	if strings.Contains(got, "\r") || containsEscape([]byte(got)) {
		t.Errorf("a non-terminal writer received control bytes: %q", got)
	}

	if strings.Contains(got, "pct=") || strings.Contains(got, "done=") {
		t.Errorf("a text line stated the progress numbers twice - as params and beside the bar: %q", got)
	}
}

// A bar in a log file is 120 bytes of box-drawing characters nobody asked for. The numbers are the part
// a file reader needs, so the remainder and the filled run are drawn only where a terminal is.
// A file cannot redraw, so a bar there is drawn once per row and every row is its own line. That is the
// shape the feature has in a file: the history of a run, row by row, with the bar lengthening down the
// page. It costs 120 bytes a row at the default width - the price of the shape, not an oversight - and
// the redraw budget is what keeps it from becoming 120 bytes times a million rows.
func TestProgressWritesOneRowPerUpdateIntoAFileWithTheBarOnEach(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out)

	p := l.WithProgress(3, ProgressOptions{Width: 6, Level: InfoLevel, Every: time.Nanosecond})
	p.Tick(1)
	p.Tick(1)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	rows := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	if len(rows) != 3 {
		t.Fatalf("got %d rows for two ticks and a Done, want 3: %q", len(rows), got)
	}

	for i, row := range rows {
		if strings.Contains(row, "\r") || containsEscape([]byte(row)) {
			t.Errorf("row %d carries control bytes, which a file cannot redraw with: %q", i, row)
		}

		if !strings.Contains(row, barEmpty) || !strings.Contains(row, barFilled) && i < 2 {
			t.Errorf("row %d has no bar on it: %q", i, row)
		}

		if !strings.Contains(row, " 1/3 33% ") && !strings.Contains(row, " 2/3 66% ") && !strings.Contains(row, " 3/3 100% ") {
			t.Errorf("row %d does not hold its counts with a space between them and the bar: %q", i, row)
		}
	}

	// The bars lengthen down the page, which is the whole point of drawing one in a file.
	for i := 1; i < len(rows); i++ {
		if strings.Count(rows[i], barFilled) < strings.Count(rows[i-1], barFilled) {
			t.Errorf("row %d has a shorter bar than the row before it: %q", i, got)
		}
	}

	if strings.Contains(got, "pct=") || strings.Contains(got, "done=") {
		t.Errorf("a text row stated the progress numbers twice - as params and beside the bar: %q", got)
	}
}

// The JSON form is the opposite decision, and it is the one a parser will thank the package for. Each
// update is its own object row, and the bar arrives as the two numbers that draw it - filled cells and
// width - rather than 120 bytes of box-drawing characters inside something meant to be read by a
// machine. A bar is a rendering, and the rendering can be rebuilt from the fields.
func TestProgressJSONRowsCarryTheBarAsNumbersNotGlyphs(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Format: FormatJSON}).WithWriter(&out)

	p := l.WithProgress(6, ProgressOptions{Level: InfoLevel, Message: "uploading", Every: time.Nanosecond})
	p.Tick(1)
	p.Tick(3)

	if err := p.Done("upload finished"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if strings.ContainsAny(got, "█░") {
		t.Errorf("a JSON row carried bar glyphs: %q", got)
	}

	if strings.Contains(got, "\r") || containsEscape([]byte(got)) {
		t.Errorf("a JSON stream received control bytes: %q", got)
	}

	rows := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	if len(rows) != 3 {
		t.Fatalf("got %d JSON rows for two ticks and a Done, want 3: %q", len(rows), got)
	}

	for i, row := range rows {
		for _, want := range []string{`"pct"`, `"done"`, `"total"`, `"ms"`, `"filled"`, `"width"`} {
			if !strings.Contains(row, want) {
				t.Errorf("JSON row %d is missing %s: %q", i, want, row)
			}
		}
	}

	if !strings.Contains(rows[1], `"done":4`) || !strings.Contains(rows[1], `"filled":26`) {
		t.Errorf("the second row does not carry the counts a reader needs to rebuild the bar: %q", rows[1])
	}
}

// The redraw, and the shape that makes it a redraw: one line, replaced, no newline in the middle.
func TestProgressRedrawsOneLineOnATerminal(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff, Style: Style{NoColors: false}}).WithWriter(rec)
	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond, Level: WarnLevel, Message: "uploading"})
	asTerminal(p.sl)

	p.Tick(1)
	p.Tick(1)

	if err := p.Done("upload finished"); err != nil {
		t.Fatal(err)
	}

	for i, w := range rec.writes {
		if len(w) > 0 && w[0] == '\r' && !bytes.Contains(w, []byte("\x1b[K")) {
			t.Errorf("redraw %d has no clearing sequence: %q", i, w)
		}

		if i < len(rec.writes)-1 && len(w) > 0 && w[0] == '\r' && bytes.Contains(w, []byte("\n")) {
			t.Errorf("redraw %d wrote a newline, so the next one could not replace it: %q", i, w)
		}
	}

	text := rec.text()

	if !strings.HasPrefix(text, "\r") {
		t.Errorf("the first redraw does not open the line: %q", text)
	}

	if !strings.Contains(text, "WARN uploading") {
		t.Errorf("the redrawn line has no level label and no message in it: %q", text)
	}

	if !strings.HasSuffix(text, progressClear+"\n") {
		t.Errorf("Done did not terminate the line it redrew: %q", text)
	}
}

// Done replaces the bar's line rather than adding one after it. Two lines where one kept being redrawn
// is how a progress bar ends up scrolling the reader's output.
func TestProgressDoneReplacesTheBarLine(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(2, ProgressOptions{Width: 4, Every: time.Nanosecond, Level: InfoLevel})
	asTerminal(p.sl)

	p.Tick(1)
	p.Tick(1)

	if err := p.Done("index rebuilt"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	// Three writes: two redraws and the terminating one. And exactly one newline in the whole run,
	// because a bar that ends by adding a line after itself scrolls the reader's output instead of
	// finishing a line.
	if got := len(rec.writes); got != 3 {
		t.Errorf("got %d writes for two ticks and a Done, want 3: %q", got, text)
	}

	if n := strings.Count(text, "\n"); n != 1 {
		t.Errorf("got %d newlines in a bar run that should end one line: %q", n, text)
	}

	last := rec.writes[len(rec.writes)-1]

	if !bytes.HasSuffix(last, []byte(progressClear+"\n")) {
		t.Errorf("the last write does not clear and terminate the line it redrew: %q", last)
	}

	if !strings.Contains(string(last), "index rebuilt") {
		t.Errorf("the terminating line does not carry Done's message: %q", last)
	}

	if strings.Count(text, barFilled+barFilled+barFilled+barFilled) < 2 {
		t.Errorf("the completed bar disappeared rather than being replaced: %q", text)
	}
}

// A redraw is not a log line in the way that costs anything: a thousand ticks with a one-hour budget
// produce two writes, whatever the loop looks like.
func TestProgressRedrawBudgetBoundsWrites(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(1000, ProgressOptions{Width: 20, Every: time.Hour})
	asTerminal(p.sl)

	for i := 0; i < 1000; i++ {
		p.Tick(1)
	}

	if err := p.Done("done"); err != nil {
		t.Fatal(err)
	}

	if got := rec.redraws(); got != 2 {
		t.Errorf("got %d redraws for 1,000 ticks with a one-hour budget, want 2: %q", got, rec.text())
	}
}

// The same budget on the file side, because a bar that becomes 5,000,000 log lines is worse than no bar.
func TestProgressFileMilestonesAreBudgetedToo(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out)
	p := l.WithProgress(1000, ProgressOptions{Every: time.Hour, Level: InfoLevel})

	for i := 0; i < 1000; i++ {
		p.Tick(1)
	}

	if err := p.Done("done"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("got %d lines for 1,000 ticks with a one-hour budget, want 2: %q", n, got)
	}
}

// A bar line is a log line, so MinLevel applies to it - and the honest consequence is that a level
// filtered out is a bar that does not appear at all, not a bar hidden behind a curtain.
func TestProgressBarLinesObeyMinLevel(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{MinLevel: WarnLevel, Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(2)

	if err := p.Done("filtered away"); err != nil {
		t.Fatal(err)
	}

	if len(rec.writes) != 0 {
		t.Errorf("a filtered level still produced bar lines: %q", rec.text())
	}
}

// A bar drawn at a level that passes the filter must not be left unterminated by a result line at a
// level that does not. An orphan line on someone's terminal is not something a filter gets to cause.
func TestProgressTerminatesABarWhoseResultLineWasFiltered(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{MinLevel: WarnLevel, Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond, Level: WarnLevel})
	asTerminal(p.sl)

	p.Tick(2)

	if err := p.Done("filtered result"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	if !strings.Contains(text, "WARN") {
		t.Errorf("a bar at a level that passes the filter did not draw: %q", text)
	}

	if !strings.HasSuffix(text, "\n") {
		t.Errorf("the result line was filtered and the bar was left unterminated: %q", text)
	}

	if strings.Contains(text, "filtered result") {
		t.Errorf("a filtered level wrote the result line anyway: %q", text)
	}
}

// JSON mode gets the numbers as fields and no bar anywhere, because there is nothing to redraw on and a
// percentage string is not data.
func TestProgressInJSONModeWritesObjectsNotBars(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Format: FormatJSON}).WithWriter(&out)
	p := l.WithProgress(4, ProgressOptions{Every: time.Nanosecond, Level: InfoLevel, Message: "uploading"})
	asTerminal(p.sl)

	p.Tick(2)

	if err := p.Done("upload finished"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if strings.Contains(got, "\r") || containsEscape([]byte(got)) || strings.ContainsAny(got, "█░") {
		t.Errorf("a JSON stream received a bar or its control bytes: %q", got)
	}

	if !strings.Contains(got, `"level":"info"`) || !strings.Contains(got, `"msg":"upload finished"`) {
		t.Errorf("the result is not an ordinary JSON object: %q", got)
	}

	if !strings.Contains(got, `"pct"`) || !strings.Contains(got, `"done"`) || !strings.Contains(got, `"ms"`) {
		t.Errorf("the progress numbers are not fields: %q", got)
	}
}

// A total of zero is legal and means an indeterminate bar. It never divides, and it never claims a
// fraction of nothing.
func TestProgressZeroTotalNeverDividesOrClaimsAPercentage(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(0, ProgressOptions{Width: 4, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(3)

	if err := p.Done("indeterminate"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	if strings.Contains(text, "100%") {
		t.Errorf("a total of zero claimed a percentage: %q", text)
	}

	if strings.Contains(text, "/0") {
		t.Errorf("a total of zero printed a fraction over nothing: %q", text)
	}

	if !strings.Contains(text, "0%") {
		t.Errorf("the line has no honest percentage: %q", text)
	}
}

// The clamp exists so the bar cannot eat the whole content area, and the clearing sequence is written
// past the content limit so a cut line still erases. A bar that loses it leaves ghosts.
func TestProgressWidthIsClampedAndTheClearSequenceSurvives(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Style: Style{Progress: ProgressStyle{
		Filled:  NewStyle().Background(236).Foreground(70).Padding(Padding{Left: 4, Right: 4}),
		Empty:   NewStyle().Background(238).Padding(Padding{Left: 4, Right: 4}),
		Numbers: NewStyle().Foreground("#ABCDFF"),
	}}}).WithWriter(rec)

	p := l.WithProgress(8, ProgressOptions{Width: 4000, Every: time.Nanosecond})
	asTerminal(p.sl)

	usable := p.sl.Buffer.usable()

	if p.width*3 > usable {
		t.Errorf("the clamp let a %d cell bar near a %d byte content area", p.width, usable)
	}

	for i := 0; i < 8; i++ {
		p.Tick(1)
	}

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	for i, w := range rec.writes {
		if len(w) > len(p.sl.Buffer.Buffer) {
			t.Errorf("redraw %d is %d bytes against a %d byte buffer", i, len(w), len(p.sl.Buffer.Buffer))
		}

		if len(w) > 0 && w[0] == '\r' && !bytes.Contains(w, []byte(progressClear)) {
			t.Errorf("redraw %d lost its clearing sequence: %q", i, w)
		}
	}
}

// Top and Bottom padding are refused inside a bar line - they write newlines, and a redraw needs its
// line. Left and Right are honoured. This includes padding on the level style, which is the case the
// shared line builder has to survive.
func TestProgressBarHonoursSidePaddingAndRefusesVerticalPadding(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Style: Style{
		Level:    [MaxLevel]CompStyle{DebugLevel: NewStyle().Padding(Padding{Top: 3, Bottom: 3}).Foreground(70)},
		Progress: ProgressStyle{Filled: NewStyle().Padding(Padding{Top: 2, Left: 2, Bottom: 2, Right: 2}).Foreground(70)},
	}}).WithWriter(rec)

	p := l.WithProgress(2, ProgressOptions{Width: 4, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(1)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	for i, w := range rec.writes {
		// The terminating line legitimately ends with a newline; every other redraw must not.
		if len(w) > 0 && w[0] == '\r' && !bytes.HasSuffix(w, []byte(progressClear+"\n")) && bytes.Contains(w, []byte("\n")) {
			t.Errorf("redraw %d wrote newlines from vertical padding: %q", i, w)
		}
	}

	if !strings.Contains(rec.text(), "  "+barFilled) {
		t.Errorf("Left padding was not written: %q", rec.text())
	}

	if !strings.Contains(rec.text(), barFilled+"  ") {
		t.Errorf("Right padding was not written: %q", rec.text())
	}
}

// The params a bar line carries are the logger's, then the caller's, then the bar's own four numbers.
// That order is the contract, and a key collision resolves in the open rather than by accident.
func TestProgressLineCarriesTheLoggersParamsThenItsOwnThenTheNumbers(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).
		WithWriter(&out).
		WithParams("file", "index.sqlite")

	p := l.WithProgress(2, ProgressOptions{Params: []any{"job", "reindex"}, Every: time.Nanosecond})

	p.Tick(1)
	p.Tick(1)

	if err := p.Done("index rebuilt"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	for _, want := range []string{
		"DEBU progress file=index.sqlite job=reindex ████████████████████░░░░░░░░░░░░░░░░░░░░ 1/2 50% ",
		"DEBU progress file=index.sqlite job=reindex ████████████████████████████████████████ 2/2 100% ",
		"INFO index rebuilt file=index.sqlite job=reindex ████████████████████████████████████████ 2/2 100% ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line:\n got: %q\nwant: %q", got, want)
		}
	}
}

// derive() copies the params slice header, so a logger derived with WithPrefix shares its parent's
// backing array and its spare capacity. Appending the bar's numbers onto that slice would write into
// memory the parent is still using, so the line is built in the capacity the bar allocated for itself.
func TestProgressNumbersDoNotWriteIntoTheLoggersParamsArray(t *testing.T) {
	var out bytes.Buffer

	base := CreateLogger(Config{Style: Style{NoColors: true}}).
		WithWriter(&out).
		WithParams("service", "upload")

	p := base.WithPrefix("http").WithProgress(2, ProgressOptions{})

	p.Tick(2)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	out.Reset()

	if err := base.Info("after the bar"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if !strings.Contains(got, "service=upload") {
		t.Errorf("the base logger lost its own params after a bar wrote one of its lines: %q", got)
	}

	if strings.Contains(got, "pct=") || strings.Contains(got, "ms=") {
		t.Errorf("the bar's numbers were written into the shared params array: %q", got)
	}
}

// The bar's own params must not leak back into the logger it was derived from.
func TestProgressLeavesTheLoggersOwnParamsAlone(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out).WithParams("service", "upload")

	p := l.WithProgress(2, ProgressOptions{})
	p.Tick(2)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	out.Reset()

	if err := p.sl.Info("next line"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if strings.Contains(got, "pct=") || strings.Contains(got, "ms=") {
		t.Errorf("the bar's params leaked into a later line: %q", got)
	}

	if !strings.Contains(got, "service=upload") {
		t.Errorf("the logger's own params were not restored: %q", got)
	}
}

// Abort is the same shape of line at ErrorLevel, for work that did not finish.
func TestProgressAbortWritesTheResultAtErrorLevel(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(10, ProgressOptions{Width: 10, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(3)

	if err := p.Abort("upload failed"); err != nil {
		t.Fatal(err)
	}

	text := rec.text()

	if !strings.Contains(text, "ERRO upload failed") {
		t.Errorf("Abort did not write the result at error level: %q", text)
	}

	if !strings.HasSuffix(text, progressClear+"\n") {
		t.Errorf("Abort left the bar unterminated: %q", text)
	}
}

// Finish is how a result line gets a level, and in this package a level is how a line gets its look.
func TestProgressFinishChoosesTheResultLevel(t *testing.T) {
	var out bytes.Buffer

	l := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out)
	p := l.WithProgress(10, ProgressOptions{})

	p.Tick(7)

	if err := p.Finish(WarnLevel, "index rebuilt with 3 skipped"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if !strings.Contains(got, "WARN index rebuilt with 3 skipped") || !strings.Contains(got, " 7/10 70% 0.0s") {
		t.Errorf("Finish did not write the result at the level named: %q", got)
	}
}

// The first write failure is kept and reported. A bar cannot retry on the caller's behalf and inventing
// a second error is worse than carrying the first one. failingWriter is the one plain_test.go provides.
func TestProgressReportsTheFirstWriteError(t *testing.T) {
	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(failingWriter{})
	p := l.WithProgress(4, ProgressOptions{Width: 4, Every: time.Nanosecond})
	asTerminal(p.sl)

	p.Tick(1)
	p.Tick(1)

	err := p.Done("upload finished")
	if err == nil {
		t.Fatal("no error reported for a writer that always fails")
	}

	if err != io.ErrClosedPipe {
		t.Errorf("got %v, want the underlying io.ErrClosedPipe", err)
	}
}

// The defaults, in one place, because "the zero value means the package default" is only worth
// promising if somebody checks it.
func TestProgressDefaults(t *testing.T) {
	l := CreateLogger(Config{}).WithWriter(&bytes.Buffer{})
	p := l.WithProgress(1, ProgressOptions{})

	if p.width != progressWidthDefault {
		t.Errorf("width %d, want the documented default %d", p.width, progressWidthDefault)
	}

	if p.every != progressEveryDefault {
		t.Errorf("every %v, want the documented default %v", p.every, progressEveryDefault)
	}

	if p.level != DebugLevel {
		t.Errorf("level %v, want the documented default DebugLevel", p.level)
	}

	if p.message != progressDefaultMessage {
		t.Errorf("message %q, want %q", p.message, progressDefaultMessage)
	}

	if p.redraws() {
		t.Error("a buffer writer was treated as a terminal")
	}
}

// WithProgress derives, so the bar writes through its own logger with its own buffer and the parent is
// left alone. Two bars on one logger must not end up sharing one 1024-byte array.
func TestWithProgressDerivesItsOwnLoggerAndBuffer(t *testing.T) {
	var out bytes.Buffer

	base := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out)

	first := base.WithProgress(2, ProgressOptions{})
	second := base.WithProgress(2, ProgressOptions{})

	if first.sl.Buffer == second.sl.Buffer || first.sl.Buffer == base.Buffer {
		t.Error("two bars share a buffer, so their lines would overwrite each other")
	}

	if first.sl.Config.Timestamp != base.Config.Timestamp {
		t.Error("the bar's logger lost the config it was derived from")
	}

	first.Tick(2)
	second.Tick(2)

	if err := first.Done("first"); err != nil {
		t.Fatal(err)
	}

	if err := second.Done("second"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if strings.Count(got, "INFO first") != 1 || strings.Count(got, "INFO second") != 1 {
		t.Errorf("the two bars did not write whole independent lines: %q", got)
	}

	out.Reset()

	if err := base.Info("after both bars"); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "pct=") {
		t.Errorf("a bar changed the logger it was derived from: %q", out.String())
	}
}

// The percentage text and the filled cells are two different granularities, and the line is due when
// either of them would look different. A bar over 1,000 items in 20 cells changes its cells far less
// often than its percentage, and both are checked.
func TestProgressRedrawsWhenEitherTheCellsOrThePercentageChanges(t *testing.T) {
	rec := &frameRecorder{}

	l := CreateLogger(Config{Colors: ColorsOff}).WithWriter(rec)
	p := l.WithProgress(100, ProgressOptions{Width: 10, Every: time.Nanosecond})
	asTerminal(p.sl)

	for i := 0; i < 100; i++ {
		p.Tick(1)
	}

	if err := p.Done("done"); err != nil {
		t.Fatal(err)
	}

	if got := rec.redraws(); got != 101 {
		t.Errorf("got %d redraws for 100 ticks of a 100-item bar, want one per percent plus the last: %q", got, rec.text())
	}
}

// WithProgress derives, and Logger hands that derived logger back. It is the way a caller keeps
// specialising a run - a prefix per stage, params per file - without the bar's own lines being affected
// and without a second bar being built on the first one's buffer.
func TestProgressLoggerHandsBackTheDerivedLoggerItWritesThrough(t *testing.T) {
	var out bytes.Buffer

	base := CreateLogger(Config{Style: Style{NoColors: true}}).WithWriter(&out)

	p := base.WithPrefix("http").WithProgress(2, ProgressOptions{Level: InfoLevel})
	p.Logger().Config.Timestamp = false

	if p.Logger() != p.sl {
		t.Error("Logger does not return the logger the bar actually writes through")
	}

	if p.sl.Buffer == base.Buffer {
		t.Error("the bar's logger shares the parent's buffer, so their lines would overwrite each other")
	}

	if p.sl.prefix != "http" {
		t.Errorf("the bar lost the prefix it was derived with: %q", p.sl.prefix)
	}

	p.Tick(2)

	if err := p.Done("finished"); err != nil {
		t.Fatal(err)
	}

	if err := p.Logger().Info("still carrying the prefix"); err != nil {
		t.Fatal(err)
	}

	got := out.String()

	if !strings.Contains(got, "INFO http: finished") {
		t.Errorf("the bar result line dropped the derived prefix: %q", got)
	}

	if !strings.Contains(got, "INFO http: still carrying the prefix") {
		t.Errorf("the derived logger did not keep carrying the prefix after the bar ran: %q", got)
	}
}
