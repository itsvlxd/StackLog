package stacklog

import (
	"errors"
	"io"
	"os"
	"time"
)

// StackLog writes one line per call to standard output.
//
// Params and prefix are configured through WithParams and WithPrefix, which
// return new loggers rather than modifying the original. Each derived logger
// holds its own buffer, so a base logger can be shared and specialised per call
// site. Two goroutines calling the same logger are not covered by that: give
// each goroutine its own derived logger.
//
// Output is written into a fixed 1024-byte buffer. Content stops at 1020 bytes so
// the tail can always carry the truncation marker and the line terminator. A value
// longer than the remaining content space is cut, and the line ends with "...".
type StackLog struct {
	Config Config
	Buffer *StackBuffer
	params []any
	prefix string
	wr     io.Writer
	// emitColor is the colour question answered once, when the writer became known. hasColor reads
	// it instead of consulting Config.Colors per line, because answering that question properly costs
	// a Stat syscall and a line is not the place to pay for it.
	emitColor bool
	// isTTY is the same answer with the overrides removed: is this writer a terminal, whatever the
	// caller asked for. Colours can be forced into a file with ColorsOn; control sequences cannot, and
	// Progress reads this field rather than calling Stat a second time.
	isTTY bool
}

// LogLevel selects the prefix and the colours used for one line.
type LogLevel int

// The levels a logger can write at.
const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
	MaxLevel
)

var (
	levelFgColor [MaxLevel]any     = [MaxLevel]any{5, 6, 3, 1, 1}
	levelPadding [MaxLevel]Padding = [MaxLevel]Padding{}
)

// Config controls what a logger writes.
//
// TimeFormat is a Go reference-time layout, for example "15:04:05". When it is
// empty, time.DateTime is used in text mode and time.RFC3339 in JSON mode: a JSON
// timestamp is meant to be parsed, and "2026-11-11 13:04:22" carries no offset.
//
// Style is optional. Any part left as the zero value falls back to the package
// default for that component. Config is copied by CreateLogger, so changing it
// after the call has no effect on the logger.
//
// MinLevel drops every call below it. The zero value is DebugLevel, which writes
// everything, so a Config that says nothing about levels behaves exactly as it did
// before the field existed. Levels ascend in severity, so a filter can never hide a
// more severe call than the level it keeps: with MinLevel set to WarnLevel, warn,
// error and fatal all still write and only debug and info are dropped. Setting
// MinLevel above FatalLevel silences everything including Fatal, which is a decision
// rather than an accident, and it is the one way to lose the line that explains a
// crash.
//
// Format selects what one call writes: FormatText, the coloured line, or FormatJSON,
// one JSON object per call terminated by a newline. The zero value is the empty string,
// which is FormatText, so a Config that says nothing about format writes what it wrote
// before the field existed. An unknown name resolves to FormatText, the same rule an
// unknown preset name follows.
//
// FormatJSON suppresses colours through NoColors rather than through a second path, so
// Style colour components are inert in that mode and Config.Style.NoColors reads back as
// true on a JSON logger. Padding is inert as well: Top and Bottom produce extra lines,
// and between JSON objects those are stray newlines rather than padding. Style.LevelPrefix
// is not read either, because the level is a field with fixed values there - see levelNames.
//
// Content in a JSON line stops at 1004 bytes against 1020 in text mode. The wider tail is
// the price of a line a parser can always close.
//
// Colors decides whether escape sequences are written at all: ColorsAuto, ColorsOn or ColorsOff.
// The zero value is the empty string, which is ColorsAuto - colour only where the writer is standard
// output or standard error attached to a terminal. ColorsOn writes colour into a pipe, a file or
// /dev/null alike; ColorsOff never writes one. An unrecognised name resolves to ColorsAuto, the way
// an unrecognised preset name resolves to the default preset.
//
// Style.NoColors outranks all three and can only ever turn colour off. Colors is answered when the
// writer is known - in CreateLogger and in WithWriter - and changing Config.Colors afterwards does
// not change the answer, which is asserted in a test rather than left to inference.
type Config struct {
	Style      Style
	Timestamp  bool
	TimeFormat string
	MinLevel   LogLevel
	Format     string
	Colors     string
}

