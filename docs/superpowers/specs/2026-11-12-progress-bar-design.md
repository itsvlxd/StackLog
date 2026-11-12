# A progress line that redraws in place and logs the result — design

Issue: #29, blocked by #28. Implemented 2026-11-12, after #28 landed, which is the order the cards
asked for. The card body is the shape this shipped in; what follows records where the build differed
from the sketch, and why.

## The contract, decided before the code

A progress bar in a logger is two problems wearing one coat:

1. **N frames, one record.** Sixty frames of a bar must not produce sixty log lines. The frames are
   terminal output; the record is a log line with a level, params and a timestamp like every other line
   the package writes. Everything else in this file follows from keeping those two apart.
2. **Control bytes in a log file are corruption.** A `\r` and an `ESC[K` make a file unreadable and add
   kilobytes a parser has to skip. So frames require the same proof colour requires — and #28 already
   computes it once per writer. This card does not implement detection a second time; it reads the
   stored verdict.

## What the card got right, and used unchanged

- A separate object rather than a method on the logger, because it holds state a logger must not carry:
  `done`, `total`, the last frame time, the last filled cell count, the first error.
- `Tick(n)` deciding whether a frame is due, and `Done` terminating the line and emitting the record.
- Frames as `\r` + bar + counts + `ESC[K`, with no newline written between them. There is no syscall for
  "delete a line"; the redraw is bytes the terminal emulator interprets, and `write(2)` is the only
  syscall in the trick.
- Fixed bar width, because terminal width is an `ioctl(TIOCGWINSZ)` — not available in the Go standard
  library in portable form, so it would cost `syscall` with build tags or `golang.org/x/term`. Both
  collide with the no-dependency rule. The card said "fixed 40-cell bar, documented as fixed" and that
  is what shipped.
- No alternate screen, no spinner, no multi-line TUI, no bar colour styling. One line.
- The concurrency story is one sentence: one progress object per writer, owned by one goroutine.

## What the card got wrong, corrected in the build

1. **"Two bytes per cell." The glyphs are three.** `█` is U+2588 and `░` is U+2591 — three bytes each in
   UTF-8, not two. The clamp that keeps a frame inside the fixed buffer is arithmetic on bytes, so the
   wrong constant would have let a bar overflow into the reserved tail and lose its `ESC[K`. A bar that
   loses its clearing sequence leaves ghosts of longer frames behind it. The clamp uses three and the
   measured overhead constant is derived from the worst case of each `Append` rather than from the
   sketch.
2. **Elapsed as a float seconds value printed garbage.** The generic float writer produces the shortest
   representation that round-trips, which is right for a benchmark and wrong for a bar: the first
   version drew `1.789e-06s` in the middle of a progress line. Fixed in two places — the frame now
   writes whole seconds and one decimal in fixed point from integer division (`8.2s`), and the record
   emits elapsed as `ms` with an integer count of milliseconds, the shape the package already uses for
   durations. A log reader should not have to parse exponent form to learn a job took two milliseconds.
3. **The card listed `ProgressOptions{Every: ...}` as the only option; `Width` had to be added.** Not for
   configurability — a 40-cell bar in a 40-column terminal wraps, and a wrapped progress bar is worse
   than none. Since the package cannot know the width, the caller is the only party who does. `Width` is
   that escape hatch, and it is clamped rather than trusted.
4. **`Abort` was not on the card and is in the build.** A bar whose run fails would otherwise leave an
   unterminated line on the reader's screen with nothing to say about why it stopped. It terminates the
   line and writes the same record at `ErrorLevel`. Six lines of code that complete the feature's own
   promise.
5. **The card assumed the frames and the record would both be capped by a time budget. The better rule
   is a change rule plus a time budget.** A frame is due when the filled cells or the whole-number
   percentage would look different, *and* not before `Every` has passed. The seconds text changes
   constantly and is deliberately not on its own worth a syscall — a bar that refreshes its elapsed
   figure every millisecond is a benchmark of the terminal, not a progress bar.

## Two defects the tests caught, both in the first version

