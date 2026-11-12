package stacklog

import (
	"io"
	"os"
)

// Colors selects when this logger writes escape sequences at all. It matches how Preset and Format
// take a name: a string, an exported constant, and an unrecognised value that resolves to the
// default rather than failing.
//
// The zero value is the empty string, which means ColorsAuto - and ColorsAuto is the behaviour
// change this file exists for. Before it, a Config wrote coloured output into a pipe, a file and a
// terminal alike, and the only answer was for the caller to remember Style.NoColors. Now colour
// goes where a terminal is, and ColorsOn is how you ask for it anywhere else.
const (
	ColorsAuto = "auto"
	ColorsOn   = "on"
	ColorsOff  = "off"
)

// resolveColor answers the colour question once, at the point where the writer is known, and the
// answer is stored on the logger rather than recomputed per line. Order matters and each rule is
// reachable from the outside:
//
//  1. Style.NoColors is the absolute kill switch it has always been. It wins over Colors and it
//     can never turn colour on, which is what its current tests promise.
//  2. ColorsOff never writes an escape, whatever the writer is.
//  3. ColorsOn always writes one, whatever the writer is - a file, a pipe, /dev/null.
//  4. ColorsAuto, the empty string and any unrecognised name fall through to the writer.
func resolveColor(config Config, w io.Writer) bool {
	if config.Style.NoColors {
		return false
	}

	switch config.Colors {
	case ColorsOn:
		return true
	case ColorsOff:
		return false
	default:
		return isTerminal(w)
	}
}

// isTerminal reports whether w is standard output or standard error attached to a terminal.
//
// Both halves of the test are needed and the reason is measurable. ModeCharDevice on its own calls
// /dev/null a terminal, because /dev/null is a character device - so a rule built on that bit alone
// paints escape sequences into the bit bucket and reports success. Requiring the descriptor to be
// 1 or 2 says "this is the program's output", and requiring the character-device bit says "that
// output is attached to something that interprets it". A pipe fails the second test, a redirected
// file fails it too, and a terminal passes both.
//
// Nothing here sees through a wrapper. A bufio.Writer around os.Stdout is not an *os.File, so it
// reports "not a terminal" - and that is a stated limit rather than a bug: bufio.Writer is the
// writer the README recommends for throughput, so wrapping a writer takes this decision away from
// the package and hands it to the caller, who has ColorsOn and ColorsOff for the purpose.
//
// The fd numbers are constants on purpose. os.File.Fd() is documented as the way to reach the
// descriptor for a system call, and it works on every platform this package compiles on, which is
// more than can be said for the terminal-specific calls that would be needed to do this properly.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	if fd := f.Fd(); fd != 1 && fd != 2 {
		return false
	}

	stat, err := f.Stat()
	if err != nil {
		return false
	}

	return stat.Mode()&os.ModeCharDevice != 0
}
