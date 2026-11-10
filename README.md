<div align="center">

<img src="assets/stacklog.png" alt="stacklog" width="520">

A console logger for Go built on a fixed 1024-byte buffer. No string building, no
growth, no allocation on the per-line path.

<p align="center">
  <img alt="Release" src="https://img.shields.io/github/v/release/itsvlxd/stacklog?style=for-the-badge&labelColor=0a0a0a&color=00ADD8&prefix=v">
  <img alt="Go 1.27" src="https://img.shields.io/badge/Go-1.27-00ADD8?style=for-the-badge">
  <img alt="CI" src="https://img.shields.io/github/actions/workflow/status/itsvlxd/stacklog/tests.yml?style=for-the-badge&labelColor=0a0a0a&label=Tests&color=00ADD8">
  <a href="https://pkg.go.dev/github.com/itsvlxd/stacklog"><img alt="pkg.go.dev" src="https://img.shields.io/badge/pkg.go.dev-reference-00ADD8?style=for-the-badge"></a>
</p>

</div>

`stacklog` writes one line per call into a fixed, pre-allocated buffer and flushes
it in a single write. Values are formatted directly into the buffer instead of
being converted to strings first.

The API is deliberately small: a logger, a config, key/value pairs, a prefix,
colours, and padding. There is no reflection, no field marshalling, and no
dependency outside the standard library.

## Features

- Fixed 1024-byte buffer, no growth, no reallocation
- A line longer than the content area is cut and marked with `...`, and it keeps its
  terminator so two log calls never merge
- Zero allocations on the per-line path
- Leveled logging: debug, info, warn, error, fatal
- Key/value pairs written as `key=value`
- Per-level colours and per-pair styles
- Named style presets, with a single component override standing against the preset
- Colour index, 256-colour, truecolour hex, and background colours
- Padding around a line, including coloured blocks
- Optional prefix and timestamp
- Configurable writer — file, buffer, `io.Discard`
- Write errors returned, including short writes

## Installation

```bash
go get github.com/itsvlxd/stacklog
```

## Getting started

The images below are terminal captures of the examples on this page, run as one
session from a single service.

### Simple logging example

```go
package main

import "github.com/itsvlxd/stacklog"

func main() {
	logger := stacklog.CreateLogger(stacklog.Config{})

	logger.Info("service started, listening on 0.0.0.0:8080")
}
```

![Simple logging example](assets/info-service-started.png)

> By default the logger writes to `os.Stdout`.

### Leveled logging

```go
logger := stacklog.CreateLogger(stacklog.Config{})

logger.Debug("cache miss for key user:42, going to the database")
logger.Info("request completed in 12ms")
logger.Warn("retry 3 of 5 for user:42, the upstream is slow")
logger.Error("upstream returned 502 for user:42")
logger.Error("connection reset while reading the response")
```

![Leveled logging](assets/level-logging.png)

Levels are: `DebugLevel`, `InfoLevel`, `WarnLevel`, `ErrorLevel`, `FatalLevel`.

There is no global level filter. Every call writes. If you want filtering, decide
it before you call the logger.

### Key/value context

Arguments are read in pairs — key, value, key, value — and rendered as
`key=value`.

```go
kv := logger.WithParams("request_id", "b7f1c2", "user", "vlad")

kv.Error("upstream returned 502, retried 3 times")
```

![Key/value context](assets/upstream-502-error.png)

Supported value types: every `int` and `uint` width, `float32`, `float64`, `bool`,
`string`, `error`, and `time.Duration`. A `time.Duration` is written as nanoseconds,
because that is what it is — an `int64` counted in nanoseconds — so
`1500 * time.Microsecond` writes `1500000`. `float32` is written in the shortest form
that reads back as the same `float32`, so `float32(1.1)` writes `1.1` and not
`1.100000023841858`. A nil value writes `<nil>`. Anything else is written as
`<unsupported>`.

```go
kv.WithParams("when", time.Now()).Error("a time.Time is not a supported value")
```

> No capture for this section yet. `assets/slow-req-error.png` shows a `float64`
> written as `<unsupported>`, and that stopped being true the moment `float64` became
> a supported type. #23 regenerates the whole set from one tape.

> An odd number of arguments is not validated. The last value is written with no
> key.

### Prefix

```go
prefixed := kv.WithPrefix("http")

prefixed.Error("handler entered, upstream slow")
```

![Prefix](assets/http-prefix-error.png)

`WithParams`, `WithPrefix` and `WithWriter` return new loggers. The original is
never modified, so a base logger can be shared and specialised per call site.

> Each derived logger owns its own buffer, so deriving one per goroutine is the
> safe pattern. Calling the same logger from two goroutines is not safe, and no
> lock guards a logger. A derived logger shares its writer, so a writer used by
> several loggers has to be safe for concurrent use in its own right: `os.Stdout`
> is, a `bytes.Buffer` is not.

> Because the copy takes its own `Config`, a style change made after the copy does
> not reach the copy. Set the style first, then derive the logger.

### Colours and styles

