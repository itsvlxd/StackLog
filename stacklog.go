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
	levelPrefix  [MaxLevel]string  = [MaxLevel]string{"DEBU", "INFO", "WARN", "ERRO", "FATA"}
	levelFgColor [MaxLevel]any     = [MaxLevel]any{5, 6, 3, 1, 1}
	levelBgColor [MaxLevel]any     = [MaxLevel]any{ColorReset, ColorReset, ColorReset, ColorReset, ColorReset}
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
type Style struct {
	Timestamp CompStyle
	Level     [MaxLevel]CompStyle
	Params    [MaxLevel]ParamsStyle
}

// ParamsStyle is the colour set for one key/value pair: the key, the separator,
// and the value.
type ParamsStyle struct {
	Param CompStyle
	Equal CompStyle
	Value CompStyle
}

// Predefined styles
var whiteFGStyle = NewStyle().
	Background(0).
	Foreground(15)

var grayFGStyle = NewStyle().
	Background(0).
	Foreground(242)

// CreateLogger returns a logger for the given config. Every part left empty is
// filled with the package default for that component.
//
// The returned logger writes to os.Stdout.
func CreateLogger(config Config) *StackLog {
	var sb *StackBuffer = CreateBuffer()

	// Init default values
	if config.TimeFormat == "" {
		config.TimeFormat = time.DateTime
	}

	if (config.Style.Timestamp == CompStyle{}) {
		config.Style.Timestamp = NewStyle().
			// Padding(Padding{Left: 1, Right: 1}).
			Foreground(242)
	}

	for i := LogLevel(0); i < MaxLevel; i++ {
		if (config.Style.Level[i] == CompStyle{}) {
			config.Style.Level[i] = NewStyle().
				Padding(levelPadding[i]).
				Background(levelBgColor[i]).
				Foreground(levelFgColor[i])
		}

		if i > 2 {
			switch true {
			case levelFgColor[i] != ColorReset:
				if config.Style.Params[i] == (ParamsStyle{}) {
					config.Style.Params[i] = ParamsStyle{
						Param: NewStyle().
							Background(0).
							Foreground(levelFgColor[i]),
						Equal: grayFGStyle,
						Value: whiteFGStyle,
					}
				}
			case levelBgColor[i] != ColorReset:
				if config.Style.Params[i] == (ParamsStyle{}) {
					config.Style.Params[i] = ParamsStyle{
						Param: NewStyle().
							Background(0).
							Foreground(levelBgColor[i]),
						Equal: grayFGStyle,
						Value: whiteFGStyle,
					}
				}
			}
		} else {
			if config.Style.Params[i] == (ParamsStyle{}) {
				config.Style.Params[i] = ParamsStyle{
					Param: grayFGStyle,
					Equal: grayFGStyle,
					Value: whiteFGStyle,
				}
			}
		}

	}

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
	buf.WriteString(levelPrefix[level])
	sl.addPostStyle(levelStyle)

	buf.WriteBlank()
}

func (sl *StackLog) addLoggerPrefix() {
	buf := sl.Buffer

	sl.addPreStyle(grayFGStyle)
	buf.WriteString(sl.prefix + ":")
	sl.addPostStyle(grayFGStyle)

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
