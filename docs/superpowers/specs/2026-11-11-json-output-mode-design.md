# JSON output mode — design

Issue: #5. Size on the board: L, P2, due 2026-11-20.

## Intent

`stacklog` writes one coloured line per call. This adds a mode that writes one JSON
object per call, newline terminated (JSON Lines), so the same logger can feed a parser
without the caller re-implementing formatting outside the package.

The contract that does not move: one line per call, one `Write` per line, a fixed buffer
that never grows, and no allocation on the per-line path. Pretty-printed JSON is not this
feature — it breaks one-write-per-line.

## Decisions already taken by Vlad

1. **Truncation comes off the main message.** When a line will not fit, the `msg` field is
   cut, ends with `...`, and the object carries `"truncated":true`. The structure is never
   left unparseable.
2. **The line shape is exported.** A consumer can decode a line into a named Go type from
   the package, so the field names are part of the API and not a coincidence of the writer.

## Shape

Emitted, in this order, one object per call:

```json
{"time":"2026-11-11T13:04:22Z","level":"error","prefix":"http","msg":"upstream 502","params":{"retry":3,"timeout":250000000},"truncated":true}
```

Exported decoder type:

```go
// Entry is the JSON form of one log line. A caller can decode any line this package
// writes in JSON mode into it.
type Entry struct {
	Time      string         `json:"time"`
	Level     string         `json:"level"`
	Prefix    string         `json:"prefix,omitempty"`
	Msg       string         `json:"msg"`
	Params    map[string]any `json:"params,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}
