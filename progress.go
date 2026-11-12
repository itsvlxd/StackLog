package stacklog

import (
	"time"
)

// Progress is a bar that hangs off the end of a log line.
//
// Every line this writes is an ordinary stacklog line - timestamp if the logger has one, the level's
// label and colour, the prefix, the message, the key/value pairs - and the bar is what comes at the
// end of it. On a terminal the line replaces itself: carriage return, the line again with a longer bar
// in it, erase-to-end-of-line, no newline written. In a file the same composition lands as a normal
// terminated line, one per update, without the bar characters.
//
// That is a deliberate departure from the other common shape of this feature, where a bar lives on a
// line of its own and the log record is a separate thing. Here there is one kind of output. The
// consequence worth understanding: the bar's lines are log lines, so they are filtered by MinLevel at
// the level ProgressOptions.Level names (DebugLevel by default, which is the level the package already
// treats as "information about work in flight"). Set MinLevel above it and there is no bar at all - not
// a hidden one, none - because a logger that will not write debug lines will not write debug lines.
//
// A redraw is still not a log line, in the way that matters for the fixed buffer and for the cost of a
// write. A frame is due only when the bar or the percentage would actually look different, and never
// more often than Every, so a 5,000,000-iteration loop cannot produce 5,000,000 writes. The number of
// lines a run produces is bounded by the bar width and the 100-step percentage, not by the work.
//
// Frames go only to a writer the package proved was a terminal, which is the answer Config.Colors is
// built on and the same computed verdict Progress reads. ColorsOn permits colour in a file; it does not
// permit control bytes there, and Progress does not draw a bar in a file in either direction.
type Progress struct {
	sl    *StackLog
	total int
	done  int

	level   LogLevel
	message string

	width int
	every time.Duration

	drawn  bool
	filled int
	pct    int
	last   time.Time
	start  time.Time

	// err is the first write error seen. A progress bar cannot retry a failed write on the caller's
	// behalf, and it is not going to invent one either, so the first failure is kept and reported.
	err error

	// params is every key/value pair the bar's lines can carry: the logger's own pairs, then the ones
	// ProgressOptions.Params added, with room in the capacity for the progress numbers, so writing a
	// line needs no allocation and no copy.
	params []any

	// rec holds the progress pairs. The keys are set at construction and never change; only the values
	// move, into the spare capacity of params.
	rec [12]any
}

const (
	// progressWidthDefault is a fixed bar length in cells, and "fixed" is the honest word for it.
	// Asking a terminal how wide it is means ioctl(fd, TIOCGWINSZ) - not available in the Go standard
	// library in portable form, so it would cost either syscall with build tags or a dependency like
	// golang.org/x/term. Both collide with the no-dependency rule, so the package picks a width and says
	// so. ProgressOptions.Width is the escape hatch for a caller who knows their terminal, and #31 is the
	// card that decides whether the package ever learns the width itself.
	progressWidthDefault = 40

	// progressEveryDefault is the redraw budget. 80 ms is about 12 frames a second, which is faster than
	// a bar needs to move to look alive and slow enough that the writes stop being the cost.
	progressEveryDefault = 80 * time.Millisecond

	// progressDefaultMessage is what a bar line says when ProgressOptions.Message is left empty. It is
	// a word a reader can filter on in a log file, which is the point of giving the line a message at
	// all.
	progressDefaultMessage = "progress"

	// The bar glyphs are three bytes each in UTF-8 (U+2588 and U+2591). The design note this feature
	// started from said "two bytes per cell"; the buffer counts bytes, so the clamp uses three.
	barFilled = "█"
	barEmpty  = "░"

	// progressBarOverhead is what a bar line carries besides the cells: the carriage return, the level
	// label with its escapes, the spaces, the counts, the percentage, the elapsed seconds, the style runs
	// around the bar and the "ESC[K" that clears the rest of the line. Each part is sized from the worst
	// case its Append can produce rather than from a guess.
	//
	// It bounds the bar, not the line: content writes stop at the content limit no matter what, and the
	// clearing sequence is written into the reserved tail. A long message plus a long bar cuts the line
	// and keeps its erasing - which is why the clamp is about a useful bar rather than about safety.
	progressBarOverhead = 1 + 1 + 41 + 6 + 23 + 3*(21+4) + 3

	// progressClear is EL - erase in line, from the cursor to the end of the line. It is what makes a
	// shorter line erase the remnants of a longer one, so a bar cannot leave ghosts behind as the counts
	// change width.
	progressClear = "\x1b[K"
)