- **The frame decision had no negative branch.** `due` was computed, and the throttle returned early when
  a frame was not due — but when nothing had changed, `due` was false and execution fell through into
  the draw anyway. `TestProgressFrameBudgetBoundsWrites` asked for two frames out of a thousand ticks with
  a one-hour budget and got ten. Fixed with an explicit `if !due { return }`.
- **The final frame hardcoded 100%.** `finish` drew `drawFrame(p.width, 100)`, so `Abort` on a bar at 30%
  drew a full bar at 100% while the record beside it said `pct=30`, and a bar with `total` of zero claimed
  100% of nothing. The final frame now uses the same computed fill and percentage as every other frame,
  and an indeterminate bar ends at 0% because 100% of nothing is a lie with a nicer face.

Both were found by tests written before the fixes, and both are pinned by the tests that found them.

## The gate, and why colour permission is not control permission

`resolveColor` takes the terminal verdict as an argument now instead of deriving it, and `StackLog`
carries `isTTY` beside `emitColor`. One `isTerminal` call per writer assignment answers both questions,
so `Progress` does not cost a second `Stat`.

The distinction is the load-bearing one: `ColorsOn` is permission for **colour**, and a logger that
forces colour into a file still does not draw frames into it. Anyone who reads `ColorsOn` as "let control
sequences through" has bought a corrupted log file.

`live` is therefore `isTTY && Format != FormatJSON`. JSON mode draws nothing, because control bytes in a
JSON stream are worse than in a text file.

## The non-terminal half

A writer that is not a terminal gets log lines: milestones at `DebugLevel` carrying `pct`, `done`,
`total` and `ms`, then the record at `InfoLevel`.

Milestones at `DebugLevel` is the noise control, and it comes from existing machinery rather than a new
option: `MinLevel` set to `Info` keeps the record and drops the milestones; set to `Warn` drops both. A
5,000,000-iteration loop cannot produce 5,000,000 writes, and it also cannot produce 5,000,000 log lines,
so the same change-and-budget rule caps both halves.

Frames are not filtered by `MinLevel` and are not wrapped by `FormatJSON` — the ground `Plain`
established. The record is a log line, so the filter applies to it. That asymmetry is deliberate and
asserted by `TestProgressFramesIgnoreMinLevelAndTheRecordDoesNot`: a logger filtered to warn still draws
its bar and still does not log a finished bar at info. Worth knowing before a long job runs quietly.

The record borrows the logger's own `params` slots for the duration of one `print` call and restores
them after. `Progress` carries eight slots for the four pairs, so the keys are literals and the slots
exist with the object. `TestProgressLeavesTheLoggersOwnParamsAlone` asserts the restore, because a
progress bar that left `pct=` on a logger would show up three lines later in someone else's output.

## What the buffering question decided

The card asked which wins: the redraw needing a `Flush` per frame, or the README's throughput advice.

Answer: neither, because the case does not arise. A `bufio.Writer` is not an `*os.File`, so
`isTerminal` reports "not a terminal" and the logger gets milestone lines instead of frames. Frames go to
a proven terminal, which in practice is an unbuffered standard output. The buffering advice and the
terminal gate are the same decision from #28 seen from two sides — wrapping a writer hands both to the
caller.

## Cost, measured in a clean sitting

Six counts of 3,000,000 each, nothing else running on the 24 logical processors:

| benchmark | median | range | B/op | allocs/op |
|---|---|---|---|---|
| `ProgressTickSkipped` — a tick that draws nothing | 17.7 ns | 16.9–18.6 | 0 | 0 |
| `ProgressFrame` — one frame composed and written | 142.4 ns | 139.4–152.0 | 0 | 0 |
| `ProgressDone` — the completion record, small counts | 208.8 ns | 202.4–210.7 | 256 | 1 |
| `ProgressDoneLargeCounts` — 120,000 items, 3 s elapsed | 250.7 ns | 245.5–257.4 | 280 | 4 |