// Style holds the colours for each component of a line. Set a style with
// NewStyle and the builder methods rather than by assigning the fields.
//
// Level, Params and LevelPrefix are three arrays of length MaxLevel. The fixed
// length is a choice rather than an accident: a Style is copied by value into every
// derived logger, and three arrays copy as three values. A map or a slice keyed by
// level would copy a header and leave the derived logger sharing one mutable style
// with the logger it came from, which is the exact failure the per-logger buffer
// exists to prevent.
//
// The length is the caller's problem in one place only: an array literal. A caller
// who takes a preset, or who sets one component after CreateLogger with
// Config.Style.Level[ErrorLevel], never has to know how many levels exist. Indices
// are written with the level constants for that reason. A positional 3 styles the
// wrong level the day one is inserted, and nothing reports it.
//
// MaxLevel is exported so a literal can name it rather than hard-code 5. Adding a
// level changes that length, which breaks every caller who wrote a literal. Saying
// so here is the whole point: it is a decision taken in the open rather than a
// surprise found in a release.
//
// Preset names a base style. CreateLogger takes each component from the preset
// only when that component is left as the zero value, so a component set here
// overrides the preset on its own. An unknown name resolves to PresetDefault.
//
// LevelPrefix holds the text written for each level. It is text, not colour, but
// it lives here so the same rule applies to it: a preset fills only the entries
// left empty, so a single label can be overridden on its own.
//
// An empty string means "not set", so a preset cannot express an intentionally
// empty label. That is the same class of problem as the colour sentinel.
type Style struct {
	Preset      string
	Timestamp   CompStyle
	Prefix      CompStyle
	Level       [MaxLevel]CompStyle
	Params      [MaxLevel]ParamsStyle
	LevelPrefix [MaxLevel]string
	// Progress is the colour set for the bar a progress line ends with: the cells already filled, the
	// cells still empty, and the counts, percentage and elapsed text. Every preset carries one, so the
	// package default is a green bar on a grey track, and Style.Progress is how you change it - the same
	// CompStyle builder, the same colour forms, the same NoColors and ColorsOff gates.
	Progress ProgressStyle

	// NoColors suppresses every escape sequence. It wins over every component, so a
	// caller who sets a colour and sets this gets no colour. That is the precedence
	// and it is stated here rather than discovered at the call site.
	//
	// Padding is not affected. Left and Right exist to sit inside a coloured block,
	// and with no colour there is no block, so they become plain whitespace. That is
	// an open decision, not a solved one.
	NoColors bool
}

// ParamsStyle is the colour set for one key/value pair: the key, the separator,
// and the value.
type ParamsStyle struct {
	Param CompStyle
	Equal CompStyle
	Value CompStyle
}

// ProgressStyle is the colour set for the bar a progress line ends with: three runs, because a bar has
// three parts that want to look different.
//
// Each is an ordinary CompStyle, so foreground, background and hex colours work the way they do anywhere
// else in this package, and NoColors, ColorsOff, and a writer that is not a terminal suppress every one
// of them. Left and Right padding are honoured inside a bar. Top and Bottom are not: they write newlines,
// and a progress line is one line that gets redrawn - padding that adds lines would move the bar off the
// line it lives on.
//
// Every preset sets one. The package default is a green filled run on a grey remainder, with the numbers
// in white, because a bar with no colour in it is a bar that looks like a formatting accident.
type ProgressStyle struct {
	Filled  CompStyle
	Empty   CompStyle
	Numbers CompStyle
}

// Predefined styles
var whiteFGStyle = NewStyle().Foreground(15)

var grayFGStyle = NewStyle().Foreground(242)

// CreateLogger returns a logger for the given config. Every part left empty is
// filled from the named preset, which falls back to PresetDefault.
//
// The returned logger writes to os.Stdout.
func CreateLogger(config Config) *StackLog {
	var sb *StackBuffer = CreateBuffer()

	// Init default values
	if config.TimeFormat == "" {
		if config.Format == FormatJSON {
			config.TimeFormat = time.RFC3339
		} else {
			config.TimeFormat = time.DateTime
		}
	}

	loadPreset(&config.Style)
	applyJSONFormat(&config, sb)

	sl := &StackLog{Config: config, Buffer: sb, wr: os.Stdout}
	sl.isTTY = isTerminal(sl.wr)
	sl.emitColor = resolveColor(sl.Config, sl.isTTY)

	return sl
}

