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
// return new loggers rather than modifying the original. A base logger can be
// shared and specialised per call site.
//
// Output is written into a fixed 1024-byte buffer. A value longer than the
// remaining space is truncated silently.
type StackLog struct {
	Config Config
	Buffer *StackBuffer
	params []any
	prefix string
	wr     io.Writer
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
// empty, time.DateTime is used.
//
// Style is optional. Any part left as the zero value falls back to the package
// default for that component. Config is copied by CreateLogger, so changing it
// after the call has no effect on the logger.
type Config struct {
	Style      Style
	Timestamp  bool
	TimeFormat string
}

// Style holds the colours for each component of a line. Set a style with
// NewStyle and the builder methods rather than by assigning the fields.
//
// The Level and Params arrays have length MaxLevel, which is exported, so a
// caller can write [MaxLevel]CompStyle rather than a literal 5. Adding a level
// changes that length, which is a breaking change for anyone constructing a
// Style. See the MaxLevel card.
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
}

// ParamsStyle is the colour set for one key/value pair: the key, the separator,
// and the value.
type ParamsStyle struct {
	Param CompStyle
	Equal CompStyle
	Value CompStyle
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
		config.TimeFormat = time.DateTime
	}

	loadPreset(&config.Style)

	return &StackLog{Config: config, Buffer: sb, wr: os.Stdout}
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
	newLogger := *sl

	newLogger.params = params

	return &newLogger
}

// WithPrefix returns a new logger that writes prefix before the message. The
// original logger is not changed.
func (sl *StackLog) WithPrefix(prefix string) *StackLog {
	newLogger := *sl

	newLogger.prefix = prefix

	return &newLogger
}

// WithWriter returns a new logger that writes to w. The original is not changed.
// If w is nil, os.Stdout is used.
//
// wr is unexported, so this is the only way to set it from outside the package.
func (sl *StackLog) WithWriter(w io.Writer) *StackLog {
	if w == nil {
		w = os.Stdout
	}

	newLogger := *sl
	newLogger.wr = w

	return &newLogger
}

// ErrShortWrite is returned when the writer accepted fewer bytes than the line.
// By the io.Writer contract a short write is a failure, not a partial success.
var ErrShortWrite = errors.New("stacklog: short write")

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
// It does not return an error because it never returns. A write failure is
// reported to os.Stderr on a best-effort basis, since the configured writer is
// the thing that failed. Deferred functions do not run, and a buffered writer is
// not flushed.
func (sl *StackLog) Fatal(message string) {
	if err := sl.print(FatalLevel, message); err != nil {
		os.Stderr.Write([]byte("stacklog: write failed: " + err.Error() + "\n"))
	}

	os.Exit(1)
}

func (sl *StackLog) addTimestamp() {
	buf := sl.Buffer
	tsStyle := sl.Config.Style.Timestamp

	sl.addPreStyle(tsStyle)

	timeBuf := time.Now().AppendFormat(buf.Buffer[buf.BufLen:buf.BufLen], sl.Config.TimeFormat)
	buf.BufLen += len(timeBuf)
	buf.ResetColor()

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

func (sl *StackLog) print(level LogLevel, message string) error {
	if sl.Config.Timestamp {
		sl.addTimestamp()
	}

	sl.addLevelPrefix(level)

	if len(sl.prefix) > 0 {
		sl.addLoggerPrefix()
	}

	// Adds the raw byte message inside the buffer
	sl.Buffer.WriteString(message)

	if len(sl.params) > 0 {
		sl.addParams(level)
	}

	sl.Buffer.WriteNL()

	sl.addBottomPadding()

	return sl.printBuffer()
}