`ProgressFrame` calls `drawFrame` directly, because a bar that is not due does not draw and a benchmark
that ticked its way to a frame would mostly measure the skipping. An earlier version of that benchmark
reported 4.0 ns for "a frame" for exactly that reason; it was measuring the skip path and naming it a
draw.

The one allocation in `ProgressDone` is the `Progress` object. Small integers need no boxing; 120,000 and
a duration in milliseconds do, which is why the second benchmark exists and shows four allocations and 24
extra bytes. A benchmark that only ever measures small numbers is measuring a case that does not ship.

Progress allocates once per operation. The per-line path is untouched: `ErrorToDiscardText` came back at
65.5 ns and `ErrorToDiscardJSONLong` at 98.9 ns in the same sitting as the colour-policy numbers, with
`isTTY` and `emitColor` two bools on the struct and no work on the line path.

## Tests

`progress_test.go` — no control bytes to a non-terminal writer · the record carries `pct`, `done`,
`total`, `ms` · frames drawn on a terminal writer with no newline inside a frame · the frame budget
bounding a thousand ticks to two writes · width clamped so a frame cannot overflow and must keep its
`ESC[K` · zero total never dividing and never claiming a percentage · frames ignoring `MinLevel` while the
record does not · no frames in JSON mode · params restored after the record · `Abort` terminating the line
and logging at error · the first write error reported rather than swallowed · the documented defaults.

The live half is tested by resolving the verdict to true in-package (`l.isTTY = true`), which is what a
test is allowed to do with an unexported field it owns. `isTerminal` itself is asserted in
`colorpolicy_test.go`, and the positive real-terminal case is demonstrated by the capture pipeline, where
vhs hands the program a pty and the recorded GIF shows the bar redrawing.

## Not in this build, on purpose

- Terminal width, and therefore an auto-sized bar. That needs the `ioctl`, and it is its own card.
- Bar colour. The frame is deliberately unstyled; the level label and params of the record carry the
  style, which is the boundary this package already draws.
- Spinners, multi-stage bars, alternate screen, percentage labels per stage.
- Concurrency beyond one owner. Two goroutines redrawing one line is a flicker war with no correct answer.
- `ProgressOptions.Every` below the frame budget floor. A caller who wants 200 frames a second can have
  them, and they will pay for the writes.

## Styleability pass, added after the first build was green

The request was specific: the progress records should carry params like any other line, and the bar
should be styleable as far as this package lets anything be styled. What landed:

- **`Style.Progress`, a `ProgressStyle` with `Filled`, `Empty` and `Numbers`**, each an ordinary
  `CompStyle`. It goes through `hasColor`, so `NoColors`, `ColorsOff` and a writer that is not a
  terminal suppress it exactly where they suppress colour anywhere else. It is the only component in the
  package with no preset colour, and that is a decision rather than an oversight: a bar in the
  terminal's own colours is the honest default for something that is not a log line.
- **Padding honoured on the sides and refused on the top and bottom.** `addPreStyle` writes Top padding
  when the buffer is at a line start, and `addPostStyle` records Bottom padding to flush at the end of
  the line. A frame is one line that gets redrawn, so both of those would move the bar off the line it
  lives on. `framePre` and `framePost` exist as a frame-specific pair for that reason rather than
  reusing the line pair and hoping.
- **`ProgressOptions.Params`**, and a record whose params are the logger's own pairs, then the options'
  pairs, then `pct`, `done`, `total` and `ms`. The order is stated because `pct` colliding with a
  caller's own key is a realistic accident.
- **`ProgressOptions.MilestoneLevel`** — the zero value is `DebugLevel`, which is genuinely the default
  and not a sentinel problem, unlike the record level. `Finish(level, message)` is the escape hatch;
  `Done` and `Abort` are `Finish` with the level chosen for you. Naming a level *is* how a line gets
  styled in this package, since `Style.Level[level]` and `Style.Params[level]` are keyed by level.

## Two defects that pass produced, both found by tests before the code was fixed