// ProgressOptions shapes a bar and its lines. Every field falls back to the package default when left
// as the zero value, so WithProgress(logger, 1500, stacklog.ProgressOptions{}) is the whole call for the
// common case.
type ProgressOptions struct {
	// Every is the shortest interval between redraws. Zero means progressEveryDefault.
	Every time.Duration

	// Width is the bar in cells. Zero means progressWidthDefault. Larger values are clamped down to what
	// the content area can carry.
	Width int

	// Level is the level of the bar's lines, and through Style.Level[level] and Style.Params[level] it is
	// also how those lines look. The zero value is DebugLevel, which is what progress is: information
	// about work in flight. MinLevel filters these lines, because they are lines.
	Level LogLevel

	// Message is the text every bar line carries, before the bar. Empty means "progress". The final line
	// takes its own message from Done, Abort or Finish instead, which is the difference between a
	// progress line and a result line.
	Message string

	// Params are key/value pairs these lines carry - the file being written, the job name, whatever
	// context belongs on the bar. The logger's own pairs come first and these come second. The progress
	// numbers are not in this list: a text line prints them once, beside the bar, and a JSON line prints
	// them as fields. See Progress.
	//
	// They are read once, when WithProgress runs. Deriving the logger afterwards does not reach a bar
	// that already exists, which is the same ordering rule WithParams and WithPrefix already follow.
	Params []any
}

// newProgress builds the bar for a logger that WithProgress has already derived. It is unexported
// because a progress bar is always attached to a logger, and the derivation is the part that gives it a
// buffer of its own.
func newProgress(logger *StackLog, total int, opts ProgressOptions) *Progress {
	width := opts.Width
	if width <= 0 {
		width = progressWidthDefault
	}

	// The clamp is arithmetic rather than advisory: it is where a bar stops fitting, and it counts the
	// style runs as well as the cells, because a coloured bar is a longer bar.
	if max := (logger.Buffer.usable() - progressBarOverhead) / 3; width > max {
		width = max
	}

	if width < 1 {
		width = 1
	}

	every := opts.Every
	if every <= 0 {
		every = progressEveryDefault
	}

	message := opts.Message
	if message == "" {
		message = progressDefaultMessage
	}

	p := &Progress{
		sl:      logger,
		total:   total,
		width:   width,
		every:   every,
		level:   opts.Level,
		message: message,
		start:   time.Now(),
	}

	p.params = make([]any, 0, 2*len(logger.params)+2*len(opts.Params)+len(p.rec))
	p.params = append(p.params, logger.params...)
	p.params = append(p.params, opts.Params...)

	p.rec[0], p.rec[2], p.rec[4] = "pct", "done", "total"
	p.rec[6], p.rec[8], p.rec[10] = "ms", "filled", "width"

	return p
}

// Tick advances the bar by n and writes only when a line is due. A tick that is not due is an integer
// add and two comparisons: no allocation, no syscall.
func (p *Progress) Tick(n int) {
	p.done += n

	if p.err != nil {
		return
	}

	// A bar whose level the filter drops does not draw, and does not pretend to have drawn: no state
	// moves, no line is written, and nothing later has to clean up a line nobody saw.
	if !p.sl.Enabled(p.level) {
		return
	}

	// The budget comes first, and a zero last time is the first line's exemption from it. Keeping the
	// budget separate from drawn matters because drawn means something specific - there is an open,
	// unterminated bar line on the reader's screen - and a bar writing lines into a file never opens
	// one. Fold the two into one flag and file milestones stop being budgeted at all.
	if !p.last.IsZero() && time.Since(p.last) < p.every {
		return
	}

	filled := p.filledCells()
	pct := p.percent()

	// A line is due when the bar or the percentage would look different. The elapsed text changes
	// constantly and is not on its own worth a syscall, which is the part of this a reader has to trust:
	// a bar that refreshes its seconds every millisecond is a benchmark of the terminal.
	due := !p.drawn || filled != p.filled || pct != p.pct
	if !due {
		return
	}

	p.filled, p.pct = filled, pct

	p.err = p.write(p.level, p.message, false)

	p.last = time.Now()
}

