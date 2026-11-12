# Colour policy for writers that are not terminals — design

Issue: #28. Board: Ready, P1, size M, target 2026-11-18. Implemented 2026-11-12; the commit split
and the release are the parts still pending.

## Intent

Right now a log file contains ANSI escape bytes unless the caller remembers `Style.NoColors`. The
README used to state this as a feature: "There is no terminal detection." The annoyance is real, it
is the package's default, and defaults are the part callers never override.

The change: colour becomes conditional on the writer actually being a terminal, with an explicit
override for callers who want colour anyway.

## Constraints, established by a spike rather than assumed

Run on Linux, i9-12900KS, Go 1.27.3. This table is the reason the design is shaped the way it is.

| writer | `Stat().Mode()` | char device? | naive rule | the rule that shipped |
|---|---|---|---|---|
| `os.Stdout` in a pipe | `prw-------` | no | off ✓ | off ✓ |
| `os.Stdout` on a pty (`script -qec`) | `Dcrw--w----` | yes | on ✓ | on ✓ |
| `os.Stdout` redirected to a file | `-rw-r--r--` | no | off ✓ | off ✓ |
| a regular log file via `WithWriter` | `-rw-------` | no | off ✓ | off ✓ |
| `bufio.Writer` around `os.Stdout` | not an `*os.File` | — | off ✓ but wrong, see below | off |
| `bytes.Buffer`, `io.Discard` | not an `*os.File` | — | off ✓ | off |
| **`/dev/null`** | `Dcrw-rw-rw-` | **yes** | **on ✗** | **off ✓** |

Two findings, both load-bearing:

1. **`ModeCharDevice` alone is not "is a terminal".** `/dev/null` is a character device. A rule built
   on that bit alone paints escape sequences into the bit bucket and reports success. The fix costs
   nothing: the writer must also *be* standard output or standard error, checked with
   `f.Fd() == 1 || f.Fd() == 2`. `os.File.Filer()` does not exist in this toolchain; `os.File.Fd()`
   does, and it works on every platform this package compiles on.
2. **`bufio.Writer` is invisible to detection** — it is not an `*os.File`, and there is no portable
   way to see through it. `bufio.Writer` is the writer the README recommends for throughput, so the
   honest statement is: *the package decides colour for writers it can identify; wrapping a writer
   takes the decision away from the package and hands it to the caller*, which is what `ColorsOn` and
   `ColorsOff` are for. Stated as a limit, not left as a bug to fix later.

## The shape

`Config.Colors string` with three exported values, matching how `Style.Preset` and `Config.Format`
already take a name — a string, a constant, and an unrecognised value that resolves to the default
rather than failing:

```go
const (
	ColorsAuto = "auto" // colour only where the writer is a proven terminal
	ColorsOn   = "on"   // colour always
	ColorsOff  = "off"  // never
)
```

The zero value is `""`, which resolves to `ColorsAuto`. **That is the default flip**: `Config{}`
writing to a pipe or a file now writes no escapes. It is the point of the card, and the reason it is
P1 while there are no dependents.

### Resolution rules, in order

1. `Style.NoColors == true` → no colour. It stays the absolute kill switch it has always been: it
   outranks `ColorsOn`, and it can never turn colour on.
2. `ColorsOff` → no colour, whatever the writer is.
3. `ColorsOn` → colour, whatever the writer is — a file, a pipe, `/dev/null`.
4. Empty, `ColorsAuto` or an unrecognised name → colour only when the writer passes both halves of
   the test: `*os.File` **and** `Fd()` is 1 or 2 **and** `Mode()&os.ModeCharDevice != 0`.

`hasColor` became the single gate that reads the resolved answer instead of recomputing a policy per
line:

```go
func (sl *StackLog) hasColor(style CompStyle) bool {
	return sl.emitColor && (style.fgSet || style.bgSet)
}
```

`emitColor` is resolved in `CreateLogger` (writer is `os.Stdout`) and in `WithWriter` (whatever was
handed over). `derive()` copies it, which is correct because a derived logger inherits its parent's
writer — same writer, same verdict. Answering the question properly costs a `Stat` syscall, and a
line is not the place to pay for it.

### JSON mode

`applyJSONFormat` sets `Config.Colors = ColorsOff` **and** keeps `Style.NoColors = true`. The README
and `TestJSONModeSuppressesColoursAndPadding` both promise a JSON logger reads back with `NoColors`
set, so the alias stays rather than being quietly replaced by the new vocabulary.

## What this broke, measured rather than guessed

- **Six test assertions** in `json_test.go`, `padding_test.go`, `presets_test.go`,
  `stacklog_test.go` and `writer_test.go` expected escape bytes in output captured into a
  `bytes.Buffer`. Those writers are not terminals now, so each one states `Colors: ColorsOn`
  explicitly. The suite became an honest catalogue of "this test asserts coloured output" instead of
  leaning on a default.