```

`truncated` is emitted only when a line was cut. A reader can therefore tell "this is the
whole message" from "this is what fit" without a convention, and a line that fits is not
carrying a field that means nothing.

`prefix` is omitted when the logger has no prefix. `params` is omitted when there are none.

**Why `params` is nested and not flattened at the top level.** The exported struct is the
reason. Flattened keys cannot be decoded into a fixed struct at all — every caller would
need their own type per call site, and a caller-supplied key called `level` would collide
with the level field. Nesting makes `Entry` a complete description of every line the
package can write, and it removes the collision class outright. `jq '.params.retry'` is one
more character than `.retry`, which is not a cost worth taking that completeness off the
table.

## Field name and format calls (mine, correctable)

| Field | Value | Why |
|---|---|---|
| `time` | RFC3339 when `Config.TimeFormat` is empty | JSON mode exists to be parsed. `time.DateTime` ("2006-01-02 15:04:05") carries no offset, so a parser has to guess. An explicit `TimeFormat` is honoured exactly as in text mode. |
| `level` | `debug` `info` `warn` `error` `fatal` | Canonical names, not labels. |
| `prefix` | the logger prefix, without the trailing `:` | The colon is text-mode punctuation. |
| `msg` | the message string | |
| `params` | object of key/value pairs | |

## LevelPrefix is ignored in JSON mode

This is what the card proposed and it stands. `Style.LevelPrefix` is a display label:
`DEBU`, `DBG`, `!!`. In JSON the level is a field with a known set of values, so a label
would either duplicate the level or replace it with something a parser cannot match on. A
JSON mode therefore uses canonical names and cannot use the preset label sets.

## Colours and padding are off, not switched

Colours emit no escapes in JSON mode and `NoColors` has no effect either way — escape codes
inside a JSON string are noise. `Style` component colours are inert.

Padding is inert entirely. `Top` and `Bottom` produce extra lines, which between JSON
objects are stray newlines, and `Left`/`Right` write spaces inside a coloured block that no
longer exists. A caller who sets padding and turns on JSON mode gets neither, and the doc
says so rather than leaving it to be discovered.

`Style` stays accepted in JSON mode so a `Config` does not have to be rebuilt to switch
formats.

## Value rules

| Go type | JSON |
|---|---|
| every int and uint width | number |
| `float32`, `float64` | number |
| `bool` | `true` / `false` |
| `string` | string, quoted and escaped |
| `error` | string, from `Error()` |
| `time.Duration` | number, nanoseconds — the same choice text mode makes |
| `nil` interface | `null` |
| anything else (`time.Time`, maps, slices, structs) | the string `"<unsupported>"` |

`<unsupported>` stays the same token in both formats on purpose: one vocabulary a caller can
grep for, and a JSON line stays parseable when a value cannot be written. Replacing it with
`null` would make "the package could not render this" indistinguishable from "the value is
nil", which is a worse failure to debug.

Keys are escaped by the same rule as values. A key is written as given; it is not lowercased
or renamed.

Duplicate keys are possible — `WithParams("user", "a", "user", "b")` writes both — because
the buffer appends and does not remember what it wrote. Duplicate keys are legal JSON, and
every decoder reached by a Go caller takes the last one. This is documented rather than
fixed; fixing it means a map, and a map is a heap allocation.

## Truncation

Text mode reserves 5 bytes at the tail of the 1024-byte buffer for `...` plus the terminator,
and cuts whatever is being written. That cannot work in JSON mode: cutting mid-string leaves
an unterminated string, and cutting mid-object leaves an unterminated object.

JSON mode reserves the tail it actually needs instead. The closing structure is written
after the content area, so the reserve is its worst case:

```
} ,"truncated":true } \n     20 bytes
```

Everything before that reserve is content area: **1004 bytes in JSON mode against 1020 in text
mode.** That is the price of a line that is always parseable, it is a real number, and it
belongs in the docs and in a test rather than being left for someone to discover.

The mechanism reaches shared buffer code, and that is deliberate:

- `StackBuffer.reserve` becomes a field instead of `usable()` reading the constant
  `truncateReserve`. `CreateBuffer` sets it to the text value, so text mode is untouched.
- `markTruncated` is not called on the JSON path. The `...` in JSON lives inside the `msg`
  string and the closing structure is written by the logger. Both markers firing would put
  `...` outside the quotes.
- Pair rollback is `BufLen` restored to a saved position after `truncated` is observed.
  No measuring pass, no second scan, no allocation. Two rules then apply, and they are the whole
mechanism:

1. **The message is the field that gives way.** It is written into whatever room is left and
   cut inside the string, with `...` ending the value and the quote closed after it.
2. **Every other field is written whole or not at all.** The buffer position is saved before
   a field or a pair is attempted. If the attempt reports truncation, the position rolls back
   to the saved value and the field is dropped — never half a pair, never a dangling `"key":`.
   No lookahead, no measuring pass, no second scan of the string.

`truncated` is set by either rule. A pair that was dropped is absent from the object; the
reader knows something was lost because `truncated` is present, and knows it was not the
message only because the message is not cut. There is no field listing which pairs were
dropped, and that is a gap this design accepts rather than solves — decided in the open: a
param that will not fit is not added at all, and `truncated` is present.

If the fixed skeleton itself exceeds the reserve — a very long prefix, or a `TimeFormat`
long enough to matter — the line cannot be valid. That case must fail loudly rather than
write broken JSON into the stream; the exact behaviour (an error from the log call, or the
call dropped and reported on the writer the package already uses for write failures) is
listed below as open.

## What JSON mode does not change

`Config.MinLevel` filters before anything is built, in both formats. `WithParams`,
`WithPrefix` and `WithWriter` behave the same. `Fatal` still flushes a flushable writer and
exits 1. Derived loggers copy `Config`, so a format set at creation reaches the derived
loggers.

## Switch

`Config.Format string`, with exported constants:

```go
const (
	FormatText = "text"
	FormatJSON = "json"
)
```

The zero value writes text, so every existing `Config{}` is unchanged. An unknown name
resolves to `FormatText`, which is the same rule `Style.Preset` already uses for an unknown
preset name. A string name matches how this package already takes a style decision.

## Not in scope

- Structured field marshalling. There is no `WithFields(any)`, no reflection, no field
  naming. The card's "no reflection, no field marshalling" line stays true.
- `time.Time` as a value type. It is currently `<unsupported>` in text mode too, and making
  it a real value is its own card.
- A JSON writer that grows. Fixed buffer stays.
- Per-field styles in JSON mode. Inert.
- Multi-line pretty output.

## Enforcement

Each of these is a test, not a note:

1. Every line emitted in JSON mode passes `json.Valid`, including a truncated one.
2. Every line decodes into `Entry`, and the decoded fields match what was logged.
3. Escaping: `"` `\` and every control byte below 0x20 in a message, a key and a value.
4. Invalid UTF-8 in a value produces a parseable line, not a corrupted one.
5. A long message is cut with `...` inside the string and carries `truncated:true`.
6. A pair that does not fit is absent whole — no partial `"key":` in the output.
7. No escape bytes and no padding bytes appear anywhere in JSON output.
8. Level names are canonical regardless of preset labels, and `LevelPrefix` is unused.
9. `nil` is `null`; `<unsupported>` stays `<unsupported>`; duration is nanoseconds.
10. An unknown `Format` writes text.
11. A text-mode test set still passes unchanged, and a `Config{}` with no `Format` writes the
    coloured line it writes today.

## Open, resolved

1. **The skeleton-does-not-fit case.** Decided: return `ErrLineTooLarge` and write nothing.
   A line the caller cannot parse is not a line they can act on, and `stacklog` already
   returns write errors, so this is an error the caller is expecting to handle.
2. **Showcase and captures.** JSON is demonstrated in the README images, which means
   `example/main.go` gains a `json` section, `capture.sh` gains a map entry, and
   `assets/json-output.gif` has to exist or the drift check fails the run.