// derive copies sl into a new logger with its own buffer.
//
// Every derived logger comes through here. WithParams, WithPrefix and WithWriter
// copy the struct by value, and that copy carries the *StackBuffer pointer: with
// no fresh buffer, two loggers share one 1024-byte array, one BufLen and one
// lineStart, so a line written by one is cut short or merged into a line written
// by the other. That is a data race even when each goroutine holds its own
// derived logger, and it shows up under go test -race.
//
// The new buffer is in the same state Reset() leaves a used one in, so a fresh
// buffer and a reused one behave identically and no padding rule changes.
func (sl *StackLog) derive() StackLog {
	newLogger := *sl

	newLogger.Buffer = CreateBuffer()
	applyJSONFormat(&newLogger.Config, newLogger.Buffer)

	return newLogger
}

// WithParams returns a new logger that appends key/value pairs to every line it
// writes. Arguments are read in pairs: the first is the key, the second the
// value. The original logger is not changed.
//
// An odd number of arguments is not checked. The last value is written with no
// key.
//
// Types that are not handled are written as <unsupported>.
func (sl *StackLog) WithParams(params ...any) *StackLog {
	newLogger := sl.derive()

	newLogger.params = params

	return &newLogger
}

// WithPrefix returns a new logger that writes prefix before the message. The
// original logger is not changed.
func (sl *StackLog) WithPrefix(prefix string) *StackLog {
	newLogger := sl.derive()

	newLogger.prefix = prefix

	return &newLogger
}

// WithProgress returns a progress bar bound to a derived logger: a bar whose lines carry this
// logger's prefix, timestamp, level styling, key/value styling and colour policy, on its own buffer.
// The original logger is not changed, which is what makes one bar per goroutine the safe pattern.
//
// The name belongs to the derivation family this package already uses - WithParams, WithPrefix,
// WithWriter - and the return type is the bar rather than a logger, because the bar is the stateful
// thing: it knows how far the work got, when it last drew, and what its last line looked like. A logger
// carrying that would be a logger with a timer in it.
//
// Every line the bar writes is an ordinary log line with the bar at the end of the message. See
// Progress.
func (sl *StackLog) WithProgress(total int, opts ProgressOptions) *Progress {
	derived := sl.derive()

	return newProgress(&derived, total, opts)
}

// WithWriter returns a new logger that writes to w. The original is not changed.
// If w is nil, os.Stdout is used.
//
// wr is unexported, so this is the only way to set it from outside the package.
func (sl *StackLog) WithWriter(w io.Writer) *StackLog {
	if w == nil {
		w = os.Stdout
	}

	newLogger := sl.derive()

	newLogger.wr = w
	// The colour answer belongs to the writer, so changing the writer changes the answer. A derived
	// logger that does not change it keeps whatever its parent settled on, which is the correct
	// result: same writer, same verdict. One isTerminal call answers both questions.
	newLogger.isTTY = isTerminal(w)
	newLogger.emitColor = resolveColor(newLogger.Config, newLogger.isTTY)

	return &newLogger
}

// ErrShortWrite is returned when the writer accepted fewer bytes than the line.
// By the io.Writer contract a short write is a failure, not a partial success.
var ErrShortWrite = errors.New("stacklog: short write")

// ErrLineTooLarge is returned by a JSON log call whose own structure will not fit the
// fixed buffer. It never comes from text mode: a cut text line is still a line, while half
// an object in a log stream is something a reader cannot tell apart from a corrupt one. No
// output is written when it is returned.
var ErrLineTooLarge = errors.New("stacklog: json line does not fit in the fixed buffer")

// Enabled reports whether a level writes on this logger. It is the same comparison
// print makes, exposed so a caller can skip building a message it knows will be
// thrown away.
func (sl *StackLog) Enabled(level LogLevel) bool {
	return level >= sl.Config.MinLevel
}