Three accepted forms:

```go
stacklog.NewStyle().Foreground(242)                  // 256-colour index
stacklog.NewStyle().Foreground(stacklog.ColorRGB{R: 171, G: 205, B: 239})
stacklog.NewStyle().Foreground("#D75F5F")            // truecolour hex
stacklog.NewStyle().Background(stacklog.ColorReset)
```

A style can be set through `Config` at creation, or on the logger after. A
component left as the zero value falls back to the named preset, which is the
package palette when no preset is given.

Per-level and per-pair styles:

```go
styled := stacklog.CreateLogger(stacklog.Config{})

styled.Config.Style.Level[3] = stacklog.NewStyle().Foreground("#D75F5F")

styled.Config.Style.Params[3] = stacklog.ParamsStyle{
	Param: stacklog.NewStyle().Foreground(242),
	Equal: stacklog.NewStyle().Foreground(242),
	Value: stacklog.NewStyle().Foreground(15),
}

styled.WithParams("user", "vlad").Error("upstream returned 502 for user:42")
```

![Colours and styles](assets/upstream-502-error-params.png)

> The key and the value are in different colours here, which is what the
> `ParamsStyle` block controls.

> `HexToRGB` accepts only `#RRGGBB`. Any other input returns white. `Color(0)` is
> black, not a sentinel — the "not set" state is carried separately, so a style
> that sets nothing emits no escape sequence.

### Presets

A preset is a complete style: colours, padding, and the level labels. Pick one by
name, and override a single component on top of it.

```go
logger := stacklog.CreateLogger(stacklog.Config{
	Timestamp: true,
	Style: stacklog.Style{
		Preset: stacklog.PresetBadge,
	},
})

logger.Error("upstream returned 502 for user:42")
```

Four names, and an unknown name resolves to `PresetDefault`. Every preset carries a palette.

| Preset | What it is |
|---|---|
| `PresetDefault` | grey timestamp and prefix, coloured level text, four-letter labels |
| `PresetMuted` | low-chroma palette, full-word labels. For long-running services and for output that ends up in a file |
| `PresetBadge` | the level as a block — background colour, white text, padding inside the block, three-letter labels |
| `PresetHighContrast` | bright colours for legibility, four-letter labels |

`NoColors` is a flag, not a preset. It applies to any preset and writes no escape
sequence at all:

```go
logger := stacklog.CreateLogger(stacklog.Config{
	Style: stacklog.Style{Preset: stacklog.PresetMuted, NoColors: true},
})

logger.Warn("labels from the preset, no colour codes, no reset codes")
```

The flag wins over an explicit component. A caller who sets a colour and sets
`NoColors` gets no colour. That is the precedence, and `TestNoColorsWinsOverAnExplicitStyle`
asserts it so it cannot change silently.

Padding is not affected by the flag. `Left` and `Right` exist to sit inside a
coloured block, and with no colour there is no block, so they become plain
whitespace. That is an open decision, not a solved one.

A component you set yourself wins. The preset fills only what is left empty, so
one override changes one component and the rest of the preset stands.

```go
styled := stacklog.CreateLogger(stacklog.Config{
	Style: stacklog.Style{
		Preset: stacklog.PresetBadge,
		Level: [stacklog.MaxLevel]stacklog.CompStyle{
			stacklog.NewStyle().Foreground("#FFFFFF").Background("#D75F5F"),
		},
		LevelPrefix: [stacklog.MaxLevel]string{"!!"},
	},
})

styled.Error("only level 0 is the override, the rest comes from the preset")
```

The logger prefix is styled through `Style.Prefix`. It used to be hardcoded, so a
preset could not reach it.

> A preset cannot express an intentionally empty label, because an empty string
> means "not set". Same class of problem as the colour sentinel.

> Indices 8 to 15 take the bright ANSI range — 90 to 97 for the foreground and
> 100 to 107 for the background — and 16 and above are written through the
> 256-colour form. `PresetHighContrast` chooses from the 256-colour range.

> The `PresetMuted` labels are full words — `debug`, `info`, `warn`, `error`,
> `fatal` — so they are not the same length and the message column shifts between
> levels. Accepted here: a muted line is not meant to be aligned.

> `ParamsStyle` is guarded as a whole triple, not as `Param`, `Equal` and `Value`
> separately. Setting one of them means the other two stay empty and emit nothing.

### Padding

`Top`, `Left`, `Bottom`, `Right`. Left and Right are spaces written inside the
coloured block, so a badge with a background extends over them.

```go
badge := stacklog.CreateLogger(stacklog.Config{})

badge.Config.Style.Level[3] = stacklog.NewStyle().
	Background("#D75F5F").
	Foreground(15).
	Padding(stacklog.Padding{Top: 1, Left: 1, Right: 1, Bottom: 1})

badge.Error("connection reset while reading the response")
```

![Padding](assets/connection-reset-error.png)

> Top is honoured only for the first component on the line. Bottom is never
> emitted mid-line — it is accumulated as the maximum across the line and flushed
> after the line ends.

