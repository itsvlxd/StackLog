#!/usr/bin/env bash
# Regenerate the README captures from the showcase program.
#
#   ./example/capture.sh           vhs -> animated GIF. What assets/ holds today.
#   ./example/capture.sh --png     freeze -> PNG, static, no conversion
#
# Two rules keep this honest. The section names come from the program itself
# (`go run ./example -list`), so a feature that no longer exists cannot be captured.
# And the asset names are pinned in the map below, because the README links them and a
# renaming one is a broken image on a package page.
#
# Why vhs and not freeze for the images the README uses: freeze renders a padded
# colour block by blending the block colour into the glyph edges, so a badge with a
# background looks like the background is bleeding into the text. vhs draws the
# terminal as a terminal — block behind glyphs, glyphs on top — which is what the
# package actually emits.
#
# vhs sizes its canvas in character cells with Set Columns and Set Rows and derives the
# pixel size from the font settings. That is what these captures use. Guessing a pixel
# width from the longest line is what the freeze path does, and it is why the freeze
# path is no longer the default.
#
# The GIF palette is still a real ceiling: 256 colours, indexed 8-bit. GIF is animated
# and freeze has no GIF output at all, so this is the trade being made, knowingly.
#
# Canvas width is left alone, and the alternative was measured rather than assumed. Setting
# FontSize to 15 fits the widest section at 1006 pixels instead of 1378 - inside the README
# column rather than past it - and it costs the one colour these captures exist to show. At
# FontSize 22 the nearest palette entry to #D75F5F is #DA6053, three units off in red. At 15
# it is #C15C52, twenty-two units off. A smaller font spends the same 256 entries on more
# glyph blending, so the exact value drops out of the palette entirely. Capping Columns is
# worse still: it wraps a long line onto a second row, which misrepresents what the logger
# actually wrote. GitHub scales the wide captures down; it cannot un-round a colour.
set -euo pipefail

cd "$(dirname "$0")/.."

mode=vhs

case "${1:-}" in
  --png) mode=png ;;
  -h|--help)
    sed -n '2,/^set -euo/p' "$0"
    exit 0
    ;;
esac

sections=$(go run ./example -list)

if [[ $mode == vhs ]] && ! command -v vhs >/dev/null 2>&1; then
  echo "vhs is not installed." >&2
  exit 1
fi

if [[ $mode == png ]] && ! command -v freeze >/dev/null 2>&1; then
  cat >&2 <<'EOF'
freeze is not installed.

  Arch (AUR):   yay -S freeze
  Homebrew:     brew install charmbracelet/tap/freeze
EOF
  exit 1
fi

# section:asset-name. The names the README links; the extension comes from the mode.
mapfile -t mapping <<'EOF'
simple:info-service-started
levels:level-logging
params:upstream-502-error
values:value-types
filter:level-filter
prefix:http-prefix-error
styles:upstream-502-error-params
presets:presets
padding:connection-reset-error
timestamp:info-request-timestamp
file:wrote-line-readme-log
write-errors:handling-write-errors
json:json-output
fatal:shutting-down-fatal
EOF

# Drift in either direction is an error here, not a surprise discovered later.
for entry in "${mapping[@]}"; do
  section=${entry%%:*}

  if ! grep -qx "$section" <<<"$sections"; then
    echo "capture map lists section '$section' but example/main.go does not define it" >&2
    exit 1
  fi
done

if [[ $(wc -l <<<"$sections") -ne ${#mapping[@]} ]]; then
  echo "the map covers ${#mapping[@]} sections; example/main.go lists $(wc -l <<<"$sections"). Add or remove a line." >&2
  exit 1
fi

if [[ $mode == png ]]; then
  ext=png
else
  ext=gif
fi

mkdir -p assets

png_dir=$(mktemp -d)
tapes=()
trap 'rm -rf "$png_dir"; rm -f "${tapes[@]}"' EXIT

root=$(pwd -P)

for entry in "${mapping[@]}"; do
  section=${entry%%:*}
  name=${entry#*:}

  # Fatal exits the process on purpose. A non-zero status from that section is the
  # feature working, not the pipeline failing, so it must not stop the loop.
  output=$({ go run ./example "$section" 2>&1 || true; })

  lines=$(wc -l <<<"$output")
  longest=$(sed 's/\x1b\[[0-9;]*m//g' <<<"$output" | awk '{ if (length > m) m = length } END { print m+0 }')

  if [[ $mode == png ]]; then
    # sh -c is required: freeze --execute runs its argument as a command rather than
    # through a shell, so a bare `go run ... || true` reaches exec as one word and
    # fails with `exec: "{": executable file not found in $PATH`.
    freeze -o "assets/$name.png" -W "$(((longest + 6) * 18))" -p 16 -m 24 \
      --execute "sh -c 'go run ./example $section || true'"
  else
    # Columns and Rows are character cells. The +8 leaves room for the shell prompt and
    # the command that was typed, and it keeps a long line from wrapping onto a second
    # row. A one-line section still gets a one-line canvas.
    columns=$((longest + 8))
    rows=$((lines + 3))

    tape="$root/.capture-$section.tape"
    tapes+=("$tape")

    cat >"$tape" <<EOF
Output assets/$name.gif

Require go

Set Shell "bash"
Set Theme "Builtin Dark"
Set FontSize 22
Set Columns $columns
Set Rows $rows
Set Padding 16
Set CursorBlink false
Set LoopOffset 80%
Set PlaybackSpeed 1.5

Type "go run ./example $section"
Enter
Sleep 2
EOF

    vhs "$tape"
  fi

  printf 'wrote assets/%s.%s (%s lines, longest line %s columns)\n' "$name" "$ext" "$lines" "$longest"
done