// Debug writes one line at debug level and returns the write error, if any.
func (sl *StackLog) Debug(message string) error {
	return sl.print(DebugLevel, message)
}

// Info writes one line at info level and returns the write error, if any.
func (sl *StackLog) Info(message string) error {
	return sl.print(InfoLevel, message)
}

// Warn writes one line at warn level and returns the write error, if any.
func (sl *StackLog) Warn(message string) error {
	return sl.print(WarnLevel, message)
}

// Error writes one line at error level and returns the write error, if any.
func (sl *StackLog) Error(message string) error {
	return sl.print(ErrorLevel, message)
}

// Fatal writes one line at fatal level and terminates the process with status 1.
//
// It does not return an error because it never returns. A write failure is reported
// to os.Stderr on a best-effort basis, since the configured writer is the thing that
// failed.
//
// If the writer can be flushed, it is flushed before the exit. Deferred functions do
// not run under os.Exit, so a caller who passed a buffered writer for throughput —
// the pattern the README recommends — would otherwise lose the fatal line and every
// line before it, which is precisely the output a crash is supposed to explain. The
// flush is attempted even when the write itself failed, to salvage what is already
// buffered. A flush failure is reported to os.Stderr as well.
func (sl *StackLog) Fatal(message string) {
	if err := sl.print(FatalLevel, message); err != nil {
		os.Stderr.Write([]byte("stacklog: write failed: " + err.Error() + "\n"))
	}

	if f, ok := sl.wr.(interface{ Flush() error }); ok {
		if err := f.Flush(); err != nil {
			os.Stderr.Write([]byte("stacklog: fatal flush failed: " + err.Error() + "\n"))
		}
	}

	os.Exit(1)
}

// Plain writes message to the logger's writer with nothing added: no level label, no prefix,
// no timestamp, no key/value pairs, no colour and no padding. It exists for output that is not
// a log line - a banner, an ASCII wordmark, help text - on the same writer the logger already
// owns, so a program does not need a second route to stdout just to print one thing.
//
// It is not a log level, so Config.MinLevel does not apply to it. A logger set to warn still
// prints its banner.
//
// It is not a format either. On a logger in FormatJSON mode a plain write stays exactly what it
// was handed: JSON mode wraps log lines, not arbitrary text, and turning a banner into a JSON
// string would make it useless to the human it was written for. The consequence is stated rather
// than hidden - a JSON log stream that also prints a banner contains one line that is not an
// object, so whatever parses it has to skip the lines that do not begin with "{".
//
// The bytes travel through the same fixed buffer as everything else, so the truncation rule
// applies: a plain payload longer than the content area is cut and the line ends with "...". A
// payload with newlines in it is allowed - they are just bytes - but it still has to fit in 1020
// bytes, and one call is still one Write to the writer.
func (sl *StackLog) Plain(message string) error {
	buf := sl.Buffer

	buf.WriteString(message)
	buf.markTruncated()
	buf.WriteNL()

	return sl.printBuffer()
}

func (sl *StackLog) addTimestamp() {
	buf := sl.Buffer
	tsStyle := sl.Config.Style.Timestamp

	sl.addPreStyle(tsStyle)

	timeBuf := time.Now().AppendFormat(buf.Buffer[buf.BufLen:buf.BufLen], sl.Config.TimeFormat)
	buf.BufLen += len(timeBuf)

	sl.addPostStyle(tsStyle)

	buf.WriteBlank()
}

func (sl *StackLog) addLevelPrefix(level LogLevel) {
	buf := sl.Buffer
	levelStyle := sl.Config.Style.Level[level]

	sl.addPreStyle(levelStyle)
	buf.WriteString(sl.Config.Style.LevelPrefix[level])
	sl.addPostStyle(levelStyle)

	buf.WriteBlank()
}

func (sl *StackLog) addLoggerPrefix() {
	buf := sl.Buffer

	style := sl.Config.Style.Prefix

	sl.addPreStyle(style)
	buf.WriteString(sl.prefix + ":")
	sl.addPostStyle(style)

	buf.WriteBlank()
}