### Timestamp

```go
ts := stacklog.CreateLogger(stacklog.Config{
	Timestamp:  true,
	TimeFormat: "15:04:05",
})

ts.Info("request completed in 12ms")
```

![Timestamp](assets/info-request-timestamp.png)

`TimeFormat` is a Go reference-time layout. When it is empty, `time.DateTime` is
used.

> With a timestamp enabled, the timestamp is the first component, so `Top` padding
> has to be requested on the timestamp style. Setting it on the level style does
> nothing, because the line-start state is already consumed.

### Writing to a file

```go
package main

import (
	"os"

	"github.com/itsvlxd/stacklog"
)

func main() {
	base := stacklog.CreateLogger(stacklog.Config{})

	f, err := os.OpenFile("readme-example.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		os.Stderr.WriteString("could not open the log file: " + err.Error() + "\n")
		return
	}
	defer f.Close()

	logger := base.WithWriter(f)

	if err := logger.Error("service started, listening on 0.0.0.0:8080"); err != nil {
		os.Stderr.WriteString("write failed: " + err.Error() + "\n")
	}
}
```

![Writing to a file](assets/wrote-line-readme-log.png)

Contents of `readme-example.log`:

```text

\x1b[31mERRO\x1b[31m\x1b[0m service started, listening on 0.0.0.0:8080

```

> There is no terminal detection. Colours are written unless `Style.NoColors` is
> set, so a log file contains ANSI codes by default. For plain output, set
> `NoColors`.

### Handling write errors

`Debug`, `Info`, `Warn` and `Error` return the write error. A short write is
returned as `ErrShortWrite`, because by the `io.Writer` contract a short write is
a failure, not a partial success.

```go
if err := logger.Error("upstream returned 502 for user:42"); err != nil {
	os.Stderr.WriteString("write failed: " + err.Error() + "\n")
}
```

```text
write failed: stacklog: short write
```

> No capture for this section yet. Add `assets/handling-write-errors.png` and it
> will render here.

> The buffer is reset after a failed write, so one failure does not corrupt the
> following lines. A short write also means the padding never reaches the writer,
> because the flush failed before it got there.

### Fatal

```go
logger.Fatal("shutting down, the port is still in use")
```

![Fatal](assets/shutting-down-fatal.png)

`Fatal` writes the line, flushes the writer if it can be flushed, then calls
`os.Exit(1)`. It does not return an error, because it never returns. A write failure
is reported to `os.Stderr` on a best-effort basis. Deferred functions do not run,
which is why the flush happens here instead of being left to your `defer`.

### Throughput and buffering

The line takes about 64 ns to build. Written to a real file one syscall at a time,
the same call costs 233 ns — roughly three quarters of a log line is the write.
A buffered writer is the whole difference, and `WithWriter` already takes one:

```go
bw := bufio.NewWriterSize(f, 256*1024)
defer bw.Flush()

logger := stacklog.CreateLogger(stacklog.Config{}).WithWriter(bw)
```

Measured on the same line on the same machine: 233 ns unbuffered, 33 to 55 ns
buffered, `0 allocs` either way.

Three consequences, each one a change to what the logger promises.

- **Errors move to the flush.** `logger.Error(...)` returns nil once the bytes are
  buffered. The real failure surfaces at `Flush`, and only if you check it.
- **Decide the latency you are accepting.** Buffered output leaves when the buffer
  fills, so a quiet service can hold its last lines for a long time. If that matters,
  flush on a timer as well as on size — zap defaults to 256 KB and 30 seconds,
  whichever comes first, and that interval is your worst-case visibility delay.
- **`bufio.Writer` is not safe for concurrent use.** A derived logger shares its
  writer, so one buffered writer reached from loggers in eight goroutines is a data
  race. One buffered writer per goroutine, or lock it yourself.

This package builds into its own fixed buffer and hands the bytes to whatever writer
you supply. It does not hold a write buffer for you: a logger owning a heap-allocated
buffer contradicts the fixed per-line buffer the package exists to provide.

## Benchmarks

```bash
go test -bench . -benchmem
```

Measured on an i9-12900KS, 24 logical processors, with Go 1.27.3.

```text
BenchmarkRenderPath-24      64.6 ns/op    0 B/op    0 allocs/op
BenchmarkSlogToDiscard-24  344.9 ns/op    0 B/op    0 allocs/op
```

`RenderPath` measures 64.6 ns today against 78.4 before `addPostStyle` stopped
re-emitting a colour the terminal already held: fewer escape bytes per component,
same colours, same order.

These are not the same shape of work and are not a comparison. `RenderPath`
builds a line — level label, message, one key/value pair — and never flushes, so
it measures the formatting only. `SlogToDiscard` is a complete `log/slog` call
written to `io.Discard`. For a fair head to head, this logger has to be benched
writing to a discarded writer as well, and it is not yet.

The `allocs/op` result is the claim that holds across machines. Time does not.

`CreateLogger` allocates, and so does `WithParams`. The per-line path does not.
