#!/usr/bin/env bash
# Regenerate the README captures from the showcase program.
#
#   ./example/capture.sh           freeze -> PNG, the format the README uses
#   ./example/capture.sh --gif     vhs -> GIF, animated, 256-colour palette
#
# Two rules keep this honest. The section names come from the program itself
# (`go run ./example -list`), so a feature that no longer exists cannot be captured.
# And the asset names are pinned in the map below, because the README already links
# them and renaming an asset is a broken image on a package page.
#
# PNG is the default for a reason. GIF is indexed 8-bit: 256 colours maximum, and this
# package's headline features are 256-colour indices and truecolour hex, so a GIF has
# to approximate the exact thing the capture exists to show.
set -euo pipefail

cd "$(dirname "$0")/.."

mode=png

case "${1:-}" in
  --gif) mode=gif ;;
  -h|--help)
    sed -n '2,17p' "$0"
    exit 0
    ;;
esac

sections=$(go run ./example -list)

if [[ $mode == png ]] && ! command -v freeze >/dev/null 2>&1; then
  cat >&2 <<'EOF'
freeze is not installed, and it is what writes the PNG captures the README uses. It
keeps the exact colours the package emits.

  Arch (AUR):   yay -S freeze
  Homebrew:     brew install charmbracelet/tap/freeze

Or run with --gif to capture with vhs instead, accepting the palette limit.
EOF
  exit 1
fi

if [[ $mode == gif ]] && ! command -v vhs >/dev/null 2>&1; then
  echo "vhs is not installed." >&2
  exit 1
fi

# section:asset-name. The names the README already links, plus the sections that do
# not have a capture yet.
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

mkdir -p assets

# vhs resolves its Output path relative to the tape file, so the tapes sit in the
# repository root next to assets/ and every path inside a tape is written from here.
tapes=()
trap 'rm -f "${tapes[@]}"' EXIT

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
    # freeze's -W is the output width in pixels, so the width comes from the longest
    # line the section printed. A one-line feature gets a small image and a wide one
    # gets a wide image, instead of everything being cropped to the same box.
    #
    # sh -c is required: freeze executes the string as a command rather than through a
    # shell, so a bare `go run ... || true` is handed to exec as one word.
    png_width=$(( (longest + 6) * 18 ))

    if (( png_width < 480 )); then
      png_width=480
    fi

    if (( png_width > 1000 )); then
      png_width=1000
    fi

    freeze -o "assets/$name.png" -W "$png_width" -p 16 -m 24 \
      --execute "sh -c 'go run ./example $section || true'"
  else
    # vhs sizes the canvas, so width and height come from what the program actually
    # printed instead of a number chosen by hand per section. Stripping the escape
    # codes first makes the length the text length rather than the byte length.
    width=$(( (longest + 4) * 13 ))
    height=$(( (lines + 2) * 30 ))

    if (( width < 420 )); then
      width=420
    fi

    if (( width > 1100 )); then
      width=1100
    fi

    tape="$root/.capture-$section.tape"
    tapes+=("$tape")

    # A recorder paints the terminal with its own theme: the base ANSI colours come
    # from the theme, not from the viewer's terminal. Pinning it is what makes one run
    # of captures agree with the next, and it is why the README names the theme.
    cat >"$tape" <<EOF
Output assets/$name.gif
Set Shell "bash"
Set Theme "Builtin Dark"
Set Width $width
Set Height $height
Set FontSize 22
Set Padding 24
Type "go run ./example $section"
Enter
Sleep 3
EOF

    vhs "$tape"
  fi

  printf 'wrote assets/%s.%s (%s lines, longest line %s columns)\n' "$name" "$mode" "$lines" "$longest"
done