1. **The clamp did not know a styled bar is a longer bar.** `progressFrameOverhead` counted the carriage
   return, the counts, the percentage and the clearing sequence — 52 bytes — but not the style runs. Set
   `Style.Progress` with backgrounds and Left/Right padding, ask for 4,000 cells, and the clamp allowed
   322 cells: the frame composed 1334 bytes into a 1024-byte buffer. `TestProgressFrameTailSurvivesAStyledBar`
   measured it. The constant is now 150, with each part sized from the worst case its `Append` can
   produce (`3*(21+4)` for three style runs and their resets), and the clamp lands at 290 cells.
2. **The first version appended the progress numbers onto the logger's own params slice.** `derive()`
   copies the struct by value, and a logger derived with `WithPrefix` keeps the parent's `params` slice
   header — same backing array, same spare capacity. Appending four pairs onto that slice writes into
   memory the parent is still using. The fix builds the record line in the capacity `Progress` allocated
   for itself at construction, sized `2*len(logger params) + 2*len(options params) + 8`, so the record
   also allocates nothing. `TestProgressNumbersDoNotWriteIntoTheLoggersParamsArray` is the proof: a
   `WithPrefix` child runs a bar, and the base logger's own params are checked afterwards for both
   survival and contamination.

## The tail, and why a frame does not carry the truncation marker

`StackBuffer.writeTail` copies bytes past the content limit into the reserved tail, the way
`markTruncated` does. Content writes stop at `usable()`, so a frame cut short would otherwise lose its
`ESC[K` — and a bar that loses the clearing sequence leaves the remnants of a longer frame on the
reader's screen, which is worse than a bar that looks short. Both the marker and the clearing sequence
cannot fit in a 4-byte text tail, so the frame keeps the sequence and does not carry `...`. The width
clamp is what keeps the case from arriving at all, and it is tested rather than trusted.

This is also the answer to a distinction worth having straight: `NoColors` and `ColorsOff` gate *colour*,
not control sequences. A suppressed frame still writes `\x1b[K`. `TestProgressBarIsStyledAndSuppressionStillGatesIt`
strips that one legitimate sequence and then asserts no escape byte remains.

## Re-measured with progress in the package

Progress benchmarks, six counts of 3,000,000, clean sitting:

| benchmark | median | range | B/op | allocs/op |
|---|---|---|---|---|
| `ProgressTickSkipped` | 16.9 ns | 16.7–17.5 | 0 | 0 |
| `ProgressFrame` | 151.7 ns | 150.4–154.8 | 0 | 0 |
| `ProgressStyledFrame` | 182.1 ns | 180.4–185.6 | 0 | 0 |
| `ProgressDone` | 268.5 ns | 261.6–278.7 | 416 | 2 |
| `ProgressDoneLargeCounts` | 309.0 ns | 303.4–313.2 | 440 | 5 |

Colour on a bar costs 30.4 ns a frame — 182.1 against 151.7 — the same tax the package reports for
colour on a line. `ProgressDone` is two allocations: the object and its params slice. Large counts add
three boxings.

The log-line table was measured again in the same sitting, ten counts of 3,000,000, to prove the per-line
path was not touched by any of this: `RenderPath` 62.7 ns, `ErrorToDiscardText` 63.3, suppressed 39.3,
`ErrorToDiscardJSON` 43.6, `ErrorToDiscardTextLong` 124.9, `ErrorToDiscardJSONLong` 98.1, `slog` rows
342.9 / 536.3 / 591.4, `FilteredCall` 0.8. The end-to-end tables were re-run five times each: 95 ns and
56.6 B/line for a buffered default line, 156 ns and 115.9 B with `ColorsOn`, 119 ns for JSON, against
`slog` at 568 and 627 — 6.0× on the default line, 3.6× with colour, 5.3× on JSON.

Every README table was replaced with this sitting's numbers rather than patched with a few new ones, so
the document quotes one measurement session. The movement against the previous sitting is a few percent
on rows that were already inside their own printed ranges, which is what the ranges are for.

## Still not in this build, on purpose

Terminal width and therefore an auto-sized bar. Spinners, multi-stage bars, alternate screen. Colour on
the *message text* of a record — messages are unstyled everywhere in this package, and #30 closed that
question as not planned rather than leaving it half-open here.