func (sl *StackLog) addParams(level LogLevel) {
	buf := sl.Buffer
	paramStyle := sl.Config.Style.Params

	for i := 0; i < len(sl.params); i++ {
		if i%2 == 0 {
			buf.WriteBlank()
			sl.addPreStyle(paramStyle[level].Param)
			buf.AppendAny(sl.params[i])
			sl.addPreStyle(paramStyle[level].Equal)
			buf.WriteString("=")
		} else {
			sl.addPreStyle(paramStyle[level].Value)
			buf.AppendAny(sl.params[i])
			sl.addPostStyle(paramStyle[level].Value)
		}
	}
}

func (sl *StackLog) printBuffer() error {
	p := sl.Buffer.Buffer[:sl.Buffer.BufLen]

	n, err := sl.wr.Write(p)
	if err != nil {
		sl.Buffer.Reset()
		return err
	}

	if n < len(p) {
		sl.Buffer.Reset()
		return ErrShortWrite
	}

	// Reset the buffer length back to 0
	sl.Buffer.Reset()

	return nil
}

func (sl *StackLog) addBottomPadding() {
	buf := sl.Buffer

	for i := 0; i < buf.maxBottom; i++ {
		buf.WriteNL()
	}
}

// print writes an ordinary line: the composition, the marker if the line was cut, the terminator,
// and any bottom padding the line accumulated.
func (sl *StackLog) print(level LogLevel, message string) error {
	return sl.printLine(level, message, nil, false, true)
}

// lineTail is the hook a progress bar uses to hang itself off the end of a line. It is a function
// rather than a field on the logger because only one kind of line has a tail, and the ordinary path
// should not pay for something it does not use beyond one nil check.
type lineTail func(buf *StackBuffer)

// printLine is the whole line builder, shared by an ordinary line and a progress line so the two
// cannot drift apart. Everything a line carries - timestamp, level label, prefix, message, params -
// arrives here once, and a bar is appended at the end rather than living on a line of its own.
//
// inPlace says this line is meant to replace the one above it on the reader's screen: it opens with a
// carriage return, and it does not carry the truncation marker. The tail owns its own clearing
// sequence in that case, and the sequence is written into the reserved tail, because a bar that loses
// it stops erasing the longer line before it and leaves ghosts.
//
// terminate says whether the line ends. A redrawn frame does not; the last line of a run does.
func (sl *StackLog) printLine(level LogLevel, message string, tail lineTail, inPlace, terminate bool) error {
	// One comparison, before anything is written into the buffer. A filtered call
	// costs that comparison and stops; it does not build a line, flush it, or
	// allocate.
	if !sl.Enabled(level) {
		return nil
	}

	// One comparison on the format, and the two builders never share a path. The text
	// builder is untouched by JSON mode, which is the point: an additive field should not
	// put a branch inside every step of the line it does not change.
	//
	// A JSON line has no bar in it, in either direction. There is nowhere to redraw one and a
	// percentage string is not data. The progress numbers still arrive, through params, which is where
	// numbers belong in this format.
	if sl.Config.Format == FormatJSON {
		return sl.printJSON(level, message)
	}

	buf := sl.Buffer

	if inPlace {
		buf.AppendByte('\r')
		// Top padding is written when the buffer believes it is at the start of a line, and a redraw is
		// at a line start in the terminal's eyes. Clearing the flag here is what keeps a level style's
		// Top padding from pushing the bar onto a line above itself.
		buf.lineStart = false
	}

	if sl.Config.Timestamp {
		sl.addTimestamp()
	}

	sl.addLevelPrefix(level)

	if len(sl.prefix) > 0 {
		sl.addLoggerPrefix()
	}

	// Adds the raw byte message inside the buffer
	buf.WriteString(message)

	if len(sl.params) > 0 {
		sl.addParams(level)
	}

	if tail != nil {
		buf.AppendByte(' ')
		tail(buf)
	}

	if inPlace {
		if terminate {
			buf.WriteNL()
		}

		return sl.printBuffer()
	}

	// The marker goes before the terminator. Both land in the reserved tail, so
	// neither can be dropped by a full buffer.
	buf.markTruncated()

	buf.WriteNL()

	sl.addBottomPadding()

	return sl.printBuffer()
}
