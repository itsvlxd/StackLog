// Command example is the showcase. Every section here is one feature the README
// demonstrates, in README order, and each one runs as a real program so a capture
// records what the package actually writes rather than a hand-drawn picture of it.
//
//	go run ./example            all sections, in order
//	go run ./example levels     one section, for one capture
//
// The names accepted here are the names the capture script turns into asset files,
// so a section added here is a capture added for free and a section renamed here is
// a broken capture on purpose rather than a silent one.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/itsvlxd/stacklog"
)

// shortWriter accepts fewer bytes than it is handed, which is the io.Writer
// contract's failure case and the only way to show ErrShortWrite honestly.
type shortWriter struct{ n int }

func (w shortWriter) Write(p []byte) (int, error) { return w.n, nil }

// sections is the single list. README captions, asset file names and the capture
// loop all come from it, so the three cannot drift apart on their own.
var sections = []struct {
	name string
	run  func()
}{
	{"simple", simple},
	{"levels", levels},
	{"params", params},
	{"values", values},
	{"filter", filter},
	{"prefix", prefix},
	{"styles", styles},
	{"presets", presets},
	{"padding", padding},
	{"timestamp", timestamp},
	{"file", file},
	{"write-errors", writeErrors},
	{"json", jsonOutput},
	{"logo", logo},
	{"goroutines", goroutines},
	{"fatal", fatal},
}

func main() {
	// -list exists so the capture script reads its section names from this program
	// instead of keeping its own copy of them. One list, one source of truth.
	if len(os.Args) > 1 && os.Args[1] == "-list" {
		for _, s := range sections {
			fmt.Println(s.name)
		}

		return
	}

	if len(os.Args) > 1 {
		for _, s := range sections {
			if s.name == os.Args[1] {
				s.run()

				return
			}
		}

		fmt.Fprintf(os.Stderr, "no such section: %s\n", os.Args[1])
		os.Exit(2)
	}

	for _, s := range sections {
		s.run()
		fmt.Println()
	}
}

func simple() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.Info("service started, listening on 0.0.0.0:8080")
}

func levels() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.Debug("cache miss for key user:42, going to the database")
	logger.Info("request completed in 12ms")
	logger.Warn("retry 3 of 5 for user:42, the upstream is slow")
	logger.Error("upstream returned 502 for user:42")
	logger.Error("connection reset while reading the response")
}

func params() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	kv := logger.WithParams("request_id", "b7f1c2", "user", "vlad")

	kv.Error("upstream returned 502, retried 3 times")
}

// values shows the types a log line is usually built from. A duration is written
// as nanoseconds because that is what time.Duration is.
func values() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.WithParams("ms", 1.44, "timeout", 250*time.Millisecond, "err", errors.New("connection reset by peer")).
		Error("slow request, retried once")

	logger.WithParams("when", time.Now()).Warn("a time.Time is not a supported value")
}

// filter shows Config.MinLevel dropping calls before the line is built.
func filter() {
	logger := stacklog.CreateLogger(stacklog.Config{MinLevel: stacklog.WarnLevel})

	logger.Debug("cache miss for key user:42")
	logger.Info("request completed in 12ms")
	logger.Warn("retry 3 of 5 for user:42")
	logger.Error("upstream returned 502 for user:42")
}

func prefix() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	kv := logger.WithParams("request_id", "b7f1c2", "user", "vlad")

	kv.WithPrefix("http").Error("handler entered, upstream slow")
}

// styles sets the error level colour and the key/value triple colours on one
// logger, after creation, which is the supported way to reach them.
func styles() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.Config.Style.Level[stacklog.ErrorLevel] = stacklog.NewStyle().Foreground("#D75F5F")

	logger.Config.Style.Params[stacklog.ErrorLevel] = stacklog.ParamsStyle{
		Param: stacklog.NewStyle().Foreground(242),
		Equal: stacklog.NewStyle().Foreground(242),
		Value: stacklog.NewStyle().Foreground(15),
	}

	logger.WithParams("user", "vlad").Error("upstream returned 502 for user:42")
}

// presets shows all four names, and NoColors as a flag rather than a preset.
func presets() {
	for _, preset := range []string{
		stacklog.PresetDefault,
		stacklog.PresetMuted,
		stacklog.PresetBadge,
		stacklog.PresetHighContrast,
	} {
		stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{Preset: preset}}).
			Error("upstream returned 502 for user:42")
	}

	stacklog.CreateLogger(stacklog.Config{
		Style: stacklog.Style{Preset: stacklog.PresetMuted, NoColors: true},
	}).Warn("labels from the preset, no escape codes at all")
}

// padding is the badge case: a coloured block that extends over the spaces
// written inside it.
func padding() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.Config.Style.Level[stacklog.ErrorLevel] = stacklog.NewStyle().
		Background("#D75F5F").
		Foreground(15).
		Padding(stacklog.Padding{Left: 2, Right: 2})

	logger.Error("connection reset while reading the response")
}

func timestamp() {
	logger := stacklog.CreateLogger(stacklog.Config{
		Timestamp:  true,
		TimeFormat: "15:04:05",
	})

	logger.Info("request completed in 12ms")
	logger.Warn("retry 3 of 5 for user:42")
}