# Second pass: the bar became the tail of a log line

The first build is in the sections above and it was shipped nowhere — it was two commits from being
pushed when the design was reopened. The owner's request was explicit: `WithProgress`, a bar appended at
the end of the message, and bar lines that carry a level, a message, a timestamp and params like any
other line. That is a better shape than the one I built, and the reasons are worth writing down because
they are the reasons the first shape was wrong.

## What changed, and why the new shape is better

The first design had two kinds of output: frames (terminal bytes, no newline) and records (log lines).
The second has one kind: **a log line with a bar at the end of it**, which is what a logging package
should have done from the start.

- `WithProgress` belongs to the family the package already has — `WithParams`, `WithPrefix`, `WithWriter` —
  and derives, so the bar writes through its own logger on its own buffer. `NewProgress(logger, ...)` was
  the same operation wearing a different name, and it is gone. Nothing was released with it, so removing
  it costs no one anything.
- Level, message, prefix, timestamp and params are not features the bar has to implement any more. They
  are what a line already carries, because a bar line goes through the same builder.
- The tail is the only new part. `lineTail` is a function hook rather than a field on the logger, so the
  ordinary path pays one nil check for something it does not use.

`print()` is now a thin delegation to `printLine(level, message, tail, inPlace, terminate)`, which is the
whole line builder: the filter check, the JSON branch, timestamp, level label, prefix, message, params,
the optional tail, then either the marker and terminator or the carriage return and clearing sequence.
One builder, two shapes of line, and they cannot drift apart.

## The four defects the second pass produced, all caught by tests

1. **`live` was cached at construction.** `WithProgress` derives a logger, and the bar read the derived
   logger's terminal verdict at that moment. Set the verdict afterwards — which is exactly what a test
   does, and which is what a caller doing `logger.WithWriter(f)` before `WithProgress` would hit in
   production — and the bar had a stale answer. Replaced with `redraws()`, a question asked of the logger
   each time it is drawn on. A bar should not have an opinion about its writer that outlives the writer.
2. **The budget and the "is a line open" flag were the same variable.** `drawn` was used both to decide
   whether a line was due and to decide whether the budget applied. In file mode nothing is ever "drawn"
   in the terminal sense, so the budget check never fired and a thousand ticks with a one-hour budget
   produced a thousand log lines. `TestProgressFileMilestonesAreBudgetedToo` measured 1,001 lines where
   two were promised. Two separate things now: `last` zero is the first line's exemption from the budget,
   and `drawn` means there is an open, unterminated bar line on the reader's screen.
3. **`Finish` claimed a bar line existed.** It set `drawn = true` before writing, so a run whose levels
   were all filtered out still wrote a lone newline — a line nobody had drawn being terminated. The flag
   is now set by `write()` from what actually happened: `drawn = !terminate`, and only for a bar that
   redraws. `TestProgressBarLinesObeyMinLevel` asserts a filtered run writes nothing at all.
4. **Vertical padding leaked newlines into bar lines.** A level style's `Top` padding is written when the
   buffer believes it is at a line start, and a redraw is at a line start in the terminal's eyes. The
   first padding test only styled `Style.Progress`; widening it to `Style.Level` made the failure appear
   immediately — three blank lines before the bar. `printLine` now clears `lineStart` for an in-place
   line, and the test asserts no newline inside any redraw.

One more, in the test rather than the code: an assertion that a run contains one `INFO` was wrong on
purpose-built grounds — the bar's own lines carry the level too. The claim being tested is "exactly one
newline per run, and the last write clears then terminates the line it replaced", which is what the
replacement behaviour actually means, and the test now says that instead.

## Where the progress numbers live, and what that bought

A text line states its numbers once, beside the bar: `600/1500 40% 8.2s`. A JSON line has no bar and no
tail, so the same facts arrive as fields. The first build put them in both places in text mode — `pct=`
in params and `40%` in the tail — which duplicated every number on every bar line, and cost allocations:
a bar line boxed four numbers into an `any` slice and measured **19 B and 2 allocs per line**. Moving the
numbers to the tail in text mode took them to **0 B and 0 allocs**, which is the difference between a bar
that is a log line and a bar that quietly allocates.

