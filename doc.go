// Package stacklog is a console logger that writes into a fixed, pre-allocated
// buffer rather than building strings per call.
//
// Values are formatted straight into the buffer, which never grows. The design
// goal is to keep as much work as possible on the stack, and the honest limit of
// that goal is worth stating: any value passed as any is boxed, and boxing
// allocates on the heap. Escape analysis is per call site, so a pointer stays on
// the stack only while it never leaves the frame that created it. What this
// package actually guarantees is a fixed buffer, no growth, and no allocation on
// the per-line path — not zero allocation everywhere.
//
// A line is built in order: timestamp, level, prefix, message, key/value pairs,
// then the line ending. Padding follows the same order, with one rule. Top only
// applies at the start of a line, and Bottom is never emitted mid-line — it is
// accumulated as the maximum across the line and flushed after the line ends.
// Left and Right are spaces written inside the coloured block, so a badge with a
// background extends over them.
//
// A style can be set component by component, or taken from a named preset. A preset
// is a complete style: colours, padding, and the level labels. CreateLogger applies
// the preset only to the components left as the zero value, so a single component
// override stands against the whole preset. An unknown name resolves to the default.
//
// NoColors is a flag, not a preset. It suppresses every escape sequence for any
// preset, and it wins over an explicit component - a caller who sets a colour and
// sets NoColors gets no colour. That is the precedence, and it is asserted in a
// test so it cannot change silently.
//
// Colour is also a decision about the writer, taken once when the writer becomes
// known. Config.Colors takes ColorsAuto, ColorsOn or ColorsOff, and the empty string
// is ColorsAuto: escape sequences are written only when the writer is standard output
// or standard error attached to a character device, which is what keeps a pipe and a
// log file free of codes nobody asked for. ColorsOn writes colour into a file anyway,
// ColorsOff never writes any, and an unrecognised name resolves to ColorsAuto the way
// an unrecognised preset name resolves to the default preset. NoColors outranks all
// three and can only ever turn colour off. The rule is "standard output or standard
// error, on a character device" rather than "on a character device", because /dev/null
// is a character device and would otherwise be called a terminal. A bufio.Writer around
// a terminal is not an *os.File, so it reads as not a terminal: wrapping a writer takes
// the decision out of the package and hands it to the caller.
//
// A logger is cheap to derive. WithParams, WithPrefix and WithWriter return new
// loggers and never modify the original, and each derived logger owns its own
// buffer, so one base logger can be shared and specialised per call site.
// Because the copy takes its own Config, set the style before deriving, not
// after.
//
// Derivation is also what makes concurrent use work. A logger per goroutine is
// safe; the same logger called from two goroutines is not, and no lock guards a
// logger. A derived logger shares its writer with whatever it was derived from,
// so that writer has to be safe for concurrent use in its own right - os.Stdout
// is, a bytes.Buffer shared between loggers is not.
//
// Levels ascend in severity: debug, info, warn, error, fatal. Config.MinLevel drops
// every call below it, before anything is written into the buffer, and its zero value
// is DebugLevel — a Config that says nothing about levels writes everything. Because
// severity ascends, a filter keeps its own level and everything worse, so it cannot
// hide a problem more serious than the one it keeps. The filter is copied along with
// Config when a logger is derived, so setting it at creation reaches every derived
// logger, and changing it on the base afterwards does not reach the copy.
//
// Format switches what one call writes. FormatJSON emits one JSON object per call, newline
// terminated, and it is why the fixed buffer has two content areas: 1020 bytes for a text line
// against 1004 for a JSON one. A JSON line is only worth reading if the structure that opens it
// is always closed, so the wider tail is reserved rather than hoped for. Colours are suppressed
// through NoColors rather than beside it, Config.Colors reads back as ColorsOff for the same
// reason, padding and Style.LevelPrefix are inert, and the level
// is a field with a fixed set of values. Entry is exported, so the shape of a line is part of the
// API rather than something a caller reverse-engineers from an example.
//
// A JSON line that will not fit is cut where a cut cannot break it: the msg value is cut inside
// its own string, ends with "...", and the object gains "truncated":true. A key/value pair that
// will not fit is dropped whole - never half a pair, never a dangling "key":. Either way the
// reader is told, which is what separates a complete line from a line that lost something.
//
// Plain output is a third thing, not a third format. Plain writes the payload with no level
// label, no prefix, no timestamp, no params, no colour and no padding - a banner, an ASCII
// wordmark, help text - through the same fixed buffer and the same single Write. It is not a log
// level, so MinLevel does not filter it, and it is not wrapped by FormatJSON: a banner turned
// into a JSON string is useless to the person it was printed for. A JSON stream that also prints
// a banner therefore contains one line that is not an object, which is stated here because the
// alternative is a parser failing on it in the dark.
//
// A progress bar is a log line with a bar at the end of it. WithProgress derives a logger and returns a
// bar whose lines go through the same builder every other line in this package uses - timestamp if the
// logger has one, the level's label and colour, the prefix, the message, the key/value pairs - with the
// bar as the tail. On a terminal the line replaces itself: carriage return, the line again with a longer
// bar in it, erase-to-end-of-line, and no newline until the run ends. In a file the same composition is a
// normal terminated line, one row per update, and every row carries its own bar - a file cannot replace a
// line, so the file becomes the history of the run and the bar lengthens down the page. That costs about
// 120 bytes a row at the default width, which is the price of the shape rather than an accident, and the
// redraw budget is what keeps it from becoming 120 bytes times a million rows.
//
// Because the lines are log lines, they have a level (ProgressOptions.Level, DebugLevel by default) and a
// message (ProgressOptions.Message, "progress"), and MinLevel applies to them: a filtered level means no
// bar, not a hidden one. The asymmetry is deliberate - a bar that has drawn lines is terminated even when
// its result line is filtered, because a level filter does not get to leave half a line on someone's
// screen. Done, Abort and Finish are the endings, and Finish is for the run where info and error are both
// the wrong level.
//
// The numbers appear once, in the form each format can use. A text line prints them beside the bar -
// "600/1500 40% 8.2s" - and nowhere else. A JSON line prints one object per update carrying pct, done,
// total, ms, filled and width: a bar is a rendering rather than data, so the two numbers that draw it
// travel instead of 120 bytes of box-drawing characters inside something meant for a machine.
//
// Redraws are budgeted rather than counted: a line is due when the filled cells or the whole-number
// percentage would look different, and never more often than ProgressOptions.Every, 80 ms by default.
// Bars go only to a writer the package proved was a terminal - the same verdict Config.Colors is built
// on, computed once per writer - so ColorsOn permits colour in a file and never permits control bytes
// there. JSON mode draws no bar anywhere.
//
// Style.Progress carries the bar's colours in three runs - filled, empty and numbers - each an ordinary
// CompStyle, and every preset supplies one: the package default is a green bar on the same 242 grey the
// timestamp and the keys already use. NoColors and ColorsOff gate the colours and not the clearing
// sequence, because a bar that stops erasing the longer line before it leaves ghosts. Left and Right
// padding are honoured inside a bar line; Top and Bottom are not, because they write newlines and a
// redraw needs its line to stay one line.
//
// The bar is 40 cells by default and clamped to what the content area can carry - 290 with a coloured
// bar, because a coloured bar is a longer bar - and its clearing sequence is written into the reserved
// tail so a cut line still erases cleanly. Terminal width is an ioctl rather than bytes, so the package
// does not know it and says so: ProgressOptions.Width is the caller's lever, and #31 is the card that
// decides whether that ever changes. A tick that is not due costs 13.0 ns and allocates nothing, a
// redrawn bar line costs 166.9 ns uncoloured and 204.6 ns with the preset's bar colours, and neither one
// allocates. Ending a run costs 707.9 ns and four allocations, which is the derived logger, its own
// 1,024-byte buffer, the Progress object and its params, paid once per operation rather than per line.
//
// The writer is the caller's choice, and it is where the cost of a log line lives.
// One line is built in a fixed buffer and handed to a single Write call, so the
// formatting is the smaller half: about 65 ns to build a coloured line against roughly
// 390 ns to write it to a file one syscall at a time, or about 166 ns once a buffered
// writer is supplied to WithWriter. About 25 ns of those 65 is emitting colour - the
// same line with the escapes suppressed builds in 40.7 ns - which is why both colour writers assemble
// their escape sequences in a stack array, and why the JSON escape writer copies runs of
// ordinary text in one move rather than deciding one character at a time. A call MinLevel
// drops costs under a nanosecond and allocates nothing, in either format. A caller who wants
// throughput supplies a buffered writer to WithWriter. The package does not hold that buffer
// itself — a logger owning a heap-allocated write buffer contradicts the fixed per-line
// buffer the package exists to provide. Fatal flushes a writer that can be flushed, because
// os.Exit skips the caller's deferred flush and the crash line is the one worth
// keeping.
package stacklog