// file writes through WithWriter, then reads the result back and quotes it: the same
// shape of call twice, once under the default colour policy and once with colour asked
// for explicitly, so what a log file actually receives is visible rather than implied.
func file() {
	const path = "readme-example.log"

	base := stacklog.CreateLogger(stacklog.Config{})

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open the log file:", err)

		return
	}

	defer f.Close()

	logger := base.WithWriter(f)

	if err := logger.Error("service started, listening on 0.0.0.0:8080"); err != nil {
		fmt.Fprintln(os.Stderr, "write failed:", err)

		return
	}

	// Colour in a file is a decision rather than a property of the writer, and ColorsOn is where
	// that decision is stated. Both lines land in the same file, one with escapes and one without.
	coloured := stacklog.CreateLogger(stacklog.Config{Colors: stacklog.ColorsOn}).WithWriter(f)

	if err := coloured.Error("same call, colour asked for"); err != nil {
		fmt.Fprintln(os.Stderr, "write failed:", err)

		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read failed:", err)

		return
	}

	// Quoted line by line rather than as one blob: the whole point is that a reader can see the
	// escape bytes, and a single quoted run of both lines makes a capture wide enough to render
	// small on a package page.
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}

		fmt.Println("readme-example.log:", strconv.Quote(line+"\n"))
	}
}

// writeErrors shows that a short write is an error, not a partial success.
func writeErrors() {
	logger := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{NoColors: true}})

	if err := logger.WithWriter(shortWriter{n: 4}).Error("the writer accepted fewer bytes than the line"); err != nil {
		fmt.Println("write failed:", err)
	}
}

// json shows the same call in both formats, so the difference is one line rather than two
// programs. The structured line carries no escape codes at all: colours are suppressed by
// the format, not asked for. It is a short message on purpose - a JSON object is roughly twice
// the width of the text line it replaces, and a capture that wide is a capture the README
// renders too small to read.
func jsonOutput() {
	kv := []any{"user", "vlad", "ms", 12.4}

	text := stacklog.CreateLogger(stacklog.Config{}).WithPrefix("http").WithParams(kv...)
	structured := stacklog.CreateLogger(stacklog.Config{Format: stacklog.FormatJSON}).WithPrefix("http").WithParams(kv...)

	text.Error("upstream 502")
	structured.Error("upstream 502")
}

// logo prints the wordmark through Plain: no level label, no prefix, no timestamp, no colour, no
// padding.
//
// The art is neither hand-typed nor generated at run time. It was produced by rendering "stacklog"
// in DejaVu Sans Bold at 420 px, cropping to the ink, adding a margin of at least half a cell on
// every side, then area-resampling the result into cells whose height is twice their width - the
// shape a character has in a terminal - and writing ▄, ▀ or █ according to whether the lower half, the
// upper half, or both of a cell are covered. Three bugs sat between the first attempt and this
// version: a crop tight to the ink put the g descender and the right side bearing exactly on a cell
// boundary; the row count was rounded down, which dropped the descender row outright; and a trim
// pass deleted every fully-empty column, which closed the gutters and merged the word into one
// shape. Only the outer margins are trimmed, because interior whitespace is what makes it read as
// eight letters rather than as a blob.
//
// The finished art is a checked-in constant rather than something regenerated at run time. The bytes
// are the logo, they belong in the source, and a capture that records them records what the package
// writes instead of a picture of it.
// The wordmark, one entry per row. As a single payload it is 1323 bytes against a 1020-byte
// content area, so the section prints it row by row: the fixed buffer does not grow, and a logo that
// arrives clipped is worse than a logo that costs ten writes.
var logoRows = []string{
	`           ▄▄▄                           ███        ▄███`,
	`           ███                           ███        ████`,
	`▄███████ ████████  ████████▄   ▄███████  ███  ▄███▀ ████   ▄██████▄    ▄████▄███`,
	`███▄  ▀▀  ▀███▀▀▀  ▀▀ ▄▄████  ████▀  ▀▀  ███▄███▀   ████  ███▀  ▀███  ███▀  ▀███`,
	`▀███████▄  ███     █████████  ████       ██████▄    ████  ███    ███  ███    ███`,
	`▄▄   ████  ███▄▄▄ ████  ▄███  ████▄  ▄▄  ███▀▀███▄  ████  ███▄▄ ▄███  ███▄▄▄████`,
	`███████▀   ▀█████  ▀████▀███   ▀██████▀  ███  ▀████ ▀███   ▀██████▀    ▀████▀███`,
	`                                                                       ▄▄▄▄▄████`,
	`                                                                       ▀█████▀▀`,
}

// The line under the wordmark.
const logoTagline = `fixed buffer · no growth · one line per log call`

func logo() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	for _, row := range logoRows {
		if err := logger.Plain(row); err != nil {
			fmt.Fprintln(os.Stderr, "could not print the wordmark:", err)

			return
		}
	}

	if err := logger.Plain(logoTagline); err != nil {
		fmt.Fprintln(os.Stderr, "could not print the tagline:", err)
	}
}

// goroutines is the concurrency model on one screen: a base logger, one derived logger per
// goroutine, all four sharing a single writer. Each derived logger owns its own 1024-byte
// buffer, which is the reason this is safe at all - and which is also why the buffer is not the
// shared thing here, the writer is.
//
// The order the lines land in is not a contract. The ownership is.
func goroutines() {
	base := stacklog.CreateLogger(stacklog.Config{})

	var wg sync.WaitGroup

	for _, name := range []string{"db", "http", "cache", "queue"} {
		wg.Add(1)

		go func(name string) {
			defer wg.Done()

			logger := base.WithPrefix(name)

			logger.Info("opening connection pool")
			logger.Warn("retry 1 of 3, upstream slow")
			logger.Info("ready")
		}(name)
	}

	wg.Wait()

	base.Plain("order is not guaranteed. ownership is.")
}

// fatal writes the line and exits with status 1, so it is the last section and
// running every section stops here by design.
func fatal() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.Fatal("shutting down, the port is still in use")
}