JSON still boxes them, because a JSON field is a value in a map and that is the format's own cost rather
than progress's.

## The bar has a default look now, and the earlier decision was reversed

The first pass deliberately gave `Style.Progress` no preset colour. That was wrong on the merits — a bar
with no colour in it reads as a formatting accident — and it was also inconsistent with every other
component in the package, all of which fall back to the preset. The owner asked for a green bar on a grey
track, and the honest answer is that a logger should ship with a bar that looks intentional.

Each preset now carries one, and each is its own decision rather than a copy:

| preset | filled | remainder | numbers |
|---|---|---|---|
| `PresetDefault` | `70` green | `242` grey | white |
| `PresetMuted` | `108` | `240` | `250` |
| `PresetBadge` | `70` on a `236` background slab | `240` | white |
| `PresetHighContrast` | `82` | `244` | `255` |

`loadPreset` fills the whole `ProgressStyle` when it is left as the zero value, so a caller who sets any
part of it replaces the set — the same rule a single style component follows everywhere else in this
README. `TestEveryPresetCarriesABarPalette` and `TestProgressStyleOverrideStandsAgainstThePreset` hold
both halves.

## The hot path, measured rather than asserted

The line builder was restructured, and restructuring the hot path is the sort of change that has to be
shown rather than promised. Ten counts of 3,000,000, clean sitting, nothing else running:

| benchmark | this pass | previous sitting | inside the old range |
|---|---|---|---|
| `RenderPath` | 61.6 ns | 62.7 ns | yes |
| `ErrorToDiscardText` | 65.5 ns | 63.3 ns | yes |
| `ErrorToDiscardTextNoColors` | 41.5 ns | 39.3 ns | yes |
| `ErrorToDiscardJSONLong` | 98.4 ns | 98.1 ns | yes |
| `ErrorToDiscardTextLong` | 123.7 ns | 124.9 ns | yes |
| `FilteredCall` | 1.0 ns | 0.8 ns | yes |

The `tail != nil` check and the `inPlace`/`terminate` branch are not free, and they are also not a
regression: every row moved by less than its own printed spread. The end-to-end tables were re-run five
times each after the change and moved the same way — 97 ns buffered default against 95 before, 155 against
156 — so the README was rewritten with this sitting's numbers rather than having two numbers patched in.

The progress numbers themselves: a skipped tick **12.9 ns**, a bar line **165.5 ns** uncoloured and
**203.1 ns** with the preset's bar colours — colour on a bar is about 38 ns a line, against the 24 ns a
coloured log line costs — and ending a run **596.5 ns with 3,136 bytes and four allocations**, which is
the derived logger, its own 1,024-byte buffer, the `Progress` object and its params slice, paid once per
operation. A bar over 120,000 items is 625.1 ns and the same allocations, because the numbers are no
longer boxed in text mode at all.

## What the tests say now

Twenty-six tests in `progress_test.go`, and the ones that encode decisions rather than mechanics:

- the bar is green on grey out of the box, and every preset carries a palette;
- suppression gates the bar's colours and not its clearing sequence;
- a bar line carries the level, the message, the logger's params and the caller's params in that order;
- a file gets the numbers and no bar characters, and no number appears twice in either format;
- a redraw opens with a carriage return, clears to end of line, and contains no newline;
- `Done` replaces the line the bar has been redrawing — three writes, one newline in the whole run;
- `MinLevel` filters bar lines, and a filtered *result* line still terminates a bar that was drawn;
- JSON mode writes objects with fields and no bar anywhere;
- the budget bounds a thousand ticks to two writes on a terminal and two lines in a file;
- `WithProgress` derives, `Logger()` hands the derived logger back, and two bars never share a buffer;
- the bar's numbers cannot be written into a logger whose array it shares.

# Third pass: a bar in a file is a row, and a bar in JSON is two numbers