// Done ends the run and writes the result line at InfoLevel: the message the caller gives, the params
// the logger and the caller supplied, the four progress numbers, and a completed bar at the end of it.
// On a terminal it replaces the line the bar has been redrawing rather than starting a new one, so what
// stays on screen is one line that finished.
//
// It is Finish with the level chosen for you, which is the whole reason it exists: a finished job is an
// information event.
func (p *Progress) Done(message string) error {
	return p.Finish(InfoLevel, message)
}

// Abort ends the run with the same shape of line at ErrorLevel, for work that did not finish. It exists
// because a bar that stops early should leave a line that says so rather than half a line and a silence.
func (p *Progress) Abort(message string) error {
	return p.Finish(ErrorLevel, message)
}

// Finish ends the run at the level the caller names. In this package the level is how a line gets its
// look - Style.Level[level] and Style.Params[level] are keyed by level - so naming one is also the way
// to make the result of a progress run read as a warning. "Finished with problems in it" is the case
// that asks for WarnLevel, and it is a different line from either Done or Abort.
//
// The bar is terminated here whatever the level says, including a level this logger would filter out:
// leaving an unterminated line on someone's screen is not something a level filter gets to decide. The
// *record* obeys the filter, because the record is a line and MinLevel is what lines do.
func (p *Progress) Finish(level LogLevel, message string) error {
	filled, pct := p.filledCells(), p.percent()

	if filled != p.filled || pct != p.pct {
		p.filled, p.pct = filled, pct
	}

	err := p.write(level, message, true)

	if err != nil && p.err == nil {
		p.err = err
	}

	return p.err
}

// write puts one bar line through the same line builder every other line in this package uses, with the
// bar hung off the end of the message.
//
// terminate matters most on a terminal. A frame is written without a terminator so the next frame can
// replace it; the last line of a run is written with one, so the bar ends as a line in the reader's
// history instead of a line the next thing the program writes lands on.
func (p *Progress) write(level LogLevel, message string, terminate bool) error {
	sl := p.sl

	if !sl.Enabled(level) {
		// A bar drawn at a level the filter lets through must not be left unterminated by a result line
		// at a level it does not. An orphan line on someone's terminal is not something a level filter
		// gets to cause, so the line is ended and nothing else is written.
		if terminate && p.redraws() && p.drawn {
			p.drawn = false

			return p.terminateLine()
		}

		return nil
	}

	live := p.redraws()

	// Where the numbers live is the one place the two formats differ, and the rule is that they live
	// once. A text line says "2/12 16% 0.1s" beside the bar and nowhere else, because a person is
	// reading it and a line that states the same number twice reads as noise. A JSON object has no bar
	// and no tail, so the same facts arrive as fields - which is the only way a parser can use them, and
	// boxing them is the format's cost rather than progress's.
	line := p.params

	if sl.Config.Format == FormatJSON {
		p.rec[1] = p.percent()
		p.rec[3] = p.done
		p.rec[5] = p.total
		p.rec[7] = int64(time.Since(p.start) / time.Millisecond)
		p.rec[9] = p.filledCells()
		p.rec[11] = p.width

		// The append lands in the spare capacity Progress built for itself at construction, sized for
		// exactly this: the logger's pairs, the options' pairs, and six pairs of progress numbers.
		// Appending onto the logger's own
		// slice instead would write into its backing array, and a derived logger can share that array -
		// so the numbers get added here, where the ownership is clear.
		line = append(p.params, p.rec[:]...)
	}

	saved := sl.params
	sl.params = line

	err := sl.printLine(level, message, p.tail, live, terminate)

	sl.params = saved

	// drawn means one thing: there is an open, unterminated bar line on the reader's screen. Only a
	// line that was actually written can open one, and a terminating line closes it. That invariant is
	// what keeps a filtered result from writing a newline nobody asked for and from leaving a bar that
	// was never drawn looking like one that was.
	if live {
		p.drawn = !terminate
	}

	return err
}

// redraws asks the logger it writes through, every time, whether this output is going somewhere a line
// can be replaced. It is a question rather than a field because the answer belongs to the writer, and a
// bar that cached it at construction would be a bar that could be wrong about its own output.
//
// Terminal verdict and text format, both: a JSON stream has nowhere to redraw a bar and a percentage
// string is not data in that format, so the progress numbers travel as params there instead.
func (p *Progress) redraws() bool {
	return p.sl.isTTY && p.sl.Config.Format != FormatJSON
}