- **Four benchmarks** in `bench_test.go` exist to price a coloured line and all four write to
  `io.Discard`. Under the new default they became measurements of the *uncoloured* line —
  `ErrorToDiscardText` came back at 40.8 ns, which is the number the `NoColors` benchmark has always
  reported. Two benchmarks meant to measure different things were measuring the same one, invisibly.
  They now ask for `ColorsOn`, with a comment saying why.
- The README's "There is no terminal detection" note is gone, the `NoColors` examples gained the new
  sentence beside them, and the throughput and `slog` tables were re-measured.

## One sharp edge chosen deliberately, and pinned

`Config.Colors` is resolved when the writer is known. A caller who mutates `logger.Config.Colors`
afterwards — the same shape of call the README endorses for style components,
`logger.Config.Style.Level[ErrorLevel] = ...` — does not change the answer. Making it respond would
mean a `Stat` syscall on the per-line path, which is the opposite of what this package is for. The
answer is documented and asserted by `TestColorsMutatedAfterTheWriterIsKnownHasNoEffect`, so the
documented rule is the actual rule instead of a surprise.

## Enforcement, as written

Twelve tests in `colorpolicy_test.go`:

`TestColorsAutoLeavesABufferWriterUncoloured` · `TestColorsAutoLeavesARegularFileUncoloured` ·
`TestDevNullIsNotATerminal` · `TestColorsOnColoursANonTerminalWriter` ·
`TestColorsOffSuppressesColourTheStyleWouldOtherwiseWrite` · `TestNoColorsOutranksColorsOn` ·
`TestUnknownColorsNameBehavesAsAuto` · `TestColorsMutatedAfterTheWriterIsKnownHasNoEffect` ·
`TestWithWriterResolvesTheDecisionAgain` · `TestDerivedLoggerInheritsTheVerdictOfItsParent` ·
`TestJSONLoggerStatesItsColourDecision` · `TestIsTerminalOnWriterKinds`

The old promises are untouched and still pass: `NoColors` writes no escape even where a component
carries padding, and every `NoColors` example behaves as it did.

## What the measurements came out at

Clean sitting — ten counts of 3,000,000 for the builder benchmarks, five runs of 500,000 identical
lines to a real file for the end-to-end numbers, nothing else running.

- Builder: coloured text 65.5 ns, escapes suppressed 40.7 ns. Escapes are 38% of building a coloured
  line, and `ColorsAuto` to a non-terminal writer is the 40.7 ns path reached by policy now rather
  than by a flag.
- End to end, 256 KB buffered file: `ColorsAuto` 99 ns/line at 56.6 B/line; `ColorsOn` 166 ns/line at
  115.9 B. Against `slog`'s text handler at 578 ns: **5.8×** by default, **3.5×** when colour is
  asked for explicitly.
- Bytes: a coloured realistic line spends 59.3 of its 115.9 bytes on escape sequences. The default
  line into a file is now just over half of what `slog` writes.

## What was deliberately not unit-tested, and said out loud

The positive case — a real terminal, colour arrives — was verified by hand under `script(1)`, which
allocates a pty, and again through the capture pipeline: the recorded GIFs still contain the level
colours (101 to 2,131 saturated red pixels across the captures that show error-level lines, zero in
the ones that do not). It is not a permanent test. A test whose verdict depends on whether the
machine running it has a terminal attached fails in CI for reasons unrelated to the code. The
permanent tests assert the negative half, which is the half that used to break people.

## Two measurement lessons from this sitting

1. An earlier sweep of the same code, taken while the capture pipeline was writing GIFs in the
   background on the same 24 logical processors, came out 1.5× slower on every row of the benchmark
   table, `slog`'s rows included. Recorded so nobody chases it as a regression.
2. `go run ./example file` writing to a pipe is now itself a demonstration of the policy, and the
   capture width is the part that had to be measured three times to be described honestly. The
   committed capture was 1,378×158 with one quoted line. The first version of the new demo quoted
   both lines as a single blob and recorded 1,858 pixels wide - wide enough that GitHub scales it
   down on a package page. The shipped form quotes line by line and records 1,172×188. The middle
   number never existed in a commit, which is why the README and the asset table quote 1,172.

## Non-goals

- Terminal width, `ioctl(TIOCGWINSZ)`, or anything needing `syscall` and build tags.
- Colour passthrough policy for raw escapes a caller puts in a message — that is #30.
- Detecting through `bufio.Writer` or any custom writer. Documented as out of reach.
- Removing `NoColors`. It stays as a kill switch that can never turn colour on.

## Consequence for #29

The terminal question that card was blocked on now has an answer inside the package. What #29 still
lacks is terminal width, which this card deliberately did not buy.