The second pass drew the bar on a terminal and refused to draw it anywhere else, on the grounds that 120
bytes of box-drawing characters per log line is decoration a file did not ask for. That reasoning was
half right and half cowardly, and the owner overruled the half that mattered: **a file cannot redraw, so a
progress bar in a file is a row per update, and every row carries its own bar.** The file becomes the
history of the run, and a reader scrolling it watches the bar lengthen. Refusing to draw one left a file
with a series of numbers and no shape, which is not what a progress bar is for.

```text
INFO rebuilding index file=index ███░░░░░░░░░░░░░░░░░ 2/12 16% 0.0s
INFO rebuilding index file=index ██████░░░░░░░░░░░░░░ 4/12 33% 0.1s
INFO index rebuilt file=index ████████████████████ 12/12 100% 0.4s
```

About 60 bytes a row at the demo's 20 cells, 120 at the default 40. Nothing control-encoded: no carriage
return, no erase, nothing a log viewer has to strip. The budget is what keeps the cost finite, and the
documented number is what keeps the decision honest instead of discovered.

JSON went the other way, and for the opposite reason. A JSON row is read by a machine, and a bar is a
rendering rather than data, so a JSON row carries the two numbers a rendering needs:

```json
{"level":"debug","msg":"rebuilding index","params":{"pct":16,"done":2,"total":12,"ms":80,"filled":6,"width":20}}
```

`filled` and `width` join `pct`, `done`, `total` and `ms`. One object per update, each a complete row.
`Entry.Params` carries them, so the shape stays documented API.

### What this pass cost, measured

Ten counts of 3,000,000, nothing else running:

| benchmark | median | range | B/op | allocs/op |
|---|---|---|---|---|
| `ProgressTickSkipped` | 13.0 ns | 12.9–14.2 | 0 | 0 |
| `ProgressRedrawNoColors` | 166.9 ns | 165.9–176.5 | 0 | 0 |
| `ProgressRedraw` | 204.6 ns | 203.6–224.4 | 0 | 0 |
| `ProgressDone` | 707.9 ns | 691.3–725.4 | 3,264 | 4 |
| `ProgressDoneLargeCounts` | 729.0 ns | 721.4–740.0 | 3,264 | 4 |

`ProgressDone` moved from 596.5 ns and 3,136 bytes to 707.9 ns and 3,264 bytes because the benchmark ends
a run in *file* mode, and a file mode that draws a completed bar writes 120 more bytes of them. That is
the change doing exactly what it was asked to do, visible in the measurement rather than waved at. The
allocation count is unchanged at four, which is the point of putting the numbers in the tail rather than
in params: drawing more costs bytes, not garbage.

The log path did not move because of any of this — `RenderPath` 63.1 ns (59.3–66.4 across counts, 61.6 in
the previous sitting), `ErrorToDiscardText` 66.2 against 65.5, suppressed 40.0 against 41.5, realistic
text 123.9 against 123.7, realistic JSON 99.4 against 98.4 — every difference smaller than the range the
same table prints.

### A formatting defect the new file rows exposed

With the bar drawn in a file row, the counts grew straight out of the last cell — `██░░4/12 16% 0.0s` —
because the numbers run never needed a leading space when nothing was drawn before it. The terminal shape
hid it: the space had been coming from somewhere else. The tail now puts one space between the bar and its
numbers for every line, and a test asserts the space rather than admiring the output.

### The decisions this pass reversed, listed so the record is not a palimpsest

- Superseded, in the section above: *"In a file it is not drawn at all: 120 bytes of box-drawing
  characters per line is decoration nobody asked for."* Overruled. A bar belongs in a file's rows.
- Still true from the first pass: the bar goes only to a *redrawn* line on a terminal, control sequences
  stay out of files, the numbers are stated once per line, `ESC[K` belongs to the redraw and is written
  into the reserved tail, and the width clamp counts style runs because a coloured bar is a longer bar.
- Still open, untouched by this pass: terminal width (#31), and no spinner, no alternate screen, no
  colour on message text.