// terminateLine ends the line a bar has been redrawing, with nothing on it but the terminator. It is
// the piece that keeps a filtered result from leaving a bar sitting unterminated on the reader's screen.
func (p *Progress) terminateLine() error {
	p.drawn = false

	p.sl.Buffer.Reset()
	p.sl.Buffer.WriteNL()

	return p.sl.printBuffer()
}

// tail is the bar itself, appended after the message and the params. Three runs - filled cells,
// remainder, numbers - because those three things want to look different, and each one goes through the
// same hasColor gate every other component in this package goes through, which is what makes NoColors and
// ColorsOff work on a bar without Progress knowing anything about the colour policy.
//
// The bar is drawn wherever a text line is written, terminal or file. On a terminal the line replaces
// itself, so one bar is on screen at a time. In a file every row carries its own bar, because that is the
// only honest way to put a progress bar somewhere that cannot redraw: the file becomes the history of the
// run row by row, and a reader scrolling it watches the bar lengthen. That costs 120 bytes a row at the
// default width, which is the price of the shape and is stated rather than discovered - the redraw budget
// is what keeps it from becoming 120 bytes times a million.
//
// The clearing sequence belongs to the redraw and nowhere else. It is what erases the remnants of the
// longer line before it, and a file line has nothing to erase.
func (p *Progress) tail(buf *StackBuffer) {
	style := p.sl.Config.Style.Progress

	if p.filled > 0 {
		p.sl.framePre(style.Filled)

		for i := 0; i < p.filled; i++ {
			buf.WriteString(barFilled)
		}

		p.sl.framePost(style.Filled)
	}

	if rest := p.width - p.filled; rest > 0 {
		p.sl.framePre(style.Empty)

		for i := 0; i < rest; i++ {
			buf.WriteString(barEmpty)
		}

		p.sl.framePost(style.Empty)
	}

	// A space between the bar and its numbers, so the counts do not grow out of the last cell.
	buf.AppendByte(' ')

	p.sl.framePre(style.Numbers)
	p.writeCounts(buf)
	p.sl.framePost(style.Numbers)

	if p.redraws() {
		// The clearing sequence goes past the content limit, into the reserved tail, so a line cut short
		// still erases what the previous, longer one left behind.
		buf.writeTail(progressClear)
	}
}

// writeCounts is the numbers half: how much is done, how much there is, what percentage that is, and how
// long it has taken. The total is only printed when there is one, because "17/0" is not a fraction.
func (p *Progress) writeCounts(buf *StackBuffer) {
	if p.total > 0 {
		buf.AppendUint(uint64(p.done)).AppendByte('/').AppendInt(int64(p.total)).AppendByte(' ')
	}

	buf.AppendInt(int64(p.pct)).AppendByte('%').AppendByte(' ')

	p.appendElapsed(buf)

	buf.AppendByte('s')
}

// appendElapsed writes whole seconds and one decimal, in fixed point. The generic float writer produces
// the shortest representation that round-trips, which is exactly right for a measured benchmark value and
// wrong for a line a human is watching: the first version of this printed "1.789e-06s" in the middle of a
// progress bar, which is not an elapsed time, it is a notation accident.
func (p *Progress) appendElapsed(buf *StackBuffer) {
	d := time.Since(p.start)

	buf.AppendInt(int64(d / time.Second)).AppendByte('.')
	buf.AppendInt(int64((d % time.Second) / (100 * time.Millisecond)))
}

func (p *Progress) percent() int {
	if p.total <= 0 {
		return 0
	}

	pct := p.done * 100 / p.total
	if pct > 100 {
		return 100
	}

	return pct
}

func (p *Progress) filledCells() int {
	if p.total <= 0 {
		return 0
	}

	if p.done >= p.total {
		return p.width
	}

	return p.done * p.width / p.total
}

// Logger returns the derived logger this bar writes through, so a caller can keep deriving from it -
// a prefix per stage, params per file - without the bar's own lines being filtered out by MinLevel and
// without two bars sharing one buffer.
//
// It is the same pointer the bar was built with. Calling WithProgress twice on the same logger gives two
// bars, each with its own derived logger and its own buffer, and never two bars on one line.
func (p *Progress) Logger() *StackLog {
	return p.sl
}
