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
package stacklog
